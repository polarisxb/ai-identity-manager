package evidence

import "strings"

func classifyTarget(lower string) Target {
	lower = strings.ToLower(lower)
	switch {
	case has(lower, "claude.exe") || has(lower, "claude.ai") || has(lower, "anthropic."):
		return TargetClaude
	case has(lower, "codex.exe") || has(lower, "codexapis.com"):
		return TargetCodex
	case has(lower, "chatgpt.exe") || has(lower, "openai.exe") || has(lower, "openai.com") || has(lower, "chatgpt.com") || has(lower, "oaistatic.com") || has(lower, "oaiusercontent.com"):
		return TargetOpenAI
	case has(lower, "cursor.exe") || has(lower, "cursor.com") || has(lower, "cursor.sh"):
		return TargetCursor
	case has(lower, "ipinfo.io") || has(lower, "ping0.cc"):
		return TargetProbe
	default:
		return TargetNone
	}
}

func transportOf(lower string) Transport {
	switch {
	case strings.Contains(lower, "[udp]"):
		return TransportUDP
	case strings.Contains(lower, "[tcp]"):
		return TransportTCP
	default:
		return TransportUnknown
	}
}

func businessPort(lower string) bool {
	return strings.Contains(lower, ":443") || strings.Contains(lower, ":8443")
}

func hasErrorToken(lower string) bool {
	for _, token := range []string{"err code: 403", "can not connect remote", "connection reset", "broken pipe", "i/o timeout", "timeout", "502", "bad gateway", "refused", "10054", "10060"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return strings.Contains(lower, " eof")
}

func classifyErrorSource(lower string) ErrorSource {
	switch {
	case strings.Contains(lower, "iproyal"):
		return ErrIPRoyal
	case strings.Contains(lower, "err code: 403") && strings.Contains(lower, "ai-static"):
		return ErrIPRoyal
	case strings.Contains(lower, "jms"):
		return ErrJMS
	case strings.Contains(lower, "proxies"):
		return ErrOrdinaryProxy
	case strings.Contains(lower, "claude") || strings.Contains(lower, "anthropic") || strings.Contains(lower, "openai") || strings.Contains(lower, "chatgpt") || strings.Contains(lower, "codex") || strings.Contains(lower, "cursor"):
		return ErrAIBusiness
	default:
		return ErrUnknown
	}
}

func has(s, sub string) bool {
	return sub != "" && strings.Contains(s, sub)
}
