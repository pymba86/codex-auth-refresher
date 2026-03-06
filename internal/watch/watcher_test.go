package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherEmitsOnJSONCreate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	watcher, err := New(dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer watcher.Close()

	path := filepath.Join(dir, "user.json")
	if err := os.WriteFile(path, []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	select {
	case <-watcher.Events():
	case err := <-watcher.Errors():
		t.Fatalf("watcher error = %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("expected watcher event")
	}
}
