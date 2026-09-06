package identity

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type AuditEvent struct {
	Time    time.Time `json:"time"`
	Type    string    `json:"type"`
	Summary string    `json:"summary"`
}

func AppendAudit(path string, ev AuditEvent, secrets []string) error {
	ev.Summary = Redact(ev.Summary, secrets)
	if ev.Time.IsZero() {
		ev.Time = time.Now()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}

func LoadRecentAudit(path string, n int) ([]AuditEvent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var all []AuditEvent
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev AuditEvent
		if json.Unmarshal([]byte(line), &ev) == nil {
			all = append(all, ev)
		}
	}
	if n > 0 && len(all) > n {
		all = all[len(all)-n:]
	}
	return all, nil
}
