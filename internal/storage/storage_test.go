package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomicPreservesPermissionsAndContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(path, []byte(`{"old":true}`), 0o640); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := WriteFileAtomic(path, []byte(`{"new":true}`), 0o640); err != nil {
		t.Fatalf("WriteFileAtomic() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != `{"new":true}` {
		t.Fatalf("content = %s, want updated JSON", string(data))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("perm = %v, want 0640", got)
	}
}
