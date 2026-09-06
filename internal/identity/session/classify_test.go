package session

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		host string
		proc string
		want Target
	}{
		{"api.anthropic.com", "claude.exe", TargetClaude},
		{"claude.ai", "", TargetClaude},
		{"", `C:\x\claude.exe`, TargetClaude},
		{"browser-intake-us5-datadoghq.com", "claude.exe", TargetClaude},
		{"www.codexapis.com", "", TargetCodex},
		{"", "codex.exe", TargetCodex},
		{"chatgpt.com", "", TargetOpenAI},
		{"", "ChatGPT.exe", TargetOpenAI},
		{"cursor.sh", "", TargetCursor},
		{"ipinfo.io", "chrome.exe", TargetProbe},
		{"ping0.cc", "", TargetProbe},
		{"baidu.com", "wechat.exe", TargetNone},
		{"", "", TargetNone},
	}
	for _, c := range cases {
		if got := Classify(c.host, c.proc); got != c.want {
			t.Errorf("Classify(%q,%q)=%q want %q", c.host, c.proc, got, c.want)
		}
	}
}
