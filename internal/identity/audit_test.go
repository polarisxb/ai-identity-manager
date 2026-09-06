package identity

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuditAppendAndLoadRedacts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")

	if err := AppendAudit(path, AuditEvent{Type: "verify", Summary: "level=GREEN pass=ctl-secret"}, []string{"ctl-secret"}); err != nil {
		t.Fatal(err)
	}
	if err := AppendAudit(path, AuditEvent{Type: "apply", Summary: "profile=x changed=3"}, nil); err != nil {
		t.Fatal(err)
	}

	evs, err := LoadRecentAudit(path, 10)
	if err != nil || len(evs) != 2 {
		t.Fatalf("evs=%+v err=%v", evs, err)
	}
	if evs[1].Type != "apply" {
		t.Fatalf("order wrong: %+v", evs)
	}
	if strings.Contains(evs[0].Summary, "ctl-secret") || !strings.Contains(evs[0].Summary, "<redacted-secret>") {
		t.Fatalf("not redacted: %q", evs[0].Summary)
	}
}

func TestLoadRecentAuditMissingFile(t *testing.T) {
	evs, err := LoadRecentAudit(filepath.Join(t.TempDir(), "none.log"), 5)
	if err != nil || evs != nil {
		t.Fatalf("evs=%+v err=%v", evs, err)
	}
}

func TestLoadRecentAuditTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	for i := 0; i < 5; i++ {
		if err := AppendAudit(path, AuditEvent{Type: "apply", Summary: "n", Time: time.Unix(int64(i), 0)}, nil); err != nil {
			t.Fatal(err)
		}
	}

	evs, err := LoadRecentAudit(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 {
		t.Fatalf("tail=%d", len(evs))
	}
	if !evs[0].Time.Equal(time.Unix(3, 0)) || !evs[1].Time.Equal(time.Unix(4, 0)) {
		t.Fatalf("tail order wrong: %+v", evs)
	}
}
