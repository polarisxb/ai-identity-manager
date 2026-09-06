package identity

import (
	"regexp"
	"strings"
)

var (
	basicAuthPattern = regexp.MustCompile(`(?i)(Proxy-Authorization:\s+Basic\s+)[A-Za-z0-9+/=]+`)
	urlCredPattern   = regexp.MustCompile(`(?i)(https?://)[^/@\s]+:[^/@\s]+@`)
)

func Redact(text string, secrets []string) string {
	safe := text
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			continue
		}
		safe = strings.ReplaceAll(safe, secret, "<redacted-secret>")
	}
	safe = basicAuthPattern.ReplaceAllString(safe, "${1}<redacted-basic-auth>")
	safe = urlCredPattern.ReplaceAllString(safe, "${1}<redacted-credentials>@")
	return safe
}

func (c Config) SecretValues() []string {
	return []string{c.IPRoyal.Username, c.IPRoyal.Password, c.Clash.ControllerSecret}
}
