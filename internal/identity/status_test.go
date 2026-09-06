package identity

import (
	"strings"
	"testing"
	"time"

	"ai-identity-manager/internal/identity/evidence"
)

func TestEvalTarget(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	clk := Clock{Now: now, LastApplyAt: now.Add(-30 * time.Minute), MaxAge: time.Hour}
	state := func(kind evidence.RouteKind, when time.Time) targetState {
		return evalTarget(evidence.RouteObservation{Kind: kind, When: when}, clk)
	}

	if got := state(evidence.RouteStatic, now); got != tsStaticLive {
		t.Fatalf("static fresh = %v", got)
	}
	if got := state(evidence.RouteStatic, now.Add(-2*time.Hour)); got != tsStaticStale {
		t.Fatalf("static stale = %v", got)
	}
	if got := state(evidence.RouteNonStatic, now.Add(-10*time.Minute)); got != tsLeakLive {
		t.Fatalf("leak after apply = %v", got)
	}
	if got := state(evidence.RouteNonStatic, now.Add(-45*time.Minute)); got != tsLeakStalePreApply {
		t.Fatalf("leak before apply = %v", got)
	}
	if got := state(evidence.RouteNonStatic, now.Add(-2*time.Hour)); got != tsLeakStaleOld {
		t.Fatalf("old leak = %v", got)
	}
	if got := state(evidence.RouteDirect, time.Time{}); got != tsLeakLive {
		t.Fatalf("undated leak = %v", got)
	}
}

func TestEvaluateStatusReasonMatrix(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	base := StatusInputs{
		ConfigOK:            true,
		ChainOK:             true,
		ControllerReachable: true,
		LogsReadable:        true,
		SwitchAfterFailures: 3,
		Clock: Clock{
			Now:         now,
			LastApplyAt: now.Add(-30 * time.Minute),
			MaxAge:      time.Hour,
		},
	}
	match := VerifyResult{Checked: true, ExitIP: "203.0.113.8", ExpectedExitIP: "203.0.113.8"}
	mismatch := VerifyResult{Checked: true, ExitIP: "198.51.100.9", ExpectedExitIP: "203.0.113.8"}
	noExpected := VerifyResult{Checked: true, ExitIP: "203.0.113.8"}

	cases := []struct {
		name       string
		mutate     func(*StatusInputs)
		wantLevel  Level
		wantCode   ReasonCode
		wantReason string
	}{
		{"config invalid", func(in *StatusInputs) { in.ConfigOK = false }, Red, RConfigInvalid, ""},
		{"chain incomplete", func(in *StatusInputs) { in.ChainOK = false }, Red, RChainIncomplete, ""},
		{"ai direct", func(in *StatusInputs) {
			in.Window = routeWindow(route(evidence.TargetClaude, evidence.RouteDirect, now.Add(-10*time.Minute)))
		}, Red, RAIDirect, ""},
		{"ai nonstatic undated", func(in *StatusInputs) {
			in.Window = routeWindow(route(evidence.TargetCodex, evidence.RouteNonStatic, time.Time{}))
		}, Red, RAINonStatic, ""},
		{"runtime stale leak after apply", func(in *StatusInputs) {
			in.Window = routeWindow(route(evidence.TargetCodex, evidence.RouteNonStatic, now.Add(-10*time.Minute)))
		}, Red, RRuntimeStaleLeak, ""},
		{"verify mismatch", func(in *StatusInputs) {
			in.Window = routeWindow(route(evidence.TargetClaude, evidence.RouteStatic, now))
			in.Verify = mismatch
		}, Red, RVerifyMismatch, "exit IP mismatch"},
		{"iproyal down", func(in *StatusInputs) { in.Window = errorWindow(evidence.ErrIPRoyal, 1) }, Red, RIPRoyalDown, ""},
		{"jms down", func(in *StatusInputs) { in.Window = errorWindow(evidence.ErrJMS, 3) }, Red, RJMSDown, ""},
		{"ai business fail", func(in *StatusInputs) { in.Window = errorWindow(evidence.ErrAIBusiness, 1) }, Red, RAIBusinessFail, ""},
		{"reload pending", func(in *StatusInputs) {
			in.Window = routeWindow(route(evidence.TargetCodex, evidence.RouteNonStatic, now.Add(-45*time.Minute)))
		}, Yellow, YReloadPending, ""},
		{"old leak does not red", func(in *StatusInputs) {
			in.Window = routeWindow(route(evidence.TargetCodex, evidence.RouteNonStatic, now.Add(-2*time.Hour)))
		}, Yellow, YNoAIHit, ""},
		{"no ai hit", func(in *StatusInputs) {}, Yellow, YNoAIHit, ""},
		{"verify missing", func(in *StatusInputs) {
			in.Window = routeWindow(route(evidence.TargetClaude, evidence.RouteStatic, now))
		}, Yellow, YVerifyMissing, ""},
		{"verify no expectation", func(in *StatusInputs) {
			in.Window = routeWindow(route(evidence.TargetClaude, evidence.RouteStatic, now))
			in.Verify = noExpected
		}, Yellow, YVerifyNoExpect, ""},
		{"controller down", func(in *StatusInputs) { in.ControllerReachable = false }, Yellow, YControllerDown, ""},
		{"logs unreadable", func(in *StatusInputs) { in.LogsReadable = false }, Yellow, YLogsUnreadable, ""},
		{"healthcheck noise", func(in *StatusInputs) { in.Window = healthWindow() }, Yellow, YHealthcheckNoise, ""},
		{"verified", func(in *StatusInputs) {
			in.Window = routeWindow(route(evidence.TargetClaude, evidence.RouteStatic, now))
			in.Verify = match
		}, Green, GVerified, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := base
			tc.mutate(&in)

			got := EvaluateStatus(in)

			if got.Level != tc.wantLevel || got.Code != tc.wantCode {
				t.Fatalf("got level=%s code=%s reasons=%v, want level=%s code=%s", got.Level, got.Code, got.Reasons, tc.wantLevel, tc.wantCode)
			}
			if tc.wantReason != "" && !strings.Contains(strings.Join(got.Reasons, "; "), tc.wantReason) {
				t.Fatalf("reasons=%v missing %q", got.Reasons, tc.wantReason)
			}
		})
	}
}

func route(target evidence.Target, kind evidence.RouteKind, when time.Time) evidence.RouteObservation {
	return evidence.RouteObservation{
		Target:  target,
		Process: string(target) + ".exe",
		Host:    string(target) + ".example.test",
		Kind:    kind,
		When:    when,
		Raw:     string(target) + " route",
	}
}

func routeWindow(routes ...evidence.RouteObservation) evidence.Window {
	w := evidence.Window{
		Routes:      map[string]evidence.RouteObservation{},
		ErrorCounts: map[evidence.ErrorSource]int{},
	}
	for _, ro := range routes {
		key := string(ro.Target) + "|" + ro.Process + "|" + ro.Host
		w.Routes[key] = ro
	}
	return w
}

func errorWindow(src evidence.ErrorSource, count int) evidence.Window {
	return evidence.Window{
		Routes:      map[string]evidence.RouteObservation{},
		ErrorCounts: map[evidence.ErrorSource]int{src: count},
	}
}

func healthWindow() evidence.Window {
	return evidence.Window{
		Routes:          map[string]evidence.RouteObservation{},
		ErrorCounts:     map[evidence.ErrorSource]int{evidence.ErrHealthCheck: 1},
		HealthCheckOnly: true,
	}
}
