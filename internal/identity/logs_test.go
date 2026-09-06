package identity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverLogFilesFindsNestedServiceLatestFirst(t *testing.T) {
	dir := t.TempDir()
	serviceDir := filepath.Join(dir, "logs", "service")
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		t.Fatalf("mkdir service logs: %v", err)
	}
	nested := filepath.Join(serviceDir, "service_latest.log")
	shallow := filepath.Join(dir, "logs", "latest.log")
	if err := os.WriteFile(nested, []byte("nested"), 0o600); err != nil {
		t.Fatalf("write nested log: %v", err)
	}
	if err := os.WriteFile(shallow, []byte("shallow"), 0o600); err != nil {
		t.Fatalf("write shallow log: %v", err)
	}

	files, err := DiscoverLogFiles(dir)
	if err != nil {
		t.Fatalf("discover logs: %v", err)
	}
	if len(files) < 2 {
		t.Fatalf("files = %+v", files)
	}
	if files[0].Path != nested {
		t.Fatalf("first log = %q, want nested service log %q", files[0].Path, nested)
	}
}

func TestReadLogFilesReadsTailOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latest.log")
	content := strings.Repeat("old\n", 100) + "AI-Static[IPRoyal-Static]\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	text, sources, err := ReadLogFiles([]LogFile{{Path: path, Kind: "latest"}}, 64)
	if err != nil {
		t.Fatalf("read log files: %v", err)
	}
	if !strings.Contains(text, "AI-Static[IPRoyal-Static]") {
		t.Fatalf("tail text missed latest evidence: %q", text)
	}
	if len(sources) != 1 || sources[0] != path {
		t.Fatalf("sources = %+v", sources)
	}
}
