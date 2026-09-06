# Legacy Archive 2026-06-17

This archive contains old runtime artifacts and temporary identity-keeper integration scripts from the pre-MVP proxy approach.

Kept in project root:

- `proxy-secret.local` stays in root and is not archived.
- `scripts/install-clash-iproyal-chain.ps1` stays as the verified Clash chain reference.
- `scripts/capture-iproyal-failure.ps1` and `scripts/capture-iproyal-long-tunnel.ps1` stay as diagnostic references.
- `cmd/identity-keeper/` and `internal/keeper/` are intentionally not moved yet because Go will still discover archived `.go` files under this module. They are documented as legacy and should not receive new feature work.

Archived by move:

- Browser profile/cache directories.
- Probe and run logs.
- Old identity-keeper executable and PAC file.
- Old keeper startup, PAC, browser-launch, and Clash identity scripts.

Nothing in this archive should be used as the main product path. The main product is `cmd/ai-identity-manager`.
