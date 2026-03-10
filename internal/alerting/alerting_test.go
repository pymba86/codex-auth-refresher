package alerting

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"codex-auth-refresher/internal/metrics"
)

type fakeSender struct {
	mu     sync.Mutex
	emails []Email
	errs   []error
}

func (f *fakeSender) Send(_ context.Context, email Email) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		if err != nil {
			return err
		}
	}
	copyEmail := Email{
		To:      append([]string(nil), email.To...),
		Subject: email.Subject,
		Body:    email.Body,
	}
	f.emails = append(f.emails, copyEmail)
	return nil
}

func (f *fakeSender) Emails() []Email {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Email, len(f.emails))
	copy(out, f.emails)
	return out
}

func TestNotifierProcessDeduplicatesAndResetsAfterRecovery(t *testing.T) {
	t.Parallel()
	registry := metrics.New()
	sender := &fakeSender{}
	notifier := NewNotifier(sender, registry, slog.New(slog.NewTextHandler(io.Discard, nil)), []string{"ops@example.com"}, 1)

	generatedAt := time.Date(2026, 3, 10, 9, 30, 0, 0, time.UTC)
	problem := Problem{
		Path:                "/tmp/auth/codex-a.json",
		File:                "codex-a.json",
		AccountID:           "acct-1",
		State:               "degraded",
		LastError:           "timeout",
		ConsecutiveFailures: 1,
	}

	notifier.process(context.Background(), Snapshot{GeneratedAt: generatedAt, Problems: []Problem{problem}})
	notifier.process(context.Background(), Snapshot{GeneratedAt: generatedAt.Add(time.Minute), Problems: []Problem{problem}})

	updated := problem
	updated.LastError = "invalid_client"
	notifier.process(context.Background(), Snapshot{GeneratedAt: generatedAt.Add(2 * time.Minute), Problems: []Problem{updated}})
	notifier.process(context.Background(), Snapshot{GeneratedAt: generatedAt.Add(3 * time.Minute)})
	notifier.process(context.Background(), Snapshot{GeneratedAt: generatedAt.Add(4 * time.Minute), Problems: []Problem{updated}})

	emails := sender.Emails()
	if len(emails) != 3 {
		t.Fatalf("len(emails) = %d, want 3", len(emails))
	}
	if got := emails[0].To; len(got) != 1 || got[0] != "ops@example.com" {
		t.Fatalf("emails[0].To = %#v, want recipient list", got)
	}
	if !strings.Contains(emails[0].Subject, "1 problem(s): degraded=1") {
		t.Fatalf("emails[0].Subject = %q, want degraded summary", emails[0].Subject)
	}
	if !strings.Contains(emails[0].Body, "file: codex-a.json") {
		t.Fatalf("emails[0].Body = %q, want file name", emails[0].Body)
	}
	if !strings.Contains(emails[1].Body, "last_error: invalid_client") {
		t.Fatalf("emails[1].Body = %q, want updated error", emails[1].Body)
	}

	snapshot := registry.Snapshot()
	if snapshot.EmailNotificationsSentTotal != 3 {
		t.Fatalf("EmailNotificationsSentTotal = %d, want 3", snapshot.EmailNotificationsSentTotal)
	}
	if snapshot.EmailNotificationsFailedTotal != 0 {
		t.Fatalf("EmailNotificationsFailedTotal = %d, want 0", snapshot.EmailNotificationsFailedTotal)
	}
}

func TestNotifierProcessRetriesAfterSendFailure(t *testing.T) {
	t.Parallel()
	registry := metrics.New()
	sender := &fakeSender{errs: []error{errors.New("smtp down")}}
	notifier := NewNotifier(sender, registry, slog.New(slog.NewTextHandler(io.Discard, nil)), []string{"ops@example.com"}, 1)

	problem := Problem{
		Path:      "/tmp/auth/codex-a.json",
		File:      "codex-a.json",
		State:     "reauth_required",
		LastError: "invalid_grant",
	}

	notifier.process(context.Background(), Snapshot{GeneratedAt: time.Now().UTC(), Problems: []Problem{problem}})
	notifier.process(context.Background(), Snapshot{GeneratedAt: time.Now().UTC().Add(time.Minute), Problems: []Problem{problem}})

	emails := sender.Emails()
	if len(emails) != 1 {
		t.Fatalf("len(emails) = %d, want 1 successful retry", len(emails))
	}
	snapshot := registry.Snapshot()
	if snapshot.EmailNotificationsFailedTotal != 1 {
		t.Fatalf("EmailNotificationsFailedTotal = %d, want 1", snapshot.EmailNotificationsFailedTotal)
	}
	if snapshot.EmailNotificationsSentTotal != 1 {
		t.Fatalf("EmailNotificationsSentTotal = %d, want 1", snapshot.EmailNotificationsSentTotal)
	}
}

func TestNotifierNotifyDropsWhenQueueIsFull(t *testing.T) {
	t.Parallel()
	registry := metrics.New()
	notifier := NewNotifier(&fakeSender{}, registry, slog.New(slog.NewTextHandler(io.Discard, nil)), []string{"ops@example.com"}, 1)

	notifier.Notify(Snapshot{GeneratedAt: time.Now().UTC(), Problems: []Problem{{Path: "/tmp/auth/codex-a.json", File: "codex-a.json", State: "degraded"}}})
	notifier.Notify(Snapshot{GeneratedAt: time.Now().UTC(), Problems: []Problem{{Path: "/tmp/auth/codex-b.json", File: "codex-b.json", State: "degraded"}}})

	snapshot := registry.Snapshot()
	if snapshot.EmailNotificationsDroppedTotal != 1 {
		t.Fatalf("EmailNotificationsDroppedTotal = %d, want 1", snapshot.EmailNotificationsDroppedTotal)
	}
}
