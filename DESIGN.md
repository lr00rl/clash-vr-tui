# Design

## Source of truth

- Status: Active
- Last refreshed: 2026-10-26
- Primary surfaces: interactive terminal UI over local terminal or SSH, scriptable CLI.
- Evidence: `README.md`, `docs/plans/2026-03-11-clash-vr-tui-design.md`, `assets/view-home.png`, `assets/view-config.png`, current Go UI, and the 2026-10-26 review in the project notes.

## Brand

- Personality: quiet network control desk with clear keyboard affordances.
- Trust signals: visible endpoint, selected route, data freshness, operation scope, and explicit failure.
- Avoid: decorative panels that hide state, color-only status, success-looking partial results.

## Product goals

- Goals: inspect mihomo, choose a proxy, diagnose connections or logs, and change runtime settings safely from an SSH session.
- Non-goals: full mihomo config editing, subscription management, desktop system proxy control.
- Success signals: core tasks work at 72 and 120 columns; failures and stale data are recognizable; a user can identify the next action without memorizing keys.

## Personas and jobs

- Primary persona: an operator at a keyboard in a local terminal or remote SSH pane.
- Jobs: check core health, choose a node, test it, inspect traffic, and recover from controller failure.
- Context: limited width, intermittent connection, no pointer, potentially sensitive network configuration.

## Information architecture

- Navigation: stable six-page sidebar and numbered jump keys. The current page and selected item remain visible.
- Screens: Home, Proxies, Connections, Rules, Logs, Config. Proxies adapts from group/node split view to group-then-node flow when space is insufficient.
- Hierarchy: current object and state first, primary action next, secondary controls and help last.

## Design principles

1. A value is shown with its source, measurement mode, and freshness whenever those can affect a decision.
2. Trigger, pending, success, partial, and failure are distinct states. An input acknowledgement is not a completed controller operation.
3. Consequential actions describe their scope before commitment and confirm the server state afterward.
4. Narrow layouts preserve the current task and its primary action before decorative or secondary text.
5. Escape cancels a pending choice or returns from node detail; global quit remains available through Ctrl+C.

## Visual language

- Palette: terminal background is transparent; text `#EAE1C8`, muted `#8F8978`, focus `#D8A657`, live information `#6AA3A0`, success `#80B85D`, danger `#D66A5A`. Existing border and warning colors remain supporting tokens.
- Type roles: terminal monospace only; bold for current page, selected object, action key, and state; regular for body; dim for secondary help. Do not rely on a font being installed.
- Layout rhythm: one-line top status, a compact sidebar, task content, and one-line contextual help. Content uses one blank line between sections and few boxes.
- Shape: text rows and light terminal borders; no simulated elevation.
- Motion: immediate text-state changes only, no decorative animation.
- Signature element: a compact freshness label beside live values, showing `LIVE`, `RECONNECTING`, or `STALE` and the last update time.

```text
120 columns                         72 columns
┌status + freshness─────────────┐   ┌status + freshness──────────┐
│sidebar│ current page          │   │nav│ current page           │
│       │ task content          │   │   │ task content           │
│       │ primary action        │   │   │ primary action         │
└context help───────────────────┘   └context help + ?───────────┘

Proxies wide: groups │ nodes + test state
Proxies narrow: groups -> Enter nodes -> Esc groups
```

## Components

- Reuse: `internal/ui/sidebar`, `statusbar`, `helpbar`, `overlay`, page models, and `internal/styles`.
- Change: contextual help selection, scrollable help, operation confirmation, stream and probe state labels.
- Variants: initial, live, pending, reconnecting, stale, empty, partial, error, confirmation.
- Ownership: colors and reusable text rendering live in `internal/styles`; page-specific state and copy remain with each page.

## Accessibility

- Target: all core tasks reachable by keyboard in a typical terminal; status and latency use text or symbols alongside color.
- Keyboard: visible cursor or selected row; Enter commits or opens; Esc backs out of an overlay, filter, or compact detail; Ctrl+C exits.
- Readability: never silently truncate the only error or the only path to a primary action. Long data can be abbreviated when a detail path exists.
- Screen readers: terminal support varies. Preserve plain-text CLI output and avoid conveying required meaning through ANSI styles alone.
- Reduced motion: no essential motion.

## Responsive behavior

- Contexts: 72x24, 80x24, 90x28, and 120x30 or larger SSH panes.
- Adaptation: keep navigation stable; show the primary page action in help; collapse Proxy split view when names and delay state no longer fit.
- Input: keyboard only. Resize keeps page, selection, filters, and current test mode.

## Interaction states

- Loading: identify the requested object or stream and show that work is pending.
- Empty: distinguish no data, no match, unavailable source, and skipped test.
- Error: show what failed, affected scope, and a retry or recovery step.
- Success: reflect controller-confirmed state rather than a submitted request.
- Disabled: show why an operation cannot start, including missing probe endpoints.
- Offline and slow network: retain last values only with age and a stale label.

## Content voice

- Tone: compact, operational, literal.
- Terminology: `group`, `node`, `controller`, `HTTP/TCP/ICMP`, `live`, `stale`, `skipped`, `timeout` have distinct meanings.
- Copy: explain impact before changes to LAN exposure or routing; include counts for partially completed tests.

## Implementation constraints

- Framework: Bubble Tea and Lip Gloss already in the repository. No new dependency for these changes.
- Performance: bounded probe concurrency and bounded log history; reconnect must not accumulate goroutines.
- Compatibility: Unix socket, Windows named pipe, loopback TCP, and SSH tunnel remain supported. Remote secret-bearing plaintext is rejected.
- Verification: unit and protocol-level tests plus a real terminal walkthrough at 72x24 and 120x30; exercise disconnected and connected states with a local mock controller when mihomo is unavailable.

## Open questions

- [ ] Validate compact Proxy navigation with a regular SSH user; the width threshold is a measured implementation choice, not a tested preference.
- [ ] Confirm whether future releases should support authenticated HTTPS controller endpoints directly or recommend SSH tunneling as the primary remote path.
