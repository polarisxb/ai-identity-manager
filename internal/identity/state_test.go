package identity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveAndLoadVerifyResultDoesNotPersistSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".ai-identity", "status.json")
	result := VerifyResult{
		Checked:         true,
		ExitIP:          "203.0.113.8",
		ASN:             "AS12345",
		Country:         "US",
		ExpectedExitIP:  "203.0.113.8",
		ExpectedASN:     "AS12345",
		ExpectedCountry: "US",
		Error:           "proxy failed with <redacted-secret>",
	}
	checkedAt := time.Date(2026, 6, 17, 1, 2, 3, 0, time.FixedZone("CST", 8*60*60))

	if err := SaveVerifyResult(path, result, checkedAt); err != nil {
		t.Fatalf("save verify result: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	for _, forbidden := range []string{"static-user", "static-pass", "Proxy-Authorization"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("state file leaked %q:\n%s", forbidden, raw)
		}
	}

	loaded, err := LoadVerifyResult(path)
	if err != nil {
		t.Fatalf("load verify result: %v", err)
	}
	if !loaded.Checked || loaded.ExitIP != result.ExitIP || loaded.ASN != result.ASN || loaded.Country != result.Country {
		t.Fatalf("loaded result mismatch: %+v", loaded)
	}
	if loaded.CheckedAt.IsZero() || !loaded.CheckedAt.Equal(checkedAt) {
		t.Fatalf("checked_at mismatch: %s", loaded.CheckedAt)
	}
}

func TestRecordActivityMerges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "activity.json")
	now := time.Now()

	if err := RecordApply(path, now); err != nil {
		t.Fatal(err)
	}
	if err := RecordReload(path, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	a, err := LoadActivity(path)
	if err != nil {
		t.Fatal(err)
	}
	if a.LastApplyAt.IsZero() || a.LastReloadAt.IsZero() {
		t.Fatalf("a=%+v", a)
	}
	if !a.LastApplyAt.Equal(now) || !a.LastReloadAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("activity timestamps mismatch: %+v", a)
	}
}

func TestLoadActivityMissing(t *testing.T) {
	a, err := LoadActivity(filepath.Join(t.TempDir(), "none.json"))
	if err != nil || !a.LastApplyAt.IsZero() || !a.LastReloadAt.IsZero() {
		t.Fatalf("a=%+v err=%v", a, err)
	}
}
