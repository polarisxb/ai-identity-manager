package session

import (
	"testing"

	"ai-identity-manager/internal/controller"
)

func conn(host, proc string, chains ...string) controller.Connection {
	return controller.Connection{
		Chains:   chains,
		Metadata: controller.Metadata{Host: host, Process: proc},
	}
}

func TestEvaluateAllStatic(t *testing.T) {
	conns := []controller.Connection{
		conn("api.anthropic.com", "claude.exe", "IPRoyal-Static", "AI-Static"),
		conn("claude.ai", "claude.exe", "IPRoyal-Static", "AI-Static"),
		conn("baidu.com", "wechat.exe", "DIRECT"), // 非 AI，忽略
	}
	s := Evaluate(conns, "AI-Static", "IPRoyal-Static")
	if !s.AllStatic {
		t.Fatalf("AllStatic=false, leaks=%+v", s.Leaks)
	}
	if len(s.AI) != 2 {
		t.Fatalf("AI=%d want 2", len(s.AI))
	}
	if s.AI[0].ExitNode != "IPRoyal-Static" || s.AI[0].EntryGroup != "AI-Static" {
		t.Fatalf("chain ends: exit=%q entry=%q", s.AI[0].ExitNode, s.AI[0].EntryGroup)
	}
}

func TestEvaluateDetectsLeak(t *testing.T) {
	conns := []controller.Connection{
		conn("api.anthropic.com", "claude.exe", "IPRoyal-Static", "AI-Static"),
		conn("www.codexapis.com", "codex.exe", "JMS-x", "Proxies"), // 泄漏
	}
	s := Evaluate(conns, "AI-Static", "IPRoyal-Static")
	if s.AllStatic {
		t.Fatal("expected leak, got AllStatic=true")
	}
	if len(s.Leaks) != 1 || s.Leaks[0].Target != TargetCodex {
		t.Fatalf("leaks=%+v", s.Leaks)
	}
	if s.ByTarget[TargetCodex] != 1 {
		t.Fatalf("ByTarget=%+v", s.ByTarget)
	}
}

func TestEvaluateNoAIIsAllStatic(t *testing.T) {
	s := Evaluate([]controller.Connection{conn("baidu.com", "wechat.exe", "DIRECT")}, "AI-Static", "IPRoyal-Static")
	if !s.AllStatic || len(s.AI) != 0 {
		t.Fatalf("snap=%+v", s)
	}
}

func TestEvaluateDefaultsGroupNames(t *testing.T) {
	// finalGroup/staticNode 留空时用默认 AI-Static / IPRoyal-Static
	s := Evaluate([]controller.Connection{
		conn("claude.ai", "claude.exe", "IPRoyal-Static", "AI-Static"),
	}, "", "")
	if !s.AllStatic {
		t.Fatalf("AllStatic=false with defaults, leaks=%+v", s.Leaks)
	}
}
