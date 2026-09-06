# Retired scripts

## monitor-ai-identity.ps1 — retired 2026-06-22

The original read-only AI-identity / stability monitoring scaffold (PowerShell).
**Superseded by the Go `monitor` command** (`ai-identity-manager monitor --once|--watch`),
which covers everything it did and more:

- route integrity (`all_static`) via the mihomo `/connections` over the named pipe
- exit-IP verification through the mixed port (with HTTP-status/429 robustness)
- connection-drop detection
- service-log AI chain-error scanning (403 / reset / EOF / timeout / …)
- plus reason-code judgment + per-target evidence the PS version lacked

Validated by a clean multi-day real-machine canary (`all_static=YES` throughout, no leaks).

Kept here for reference only — **do not run as part of normal operation.**
The Go monitor is the supported tool.
