package identity

import "testing"

func TestVerifyLevel(t *testing.T) {
	if VerifyLevel(VerifyResult{Checked: true, ExitIP: "1.1.1.1", ExpectedExitIP: "1.1.1.1"}) != Green {
		t.Fatal("match -> GREEN")
	}
	if VerifyLevel(VerifyResult{Checked: true, ExitIP: "2.2.2.2", ExpectedExitIP: "1.1.1.1"}) != Red {
		t.Fatal("mismatch -> RED")
	}
	if VerifyLevel(VerifyResult{Checked: true, ExitIP: "1.1.1.1"}) != Yellow {
		t.Fatal("no expected -> YELLOW")
	}
	if VerifyLevel(VerifyResult{Checked: true, Error: "boom"}) != Red {
		t.Fatal("error -> RED")
	}
}
