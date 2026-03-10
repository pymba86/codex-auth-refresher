package alerting

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

type testSMTPServer struct {
	conn net.Conn
	done chan error

	mu       sync.Mutex
	mailFrom string
	rcptTo   []string
	message  string
}

func newTestSMTPServer(conn net.Conn) *testSMTPServer {
	server := &testSMTPServer{
		conn: conn,
		done: make(chan error, 1),
	}
	go server.serve()
	return server
}

func (s *testSMTPServer) serve() {
	defer s.conn.Close()

	reader := bufio.NewReader(s.conn)
	writer := bufio.NewWriter(s.conn)
	writeLine := func(line string) error {
		if _, err := writer.WriteString(line + "\r\n"); err != nil {
			return err
		}
		return writer.Flush()
	}

	if err := writeLine("220 localhost ESMTP"); err != nil {
		s.done <- err
		return
	}

	var dataLines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			s.done <- err
			return
		}
		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)

		switch {
		case strings.HasPrefix(upper, "EHLO "), strings.HasPrefix(upper, "HELO "):
			if err := writeLine("250-localhost"); err != nil {
				s.done <- err
				return
			}
			if err := writeLine("250 8BITMIME"); err != nil {
				s.done <- err
				return
			}
		case strings.HasPrefix(upper, "MAIL FROM:"):
			s.mu.Lock()
			s.mailFrom = line
			s.mu.Unlock()
			if err := writeLine("250 2.1.0 Ok"); err != nil {
				s.done <- err
				return
			}
		case strings.HasPrefix(upper, "RCPT TO:"):
			s.mu.Lock()
			s.rcptTo = append(s.rcptTo, line)
			s.mu.Unlock()
			if err := writeLine("250 2.1.5 Ok"); err != nil {
				s.done <- err
				return
			}
		case upper == "DATA":
			if err := writeLine("354 End data with <CR><LF>.<CR><LF>"); err != nil {
				s.done <- err
				return
			}
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					s.done <- err
					return
				}
				dataLine = strings.TrimRight(dataLine, "\r\n")
				if dataLine == "." {
					break
				}
				dataLines = append(dataLines, dataLine)
			}
			s.mu.Lock()
			s.message = strings.Join(dataLines, "\n")
			s.mu.Unlock()
			if err := writeLine("250 2.0.0 queued"); err != nil {
				s.done <- err
				return
			}
		case upper == "QUIT":
			s.done <- writeLine("221 2.0.0 bye")
			return
		default:
			s.done <- fmt.Errorf("unexpected SMTP command: %s", line)
			return
		}
	}
}

func (s *testSMTPServer) Wait(t *testing.T) {
	t.Helper()
	select {
	case err := <-s.done:
		if err != nil {
			t.Fatalf("test SMTP server error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for test SMTP server")
	}
}

func (s *testSMTPServer) Snapshot() (mailFrom string, rcptTo []string, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mailFrom, append([]string(nil), s.rcptTo...), s.message
}

func TestSMTPSenderSendOverPlainSMTP(t *testing.T) {
	t.Parallel()
	clientConn, serverConn := net.Pipe()
	server := newTestSMTPServer(serverConn)

	sender, err := NewSMTPSender(SMTPConfig{
		Host:    "smtp.example.com",
		Port:    25,
		TLSMode: TLSModeNone,
		From:    "Alerts <alerts@example.com>",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewSMTPSender() error = %v", err)
	}
	sender.dialContext = func(context.Context, string, string) (net.Conn, error) {
		return clientConn, nil
	}

	err = sender.Send(context.Background(), Email{
		To:      []string{"ops1@example.com", "ops2@example.com"},
		Subject: "Codex auth issue",
		Body:    "first line\nsecond line",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	server.Wait(t)
	mailFrom, rcptTo, message := server.Snapshot()
	if !strings.Contains(mailFrom, "alerts@example.com") {
		t.Fatalf("mailFrom = %q, want alerts@example.com", mailFrom)
	}
	if len(rcptTo) != 2 {
		t.Fatalf("len(rcptTo) = %d, want 2", len(rcptTo))
	}
	if !strings.Contains(message, "From: \"Alerts\" <alerts@example.com>") {
		t.Fatalf("message = %q, want From header", message)
	}
	if !strings.Contains(message, "To: <ops1@example.com>, <ops2@example.com>") {
		t.Fatalf("message = %q, want To header", message)
	}
	if !strings.Contains(message, "Subject: Codex auth issue") {
		t.Fatalf("message = %q, want Subject header", message)
	}
	if !strings.Contains(message, "first line\nsecond line") {
		t.Fatalf("message = %q, want body", message)
	}
}
