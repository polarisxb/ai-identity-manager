package session

import "strings"

type Target string

const (
	TargetClaude Target = "claude"
	TargetOpenAI Target = "openai"
	TargetCodex  Target = "codex"
	TargetCursor Target = "cursor"
	TargetProbe  Target = "probe"
	TargetNone   Target = ""
)

func Classify(host, process string) Target {
	h, p := strings.ToLower(host), strings.ToLower(process)
	switch {
	case in(p, "claude.exe") || in(h, "claude.ai") || in(h, "anthropic."):
		return TargetClaude
	case in(p, "codex.exe") || in(h, "codexapis.com"):
		return TargetCodex
	case in(p, "chatgpt.exe") || in(p, "openai.exe") || in(h, "openai.com") || in(h, "chatgpt.com") || in(h, "oaistatic.com") || in(h, "oaiusercontent.com"):
		return TargetOpenAI
	case in(p, "cursor.exe") || in(h, "cursor.com") || in(h, "cursor.sh"):
		return TargetCursor
	case in(h, "ipinfo.io") || in(h, "ping0.cc"):
		return TargetProbe
	default:
		return TargetNone
	}
}

func in(s, sub string) bool {
	return sub != "" && strings.Contains(s, sub)
}
