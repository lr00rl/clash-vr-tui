# clash-vr-tui

A terminal UI (and CLI) for the **mihomo** core behind **Clash Verge Rev** — built
for driving your proxy from a pure terminal, including over SSH on a remote
Linux/macOS box. It talks to the mihomo controller over the Unix socket
(`/tmp/verge/verge-mihomo.sock`) or a TCP external-controller, and ships as a
single static binary with no runtime dependencies.

```
  Clash Verge TUI                                  ▲ 24.0 KB/s  ▼ 3.8 KB/s  MEM 74M
                ╭─ Core Status ──────────────────────────────────────────────────╮
  ❯ Home        │  Status         ● connected                                     │
    Proxy       │  Core Version   v1.19.26                                        │
    Conns       │  Mode           rule                                            │
    Rules       │  Mixed Port     7897                                            │
    Logs        │  Memory         74.3 MB                                         │
    About       │  Active Conns   67                                             │
                ╰────────────────────────────────────────────────────────────────╯
```

## Features

- **Home** — core status, version, mode, ports, live memory and active-connection
  count, restart the core (`R`).
- **Proxies** — two-panel groups / nodes view for 140+ node groups:
  - select a node (`Enter`), color-coded delays, sort by default/name/delay (`o`)
  - **three latency test modes** cycled with `T`:
    - **HTTP** — mihomo's delay test through the tunnel (per-group test URL)
    - **TCP** — raw connect to the node's `server:port`
    - **ICMP** — echo round-trip to the node's server
  - delay-aware filtering: `delay<200`, `delay>500`, `delay=timeout`, or a name
    substring (`/`)
  - unpin a URLTest/Fallback group back to auto (`u`)
- **Connections** — live connection table with per-connection speed, sort, filter,
  close one (`x`) / all (`X`), and a detail view (`Enter`).
- **Rules** — searchable routing rules.
- **Logs** — live `WS /logs` stream with level filter (`l`), text filter (`/`),
  pause (`space`), and clear (`c`).

> **Note on TCP/ICMP:** the mihomo API does not expose node server addresses, so
> these modes parse the running mihomo config to find each `server:port`. They
> depend on the box being able to resolve the server hostname and the server
> answering — many obfuscated/CDN nodes only resolve through the tunnel, so the
> **HTTP** delay test remains the most reliable signal.

## Install

```sh
make build          # -> ./clash-vr-tui
make install        # -> /usr/local/bin/clash-vr-tui (PREFIX overridable)
make dist           # cross-compile release binaries into dist/
```

Requires Go 1.24+.

## Usage

Launch the interactive TUI:

```sh
clash-vr-tui                          # auto-detects the verge socket
clash-vr-tui --socket /path/to.sock
clash-vr-tui --server 127.0.0.1:9090 --secret mypw   # external controller
```

### CLI (scriptable, no TUI)

```sh
clash-vr-tui status [--json]              # version, mode, ports, memory, conns
clash-vr-tui proxies                      # groups with their current node
clash-vr-tui nodes <group>                # nodes in a group with last delay
clash-vr-tui switch <group> <node>        # select a node
clash-vr-tui test <group> [--timeout MS]  # HTTP delay-test, sorted
clash-vr-tui mode [rule|global|direct]    # get or set the proxy mode
clash-vr-tui restart                      # restart the core
clash-vr-tui conns [--json]               # active connections
```

Example:

```sh
clash-vr-tui test for-test-ip --timeout 3000
clash-vr-tui switch for-test-ip 'JP-Tokyo-01'
clash-vr-tui status --json | jq .mode
```

## Keybindings

| Scope | Keys |
|-------|------|
| Global | `Tab`/`Shift+Tab` page · `1`–`6` jump · `?` help · `r` refresh · `R` restart core · `q`/`Ctrl+C` quit |
| Proxies | `←→`/`hl` panel · `Enter` select · `d`/`D` test · `T` test mode · `o` sort · `u` unpin · `/` filter |
| Connections | `Enter` detail · `x` close · `X` close all · `s` sort · `/` filter |
| Rules | `g`/`G` top/bottom · `/` filter |
| Logs | `space` pause · `l` level · `c` clear · `/` filter |

While a filter input is open, every key (including `q`) types into it; `Esc`
closes it. `Ctrl+C` always quits.

## Configuration

Connection settings resolve from **flags > env > config file > defaults**:

- env: `CLASH_VR_TUI_SOCKET`, `CLASH_VR_TUI_SERVER`, `CLASH_VR_TUI_SECRET`
- file: `~/.config/clash-vr-tui/config.yaml`

```yaml
# ~/.config/clash-vr-tui/config.yaml
socket: /tmp/verge/verge-mihomo.sock   # or use an external controller:
# server: 127.0.0.1:9090
# secret: ""
config-path: ""    # explicit mihomo config path for TCP/ICMP server lookup
test-url: ""       # default delay-test URL
```

## Architecture

Go + [bubbletea](https://github.com/charmbracelet/bubbletea) (Elm architecture)
with lipgloss styling. REST + WebSocket over a Unix-socket or TCP transport.
See `docs/plans/` for the design notes.
