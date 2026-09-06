package evidence

import "testing"

func TestClassifyTarget(t *testing.T) {
	cases := []struct {
		s    string
		want Target
	}{
		{"127.0.0.1:5(claude.exe) --> claude.ai:443", TargetClaude},
		{"codex.exe --> www.codexapis.com:443", TargetCodex},
		{"--> chatgpt.com:443", TargetOpenAI},
		{"--> cursor.sh:443", TargetCursor},
		{"--> ipinfo.io:443", TargetProbe},
		{"hipsdaemon.exe --> api.anthropic.com:123", TargetClaude},
		{"chrome.exe --> baidu.com:443", TargetNone},
	}

	for _, c := range cases {
		if got := classifyTarget(c.s); got != c.want {
			t.Errorf("classifyTarget(%q)=%q want %q", c.s, got, c.want)
		}
	}
}

func TestHasErrorTokenIncludes403(t *testing.T) {
	if !hasErrorToken("error: can not connect remote err code: 403") {
		t.Fatal("403 must be a token")
	}
	if hasErrorToken("using ai-static[iproyal-static]") {
		t.Fatal("success line is not an error")
	}
}
