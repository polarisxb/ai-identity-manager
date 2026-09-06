package identity

import (
	"fmt"
	"strings"
	"time"

	"ai-identity-manager/internal/identity/evidence"
)

type Level string

const (
	Green  Level = "GREEN"
	Yellow Level = "YELLOW"
	Red    Level = "RED"
)

type ReasonCode string

const (
	RConfigInvalid    ReasonCode = "R-CONFIG-INVALID"
	RChainIncomplete  ReasonCode = "R-CHAIN-INCOMPLETE"
	RAIDirect         ReasonCode = "R-AI-DIRECT"
	RAINonStatic      ReasonCode = "R-AI-NONSTATIC"
	RRuntimeStaleLeak ReasonCode = "R-RUNTIME-STALE-LEAK"
	RVerifyMismatch   ReasonCode = "R-VERIFY-MISMATCH"
	RIPRoyalDown      ReasonCode = "R-IPROYAL-DOWN"
	RJMSDown          ReasonCode = "R-JMS-DOWN"
	RAIBusinessFail   ReasonCode = "R-AI-BUSINESS-FAIL"
	YReloadPending    ReasonCode = "Y-RELOAD-PENDING"
	YNoAIHit          ReasonCode = "Y-NO-AI-HIT"
	YVerifyMissing    ReasonCode = "Y-VERIFY-MISSING"
	YVerifyNoExpect   ReasonCode = "Y-VERIFY-NO-EXPECTATION"
	YControllerDown   ReasonCode = "Y-CONTROLLER-DOWN"
	YLogsUnreadable   ReasonCode = "Y-LOGS-UNREADABLE"
	YHealthcheckNoise ReasonCode = "Y-HEALTHCHECK-NOISE"
	GVerified         ReasonCode = "G-VERIFIED"
	RSessionIPChanged ReasonCode = "R-SESSION-IP-CHANGED"
)

type ErrorSource string

const (
	ErrorIPRoyal        ErrorSource = "IPRoyal"
	ErrorJMS            ErrorSource = "JMS"
	ErrorHealthCheck    ErrorSource = "HealthCheck"
	ErrorOrdinaryProxy  ErrorSource = "OrdinaryProxy"
	ErrorClaudeBusiness ErrorSource = "ClaudeBusiness"
	ErrorUnknown        ErrorSource = "Unknown"
)

type VerifyResult struct {
	CheckedAt       time.Time
	Checked         bool
	ExitIP          string
	ASN             string
	Country         string
	ExpectedExitIP  string
	ExpectedASN     string
	ExpectedCountry string
	Error           string
}

type Clock struct {
	Now         time.Time
	LastApplyAt time.Time
	MaxAge      time.Duration
}

type StatusInputs struct {
	ConfigOK            bool
	ChainOK             bool
	Window              evidence.Window
	Clock               Clock
	Verify              VerifyResult
	ControllerReachable bool
	LogsReadable        bool
	SwitchAfterFailures int
}

type StatusResult struct {
	Level   Level
	Code    ReasonCode
	Reasons []string
}

type targetState int

const (
	tsNoEvidence targetState = iota
	tsStaticLive
	tsStaticStale
	tsLeakLive
	tsLeakStalePreApply
	tsLeakStaleOld
)

func evalTarget(ro evidence.RouteObservation, clk Clock) targetState {
	clk = normalizeClock(clk)
	fresh := ro.When.IsZero() || clk.Now.Sub(ro.When) <= clk.MaxAge
	if ro.Kind == evidence.RouteStatic {
		if fresh {
			return tsStaticLive
		}
		return tsStaticStale
	}
	if ro.When.IsZero() {
		return tsLeakLive
	}
	if !fresh {
		return tsLeakStaleOld
	}
	if !clk.LastApplyAt.IsZero() && ro.When.Before(clk.LastApplyAt) {
		return tsLeakStalePreApply
	}
	return tsLeakLive
}

func EvaluateStatus(in StatusInputs) StatusResult {
	if !in.ConfigOK {
		return res(Red, RConfigInvalid, "config is not valid")
	}
	if !in.ChainOK {
		return res(Red, RChainIncomplete, "AI static chain is not complete")
	}

	clk := normalizeClock(in.Clock)
	anyStaticLive := false
	anyReloadPending := false
	for _, ro := range in.Window.LatestByTarget() {
		switch evalTarget(ro, clk) {
		case tsStaticLive:
			anyStaticLive = true
		case tsLeakLive:
			if ro.Kind == evidence.RouteDirect {
				return res(Red, RAIDirect, fmt.Sprintf("%s used DIRECT", ro.Target))
			}
			if !ro.When.IsZero() && !clk.LastApplyAt.IsZero() && !ro.When.Before(clk.LastApplyAt) {
				return res(Red, RRuntimeStaleLeak, fmt.Sprintf("%s leaking after apply; reload required", ro.Target))
			}
			return res(Red, RAINonStatic, fmt.Sprintf("%s used a non-static group", ro.Target))
		case tsLeakStalePreApply:
			anyReloadPending = true
		}
	}

	ec := in.Window.ErrorCounts
	if ec[evidence.ErrIPRoyal] > 0 {
		return res(Red, RIPRoyalDown, "IPRoyal entry errors")
	}
	if in.SwitchAfterFailures > 0 && ec[evidence.ErrJMS] >= in.SwitchAfterFailures {
		return res(Red, RJMSDown, "JMS bootstrap failing")
	}
	if ec[evidence.ErrAIBusiness] > 0 {
		return res(Red, RAIBusinessFail, "AI business connection errors")
	}

	if in.Verify.Checked {
		if !verifyHasExpectation(in.Verify) {
			return attach(res(Yellow, YVerifyNoExpect, "exit identity was observed, but no expected identity is configured"), in)
		}
		if reason := verifyMismatch(in.Verify); reason != "" {
			return res(Red, RVerifyMismatch, reason)
		}
	}
	if anyReloadPending && !anyStaticLive {
		return attach(res(Yellow, YReloadPending, "applied but runtime not reloaded"), in)
	}
	if in.Verify.Checked && verifyHasExpectation(in.Verify) && verifyMismatch(in.Verify) == "" {
		if anyStaticLive {
			return attach(res(Green, GVerified, "AI traffic used AI-Static and verified exit identity matched"), in)
		}
		return attach(res(Yellow, YNoAIHit, "exit identity matched, but no AI client hit was observed"), in)
	}
	if anyStaticLive {
		return attach(res(Yellow, YVerifyMissing, "AI static hit observed, but exit identity has not been verified"), in)
	}
	return attach(res(Yellow, YNoAIHit, "static chain exists, but no AI client hit was observed"), in)
}

func normalizeClock(clk Clock) Clock {
	if clk.Now.IsZero() {
		clk.Now = time.Now()
	}
	if clk.MaxAge <= 0 {
		clk.MaxAge = 30 * time.Minute
	}
	return clk
}

func res(level Level, code ReasonCode, reason string) StatusResult {
	return StatusResult{Level: level, Code: code, Reasons: []string{reason}}
}

func attach(r StatusResult, in StatusInputs) StatusResult {
	if r.Code == YNoAIHit {
		switch {
		case !in.LogsReadable:
			r.Code = YLogsUnreadable
			r.Reasons = append(r.Reasons, "logs not readable")
		case in.Window.HealthCheckOnly:
			r.Code = YHealthcheckNoise
			r.Reasons = append(r.Reasons, "only health-check noise")
		case !in.ControllerReachable:
			r.Code = YControllerDown
			r.Reasons = append(r.Reasons, "controller unreachable")
		}
	}
	return r
}

func parseLogTimestamp(line string) time.Time {
	if idx := strings.Index(line, `time="`); idx >= 0 {
		start := idx + len(`time="`)
		if end := strings.Index(line[start:], `"`); end >= 0 {
			if parsed, err := time.Parse(time.RFC3339Nano, line[start:start+end]); err == nil {
				return parsed
			}
		}
	}
	if strings.HasPrefix(line, "[") {
		if end := strings.Index(line, "]"); end > 0 {
			value := line[1:end]
			for _, layout := range []string{"2006-01-02 15:04:05.000", "2006-01-02 15:04:05"} {
				if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
					return parsed
				}
			}
		}
	}
	return time.Time{}
}

func verifyHasExpectation(v VerifyResult) bool {
	return v.ExpectedExitIP != "" || v.ExpectedASN != "" || v.ExpectedCountry != ""
}

func verifyMismatch(v VerifyResult) string {
	if v.Error != "" {
		return "verify failed: " + v.Error
	}
	if v.ExpectedExitIP != "" && v.ExitIP != v.ExpectedExitIP {
		return fmt.Sprintf("exit IP mismatch: got %s want %s", v.ExitIP, v.ExpectedExitIP)
	}
	if v.ExpectedASN != "" && !strings.Contains(strings.ToLower(v.ASN), strings.ToLower(v.ExpectedASN)) {
		return fmt.Sprintf("ASN mismatch: got %s want %s", v.ASN, v.ExpectedASN)
	}
	if v.ExpectedCountry != "" && !strings.EqualFold(v.Country, v.ExpectedCountry) {
		return fmt.Sprintf("country mismatch: got %s want %s", v.Country, v.ExpectedCountry)
	}
	return ""
}
