package session

import (
	"testing"

	"ai-identity-manager/internal/identity"
)

func TestTrackerBaselineAndIPChange(t *testing.T) {
	tr := NewTracker()
	tr.NoteExitIP(identity.ExitIdentity{IP: "1.1.1.1"}, identity.ExitMatched)
	if tr.Worst() != identity.Green {
		t.Fatalf("baseline should be GREEN, got %s", tr.Worst())
	}
	tr.NoteExitIP(identity.ExitIdentity{IP: "2.2.2.2"}, identity.ExitMatched)
	if tr.Worst() != identity.Red || !tr.Report().IPChanged {
		t.Fatalf("ip change should be RED: %+v", tr.Report())
	}
}

func TestTrackerLeakIsRed(t *testing.T) {
	tr := NewTracker()
	tr.NoteSnapshot(Snapshot{AI: make([]ConnVerdict, 1), Leaks: make([]ConnVerdict, 1)})
	if tr.Worst() != identity.Red {
		t.Fatalf("leak should be RED")
	}
}

func TestTrackerUnverifiableIsYellow(t *testing.T) {
	tr := NewTracker()
	tr.NoteExitIP(identity.ExitIdentity{IP: "1.1.1.1"}, identity.ExitUnverifiable)
	if tr.Worst() != identity.Yellow {
		t.Fatalf("unverifiable should be YELLOW, got %s", tr.Worst())
	}
}

func TestTrackerMismatchIsRed(t *testing.T) {
	tr := NewTracker()
	tr.NoteExitIP(identity.ExitIdentity{IP: "1.1.1.1"}, identity.ExitMismatched)
	if tr.Worst() != identity.Red {
		t.Fatalf("mismatch should be RED")
	}
}

func TestTrackerChainErrorsDoNotChangeWorst(t *testing.T) {
	tr := NewTracker()
	tr.NoteChainError(identity.ErrorIPRoyal)
	tr.NoteChainError(identity.ErrorIPRoyal)
	if tr.Worst() != identity.Green {
		t.Fatalf("chain errors must not change level, got %s", tr.Worst())
	}
	r := tr.Report()
	if r.ChainErrors != 2 || r.ChainBySource[identity.ErrorIPRoyal] != 2 {
		t.Fatalf("report = %+v", r)
	}
}
