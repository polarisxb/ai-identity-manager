package evidence

import "testing"

func TestParseRouteStaticClaude(t *testing.T) {
	line := `time="2026-06-18T13:46:05.7+08:00" level=info msg="[TCP] 127.0.0.1:6(claude.exe) --> claude.ai:443 match ProcessName(claude.exe) using AI-Static[IPRoyal-Static]"`

	ro, ok := parseRoute(EvidenceLine{Raw: line, Seq: 1}, "AI-Static", "IPRoyal-Static")

	if !ok || ro.Target != TargetClaude || ro.Kind != RouteStatic || ro.Process != "claude.exe" || ro.Host != "claude.ai" || ro.Port != "443" {
		t.Fatalf("ro=%+v ok=%v", ro, ok)
	}
	if ro.When.IsZero() {
		t.Fatal("timestamp not parsed")
	}
}

func TestParseRouteNonStaticCodex(t *testing.T) {
	line := `codex.exe --> www.codexapis.com:443 match Match using Proxies[JMS-x]`

	ro, ok := parseRoute(EvidenceLine{Raw: line}, "AI-Static", "IPRoyal-Static")

	if !ok || ro.Kind != RouteNonStatic || ro.Target != TargetCodex || ro.Host != "www.codexapis.com" || ro.Port != "443" {
		t.Fatalf("ro=%+v ok=%v", ro, ok)
	}
}

func TestLinesFromTextPreservesSourceAndSeq(t *testing.T) {
	lines := LinesFromText("latest.log", "\nfirst\n\nsecond\n", 7)

	if len(lines) != 2 {
		t.Fatalf("lines=%+v", lines)
	}
	if lines[0].Source != "latest.log" || lines[0].Seq != 7 || lines[0].Raw != "first" {
		t.Fatalf("first=%+v", lines[0])
	}
	if lines[1].Source != "latest.log" || lines[1].Seq != 8 || lines[1].Raw != "second" {
		t.Fatalf("second=%+v", lines[1])
	}
}
