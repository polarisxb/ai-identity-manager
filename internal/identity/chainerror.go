package identity

import (
	"strings"
	"time"
)

type ChainError struct {
	Source ErrorSource
	When   time.Time
	Line   string
}

func ScanChainErrors(text string, cfg Config) []ChainError {
	var out []ChainError
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "level=warning") && !strings.Contains(lower, "level=error") {
			continue
		}
		if !mentionsAITarget(lower) || !hasChainErrorToken(lower) {
			continue
		}
		out = append(out, ChainError{
			Source: classifyChainErrorSource(lower),
			When:   parseLogTimestamp(line),
			Line:   line,
		})
	}
	return out
}

func mentionsAITarget(lower string) bool {
	for _, target := range []string{"claude", "anthropic", "openai", "chatgpt", "codex", "cursor", "ai-static", "iproyal-static"} {
		if strings.Contains(lower, target) {
			return true
		}
	}
	return false
}

func hasChainErrorToken(lower string) bool {
	for _, token := range []string{"err code: 403", "can not connect remote", "connection reset", "broken pipe", "i/o timeout", "502 bad gateway", "connection refused", "10054", "10060"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return strings.Contains(lower, " eof") || strings.HasSuffix(lower, "eof")
}

func classifyChainErrorSource(lower string) ErrorSource {
	switch {
	case strings.Contains(lower, "iproyal"):
		return ErrorIPRoyal
	case strings.Contains(lower, "err code: 403") && strings.Contains(lower, "ai-static"):
		return ErrorIPRoyal
	case strings.Contains(lower, "jms"):
		return ErrorJMS
	case strings.Contains(lower, "claude") || strings.Contains(lower, "anthropic") ||
		strings.Contains(lower, "openai") || strings.Contains(lower, "chatgpt") ||
		strings.Contains(lower, "codex") || strings.Contains(lower, "cursor"):
		return ErrorClaudeBusiness
	default:
		return ErrorUnknown
	}
}
