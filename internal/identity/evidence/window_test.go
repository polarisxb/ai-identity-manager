package evidence

import "testing"

func line(raw string, seq int) EvidenceLine {
	return EvidenceLine{Raw: raw, Seq: seq}
}

func TestBuildNewerOverridesOlder(t *testing.T) {
	lines := []EvidenceLine{
		line(`time="2026-06-18T10:00:00+08:00" [TCP] (codex.exe) --> www.codexapis.com:443 using Proxies[JMS-x]`, 1),
		line(`time="2026-06-18T11:00:00+08:00" [TCP] (codex.exe) --> www.codexapis.com:443 using AI-Static[IPRoyal-Static]`, 2),
	}

	w := Build(lines, "AI-Static", "IPRoyal-Static")
	latest := w.LatestByTarget()

	if latest[TargetCodex].Kind != RouteStatic {
		t.Fatalf("newest should win Static: %+v", latest[TargetCodex])
	}
}

func TestBuildIgnoresHipsDaemonUDP(t *testing.T) {
	w := Build([]EvidenceLine{line(`hipsdaemon.exe --> api.anthropic.com:123 [UDP] using DIRECT`, 1)}, "AI-Static", "IPRoyal-Static")

	if len(w.Routes) != 0 {
		t.Fatalf("UDP/123 must be ignored: %+v", w.Routes)
	}
}

func TestBuildHealthCheckOnly(t *testing.T) {
	w := Build([]EvidenceLine{line(`level=warning failed to get the second response from http://1.0.0.1`, 1)}, "AI-Static", "IPRoyal-Static")

	if !w.HealthCheckOnly || w.ErrorCounts[ErrIPRoyal] != 0 {
		t.Fatalf("w=%+v", w)
	}
}

func TestBuild403IsIPRoyalError(t *testing.T) {
	w := Build([]EvidenceLine{line(`level=warning [TCP] dial AI-Static (claude.exe) --> api.anthropic.com:443 error: can not connect remote err code: 403`, 1)}, "AI-Static", "IPRoyal-Static")

	if w.ErrorCounts[ErrIPRoyal] != 1 {
		t.Fatalf("counts=%+v", w.ErrorCounts)
	}
	if len(w.Errors) != 1 || w.Errors[0].Source != ErrIPRoyal || w.Errors[0].Target != TargetClaude {
		t.Fatalf("errors=%+v", w.Errors)
	}
}
