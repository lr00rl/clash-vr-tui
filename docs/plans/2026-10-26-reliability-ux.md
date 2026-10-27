# Reliability and terminal UX implementation plan

Date: 2026-10-26. Baseline: `9861484`. The 2026-10-26 review and `DESIGN.md` define the acceptance criteria.

## Checkpoint 1: truthful CLI and controller boundary

1. Give `status` and `nodes` explicit complete or failed outcomes. Reject malformed CLI options.
2. Validate connection configuration as a unit. Surface malformed YAML, avoid credential mixing, and make the TUI honor `test-url`.
3. Bound REST and WebSocket input, make WebSocket dialing cancellable, and reject secret-bearing non-loopback plaintext by default. Add a secret-file input using the standard library.
4. Sanitize controller-supplied terminal text at output boundaries and verify known escape sequences.
5. Make installer checksum verification mandatory and pin release actions if exact upstream SHAs can be checked.

Evidence gate: local controller mock covers partial HTTP failure, malicious text, authentication, and rejected transport. Unit tests, race tests, vet, and build pass.

## Checkpoint 2: live state and probe integrity

1. Track each stream as connecting, live, or reconnecting/stale with last event time and last error. Cancel old streams before redial.
2. Give delay results mode and request identity. Prevent old work overwriting new work; report skipped, failed, timeout, and succeeded nodes separately.
3. Show pending and completion for tests; do not silently finish a zero-node probe.

Evidence gate: simulated stream disconnect and out-of-order probe responses produce explicit stale/ignored states. Group probe partials stay visible.

## Checkpoint 3: terminal control flow

1. Confirm high-impact routing and LAN changes, then read the controller back.
2. Keep the active page action in narrow help, make Help content reachable, and adapt Proxies for narrow SSH panes.
3. Provide recovery guidance for offline states, clarify log pause behavior, and tighten Config's information column.

Evidence gate: interactive terminal walkthrough at 72x24 and 120x30 with normal, offline, loading, error, and dense data. Check all keys and rendered output.

## Documentation and stop condition

After each checkpoint, update the external project notes under `project_0009_clash-vr-tui` with implemented behavior, tests, and residual risk. Update README and `DESIGN.md` when user-facing behavior changes. Stop when all P0 and P1 review items either pass the evidence gate or have a concrete, documented blocker; keep P2 items only when they materially improve the terminal workflow.

## Status (2026-10-26)

All three checkpoints are implemented in the working tree on top of `9861484`; nothing is committed yet.

Checkpoint 1. `status` and `nodes` exit non-zero when a required endpoint fails; a controller that answers `/version` but returns 500 for `/configs`, `/connections`, and `/rules` now makes `status --json` exit 1 with the failing request named. Config loading rejects malformed YAML, unknown keys, `socket` with `server`, and a secret without an endpoint, and it picks the endpoint and secret from the same tier. `--secret-file`, `CLASH_VR_TUI_SECRET_FILE`, and `secret-file:` read the secret from a file. A secret aimed at a non-loopback TCP host is refused. REST bodies and WebSocket messages are size-bounded, WebSocket dialing honors context cancellation, controller text goes through `styles.SafeText`, the installer fails closed on checksum problems, and workflow actions are pinned to commits that match upstream release tags (checkout v6.1.0, setup-go v6.5.0, goreleaser-action v7.3.0). The TUI passes the configured `test-url` to the Proxies page, where it is the fallback for groups without their own `testUrl`.

Checkpoint 2. Each stream has a generation number; a reply from a replaced stream is dropped, the old stream is cancelled before redial, and the status bar, Home, and Logs show `STALE` with the last update time until a new stream delivers data. Delay results carry mode, group, node, and request id and are stored per mode. A superseded reply is dropped, every requested node gets `ok`, `timeout`, `skipped`, or `error`, rows show `testing` while a probe is in flight, and the header shows a count such as `HTTP Auto: 16/18 ok, 2 timeout`. `api.StatusError` and `probe.ErrTimeout` let a mihomo 504 or a network deadline read as a timeout instead of an error.

Checkpoint 3. Pages emit `messages.ConfigChangeMsg` instead of patching directly. The root model asks for `y` before enabling LAN, turning TUN off, or switching to global or direct; it then patches, reads `/configs` back, and flashes `Applied ...` or `Controller did not keep ...`. The help bar keeps the page's first two actions and `?` ahead of global keys, the help overlay scrolls, and Logs freeze the view while collection continues. Proxies switch to one panel at a time below 78 usable columns; panel boxes now fill their slots, which fixed rows wrapping at the 28-column group minimum. The status bar no longer wraps when a flash message is shown.

Evidence: `go test -race ./...`, `go vet ./...`, `gofmt -l .`, and `sh scripts/install_test.sh` pass. A tmux walkthrough at 72x24 and 120x30 against a local mock controller covered offline Home, Proxies in both layouts, group and node tests with timing-out nodes, the LAN confirmation (cancel sent no PATCH; confirm sent one and read back `ON`), the TUN confirmation, and a controller restart going `LIVE` to `STALE` and back. A node name containing OSC and CSI sequences rendered as plain text.

An independent review found no critical or high issues. Its three medium findings are fixed with tests: an empty or comment-only config file no longer aborts startup, the status bar stays on one line in narrow `STALE` panes (it drops `MEM`, then truncates), and CLI flag values may start with `-`.

Not verified: a real mihomo core, Windows named pipes, ICMP privileges on Linux, and the release workflow itself. Refusing plaintext secrets to non-loopback hosts is a breaking change for anyone who connected straight to a LAN controller with a secret; they now need an SSH tunnel. Direct HTTPS controller support remains an open question in `DESIGN.md`.
