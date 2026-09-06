package identity

import "testing"

func TestScanChainErrorsDetects403AsIPRoyal(t *testing.T) {
	line := `[2026-06-18 20:47:34] time="2026-06-18T20:47:34+08:00" level=warning msg="[TCP] dial AI-Static (match ProcessName/claude.exe) 10.0.0.2:54644(claude.exe) --> api.anthropic.com:443 error: can not connect remote err code: 403"`
	errs := ScanChainErrors(line, DefaultConfig())
	if len(errs) != 1 || errs[0].Source != ErrorIPRoyal {
		t.Fatalf("errs = %+v", errs)
	}
}

func TestScanChainErrorsIgnoresInfoSuccess(t *testing.T) {
	line := `time="2026-06-18T20:00:00+08:00" level=info msg="[TCP] 127.0.0.1:50283(claude.exe) --> claude.ai:443 match ProcessName(claude.exe) using AI-Static[IPRoyal-Static]"`
	if errs := ScanChainErrors(line, DefaultConfig()); len(errs) != 0 {
		t.Fatalf("info line should not be a chain error: %+v", errs)
	}
}

func TestScanChainErrorsClassifiesJMS(t *testing.T) {
	line := `level=warning msg="[TCP] dial Proxies (claude.exe) --> api.anthropic.com:443 error: dial tcp jms host:16975: i/o timeout"`
	errs := ScanChainErrors(line, DefaultConfig())
	if len(errs) != 1 || errs[0].Source != ErrorJMS {
		t.Fatalf("errs = %+v", errs)
	}
}
