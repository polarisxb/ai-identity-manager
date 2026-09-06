package identity

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type verifyStateFile struct {
	CheckedAt       time.Time `json:"checked_at"`
	Checked         bool      `json:"checked"`
	ExitIP          string    `json:"exit_ip"`
	ASN             string    `json:"asn"`
	Country         string    `json:"country"`
	ExpectedExitIP  string    `json:"expected_exit_ip"`
	ExpectedASN     string    `json:"expected_asn"`
	ExpectedCountry string    `json:"expected_country"`
	Error           string    `json:"error"`
}

func SaveVerifyResult(path string, result VerifyResult, checkedAt time.Time) error {
	if path == "" {
		return nil
	}
	if checkedAt.IsZero() {
		checkedAt = time.Now()
	}
	state := verifyStateFile{
		CheckedAt:       checkedAt,
		Checked:         result.Checked,
		ExitIP:          result.ExitIP,
		ASN:             result.ASN,
		Country:         result.Country,
		ExpectedExitIP:  result.ExpectedExitIP,
		ExpectedASN:     result.ExpectedASN,
		ExpectedCountry: result.ExpectedCountry,
		Error:           result.Error,
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func LoadVerifyResult(path string) (VerifyResult, error) {
	if path == "" {
		return VerifyResult{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return VerifyResult{}, nil
		}
		return VerifyResult{}, err
	}
	var state verifyStateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{
		CheckedAt:       state.CheckedAt,
		Checked:         state.Checked,
		ExitIP:          state.ExitIP,
		ASN:             state.ASN,
		Country:         state.Country,
		ExpectedExitIP:  state.ExpectedExitIP,
		ExpectedASN:     state.ExpectedASN,
		ExpectedCountry: state.ExpectedCountry,
		Error:           state.Error,
	}, nil
}

type Activity struct {
	LastApplyAt  time.Time `json:"last_apply_at"`
	LastReloadAt time.Time `json:"last_reload_at"`
}

func LoadActivity(path string) (Activity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Activity{}, nil
		}
		return Activity{}, err
	}
	var a Activity
	if err := json.Unmarshal(data, &a); err != nil {
		return Activity{}, err
	}
	return a, nil
}

func saveActivity(path string, a Activity) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func RecordApply(path string, at time.Time) error {
	a, err := LoadActivity(path)
	if err != nil {
		return err
	}
	a.LastApplyAt = at
	return saveActivity(path, a)
}

func RecordReload(path string, at time.Time) error {
	a, err := LoadActivity(path)
	if err != nil {
		return err
	}
	a.LastReloadAt = at
	return saveActivity(path, a)
}
