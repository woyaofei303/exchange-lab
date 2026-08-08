# Exchange Lab Agent Rules

- Keep week scope explicit; do not add later-phase trading features early.
- Before adding a dependency, document its purpose, data ownership, failure mode, and rollback path in the change.
- Never use float64 as a ledger, price, quantity, margin, or PnL source of truth.
- Never log passwords, tokens, full DSNs, private keys, or raw user credentials.
- AI-generated code requires human review of ports, volumes, health checks, credentials, network exposure, timeouts, and error paths.
- AI tools remain read-only until a separately reviewed design authorizes a narrow write capability.
- Every behavior change starts with a failing test and ends with fresh verification evidence.
