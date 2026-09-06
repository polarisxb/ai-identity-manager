# Monitor Usage

`monitor` checks two things:

- routing: AI traffic is on `AI-Static` and `IPRoyal-Static`
- exit identity: observed IP/ASN/country match the configured `EXPECTED_*` values, or remain stable during a watch session

The controller secret comes from `CONTROLLER_SECRET` in `proxy-secret.local`. Do not put the real value in checked-in config.

## Once

```powershell
.\ai-identity-manager.exe monitor --once --secret proxy-secret.local
```

`--once` reads one controller snapshot and takes one exit identity sample.

## Watch

```powershell
.\ai-identity-manager.exe monitor --watch --secret proxy-secret.local --interval 5s --ip-interval 90s
```

`--watch` keeps polling controller connections. It samples the exit identity on `--ip-interval` and immediately samples again when an AI connection disappears between snapshots.
It also scans Mihomo logs for AI chain errors and prints new diagnostics as `chain_error source=... line=...`.
These chain errors are diagnostic side-car evidence: they are counted and displayed, but they do not change the monitor identity level or exit code.

Use `--duration` for a bounded run:

```powershell
.\ai-identity-manager.exe monitor --watch --duration 10m --secret proxy-secret.local
```

## Reload

```powershell
.\ai-identity-manager.exe reload --secret proxy-secret.local
```

`reload` asks the controller to reload the runtime config, then reads live connections and verifies AI traffic still resolves to `AI-Static`.
It does not restart the kernel, rewrite `clash-verge.yaml`, change identity, write audit records, or sample the exit IP.

If reload reports stale routing, re-apply the profile in Clash Verge Rev and then run `monitor` again.

## KillSwitch

```powershell
.\ai-identity-manager.exe killswitch status --secret proxy-secret.local
.\ai-identity-manager.exe killswitch apply --secret proxy-secret.local
.\ai-identity-manager.exe killswitch remove --secret proxy-secret.local
```

`killswitch` manages Windows Firewall outbound UDP block rules for configured AI desktop apps.
It is intended to block STUN/WebRTC and QUIC leaks from desktop apps such as Claude Desktop.

`killswitch apply` and `killswitch remove` require an elevated shell. If Windows reports access denied, run the command again as administrator.
`killswitch status` and the `doctor` check are read-only.

This phase only blocks UDP. It does not block all direct TCP traffic; TCP fail-closed routing should be handled by Mihomo TUN strict-route with no DIRECT fallback.
Browser-based Claude is also out of scope because Chrome and Edge share browser processes; use browser WebRTC policy for that case.

## Exit Codes

- `0`: routing is static and exit identity is verified.
- `1`: routing is static, but identity is unverifiable or the exit sample failed.
- `2`: routing leak, exit identity mismatch/change, missing secret, or controller failure.

`--watch` prints `chain_errors=<n>` in `session_summary`. This count is diagnostic only.

For `reload`, exit codes mean:

- `0`: reload completed and live AI connections are on `AI-Static`.
- `1`: reload completed, but no live AI connections were available to verify.
- `2`: controller/reload failure, missing controller secret, or stale AI routing after reload.

The older PowerShell monitor script `scripts/monitor-ai-identity.ps1` can be retired once this Go monitor is in use: Go monitor now covers routing, exit IP identity, disconnect-triggered resampling, and chain error evidence.
