package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"codex-auth-refresher/internal/metrics"
)

type Problem struct {
	Path                string
	File                string
	AccountID           string
	State               string
	LastError           string
	Disabled            bool
	ConsecutiveFailures int
	NextRefreshAt       *time.Time
	LastRefreshAt       *time.Time
}

type Snapshot struct {
	GeneratedAt time.Time
	Problems    []Problem
}

type Email struct {
	To      []string
	Subject string
	Body    string
}

type Sender interface {
	Send(context.Context, Email) error
}

type Notifier struct {
	sender     Sender
	metrics    *metrics.Registry
	logger     *slog.Logger
	recipients []string
	queue      chan Snapshot

	mu           sync.Mutex
	lastNotified map[string]string
}

func NewNotifier(sender Sender, metricsRegistry *metrics.Registry, logger *slog.Logger, recipients []string, queueSize int) *Notifier {
	if logger == nil {
		logger = slog.Default()
	}
	if queueSize <= 0 {
		queueSize = 8
	}
	return &Notifier{
		sender:       sender,
		metrics:      metricsRegistry,
		logger:       logger,
		recipients:   append([]string(nil), recipients...),
		queue:        make(chan Snapshot, queueSize),
		lastNotified: make(map[string]string),
	}
}

func (n *Notifier) Notify(snapshot Snapshot) {
	select {
	case n.queue <- snapshot:
	default:
		if n.metrics != nil {
			n.metrics.IncEmailNotificationsDropped()
		}
		n.logger.Warn("email notification dropped", "reason", "queue_full", "problems", len(snapshot.Problems))
	}
}

func (n *Notifier) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case snapshot := <-n.queue:
			n.process(ctx, snapshot)
		}
	}
}

func (n *Notifier) process(ctx context.Context, snapshot Snapshot) {
	current := make(map[string]problemState, len(snapshot.Problems))
	for _, problem := range snapshot.Problems {
		if strings.TrimSpace(problem.Path) == "" {
			continue
		}
		current[problem.Path] = problemState{
			problem:     problem,
			fingerprint: fingerprint(problem),
		}
	}

	n.mu.Lock()
	for key := range n.lastNotified {
		if _, ok := current[key]; !ok {
			delete(n.lastNotified, key)
		}
	}

	changed := make([]Problem, 0, len(current))
	for key, state := range current {
		if n.lastNotified[key] != state.fingerprint {
			changed = append(changed, state.problem)
		}
	}
	n.mu.Unlock()

	if len(changed) == 0 {
		return
	}

	currentProblems := sortedProblems(current)
	changedProblems := append([]Problem(nil), changed...)
	sortProblems(changedProblems)

	email := Email{
		To:      append([]string(nil), n.recipients...),
		Subject: buildSubject(currentProblems),
		Body:    buildBody(snapshot.GeneratedAt, changedProblems, currentProblems),
	}
	if err := n.sender.Send(ctx, email); err != nil {
		if n.metrics != nil {
			n.metrics.IncEmailNotificationsFailed()
		}
		n.logger.Error("email notification failed", "error", err, "changed_problems", len(changedProblems))
		return
	}

	n.mu.Lock()
	for key := range n.lastNotified {
		if _, ok := current[key]; !ok {
			delete(n.lastNotified, key)
		}
	}
	for key, state := range current {
		n.lastNotified[key] = state.fingerprint
	}
	n.mu.Unlock()

	if n.metrics != nil {
		n.metrics.IncEmailNotificationsSent()
	}
	n.logger.Info("email notification sent", "changed_problems", len(changedProblems), "current_problems", len(currentProblems))
}

type problemState struct {
	problem     Problem
	fingerprint string
}

func fingerprint(problem Problem) string {
	return strings.TrimSpace(problem.State) + "\n" + strings.TrimSpace(problem.LastError)
}

func sortedProblems(current map[string]problemState) []Problem {
	problems := make([]Problem, 0, len(current))
	for _, state := range current {
		problems = append(problems, state.problem)
	}
	sortProblems(problems)
	return problems
}

func sortProblems(problems []Problem) {
	sort.SliceStable(problems, func(i, j int) bool {
		leftPriority := statePriority(problems[i].State)
		rightPriority := statePriority(problems[j].State)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		return strings.ToLower(problems[i].File) < strings.ToLower(problems[j].File)
	})
}

func statePriority(state string) int {
	switch state {
	case "reauth_required":
		return 0
	case "invalid_json":
		return 1
	case "degraded":
		return 2
	default:
		return 3
	}
}

func buildSubject(problems []Problem) string {
	parts := make([]string, 0, 1+len(problems))
	parts = append(parts, fmt.Sprintf("%d problem(s)", len(problems)))
	for _, summary := range summarizeStates(problems) {
		parts = append(parts, fmt.Sprintf("%s=%d", summary.State, summary.Count))
	}
	return strings.Join(parts, ": ")
}

func buildBody(generatedAt time.Time, changedProblems, currentProblems []Problem) string {
	builder := &strings.Builder{}
	timestamp := generatedAt.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	fmt.Fprintf(builder, "codex-auth-refresher detected problem(s)\n")
	fmt.Fprintf(builder, "generated_at: %s\n", timestamp.Format(time.RFC3339))
	fmt.Fprintf(builder, "changed_problems: %d\n", len(changedProblems))
	fmt.Fprintf(builder, "current_problems: %d\n", len(currentProblems))

	summaries := summarizeStates(currentProblems)
	if len(summaries) > 0 {
		builder.WriteString("summary:")
		for _, summary := range summaries {
			fmt.Fprintf(builder, " %s=%d", summary.State, summary.Count)
		}
		builder.WriteString("\n")
	}

	builder.WriteString("\nChanged problems:\n")
	for _, problem := range changedProblems {
		writeProblem(builder, problem)
	}

	builder.WriteString("\nCurrent active problems:\n")
	for _, problem := range currentProblems {
		writeProblem(builder, problem)
	}

	return builder.String()
}

type stateSummary struct {
	State string
	Count int
}

func summarizeStates(problems []Problem) []stateSummary {
	counts := map[string]int{}
	for _, problem := range problems {
		counts[problem.State]++
	}

	states := make([]stateSummary, 0, len(counts))
	for state, count := range counts {
		states = append(states, stateSummary{State: state, Count: count})
	}
	sort.SliceStable(states, func(i, j int) bool {
		leftPriority := statePriority(states[i].State)
		rightPriority := statePriority(states[j].State)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		return states[i].State < states[j].State
	})
	return states
}

func writeProblem(builder *strings.Builder, problem Problem) {
	fmt.Fprintf(builder, "- file: %s\n", fallback(problem.File))
	fmt.Fprintf(builder, "  account_id: %s\n", fallback(problem.AccountID))
	fmt.Fprintf(builder, "  state: %s\n", fallback(problem.State))
	fmt.Fprintf(builder, "  last_error: %s\n", fallback(problem.LastError))
	fmt.Fprintf(builder, "  disabled: %t\n", problem.Disabled)
	fmt.Fprintf(builder, "  consecutive_failures: %d\n", problem.ConsecutiveFailures)
	fmt.Fprintf(builder, "  next_refresh_at: %s\n", formatTime(problem.NextRefreshAt))
	fmt.Fprintf(builder, "  last_refresh_at: %s\n", formatTime(problem.LastRefreshAt))
}

func fallback(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "-"
	}
	return trimmed
}

func formatTime(value *time.Time) string {
	if value == nil {
		return "-"
	}
	return value.UTC().Format(time.RFC3339)
}
