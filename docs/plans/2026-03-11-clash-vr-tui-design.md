# Clash Verge TUI - Design Document

Date: 2026-03-11

## Overview

A terminal UI for controlling Clash Verge Rev's mihomo core via Unix socket,
built in Go with bubbletea (Elm architecture).

**Communication**: All API calls go through the mihomo Unix socket at
`/tmp/verge/verge-mihomo.sock` (macOS/Linux) or `\\.\pipe\verge-mihomo`
(Windows). Real-time data (traffic, connections) via WebSocket over the same
socket.

## Tech Stack

| Component | Choice |
|-----------|--------|
| Language | Go |
| TUI framework | bubbletea (Elm architecture) |
| Styling | lipgloss |
| Components | bubbles (list, table, textinput, viewport, etc.) |
| HTTP client | net/http with Unix socket dialer |
| WebSocket | gorilla/websocket with Unix socket dialer |

## Architecture: Single Model Composition (Option A)

```
RootModel
├── sidebar       (navigation, fixed 12-col width)
├── statusbar     (top: title + realtime traffic speed)
├── helpbar       (bottom: context-sensitive keybindings)
├── home          (sub-model)
├── proxies       (sub-model)
├── connections   (sub-model)
├── rules         (sub-model)
├── settings      (sub-model)
└── client        (shared mihomo API client)
```

All sub-models share the `client` for API access. The RootModel delegates
`Update()` to the active page's sub-model. WebSocket streams (traffic,
connections) are managed at the root level and dispatched as tea.Msg to
relevant sub-models.

## Layout

```
┌──────────┬────────────────────────────────────────────────┐
│          │  Clash Verge TUI            ▲ 12.3 KB/s ▼ 1.2 │
│ ❯ Home   ├────────────────────────────────────────────────┤
│   Proxies│                                                │
│   Conns  │           (Active Page Content)                │
│   Rules  │                                                │
│   Settings                                                │
│          │                                                │
├──────────┴────────────────────────────────────────────────┤
│ q:Quit  Tab:Next  /:Search  ?:Help  r:Refresh            │
└───────────────────────────────────────────────────────────┘
```

- **Left sidebar**: Fixed 12 columns. j/k or arrow keys to navigate, Enter to
  switch. Current page highlighted.
- **Right content**: Fills remaining width. Renders active page sub-model.
- **Top status bar**: Title + real-time upload/download speed from WS /traffic.
- **Bottom help bar**: Context-sensitive shortcuts for current page.

## Global Keybindings

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Next / previous page |
| `1`-`5` | Jump to page by number |
| `q` / `Ctrl+C` | Quit |
| `?` | Help overlay |
| `r` | Refresh current page data |

## Page Designs

### 1. Home

```
┌─ Quick Controls ─────────────────────────────┐
│ System Proxy [ON ]  TUN [OFF]  Mode [Rule]   │
└──────────────────────────────────────────────┘
┌─ Clash Info ─────────────────────────────────┐
│ Core Version   v1.18.10                      │
│ Proxy Addr     127.0.0.1:7897                │
│ Mixed Port     7897                          │
│ Uptime         2h 15m 30s                    │
│ Rules Count    1,234                         │
└──────────────────────────────────────────────┘
┌─ System Info ────────────────────────────────┐
│ OS             macOS 15.5 (darwin/arm64)     │
│ App Version    2.2.7                         │
└──────────────────────────────────────────────┘
```

**Shortcuts**: `s` toggle system proxy, `t` toggle TUN, `m` cycle mode
(rule -> global -> direct -> rule).

**Data sources**:
- Quick Controls: `GET /configs` (mode), `PATCH /configs` (toggle TUN),
  system proxy via verge config
- Clash Info: `GET /version`, `GET /configs` (mixed-port), `GET /rules`
  (count), uptime calculated from process start
- System Info: Go `runtime.GOOS`/`runtime.GOARCH`, app version from build var

### 2. Proxies

```
Mode: [Rule]  [Global]  [Direct]
──────────────────────────────────────────────
▼ Proxy (Selector) 12 nodes → JP-Tokyo-01          [Sort: Delay ↑]
│  ❯ HK-IPLC-03          28ms
│    JP-Tokyo-01          45ms
│    SG-Direct-01         95ms
│    JP-Osaka-02          120ms
│    US-SanJose-01        180ms
│    BR-Node-01           timeout
▶ Auto (URLTest) 8 nodes → HK-IPLC-03
▶ Streaming (Selector) 5 nodes → JP-Tokyo-01
▶ AI (Selector) 3 nodes → US-SanJose-01
```

**Shortcuts**: `Space` expand/collapse group, `Enter` select node,
`d` delay test group, `D` delay test single node, `o` cycle sort
(default -> name -> delay), `/` filter.

**Sort modes** (global, applies to all expanded groups):
- Default: original config order
- By Name: alphabetical A-Z
- By Delay: ascending, timeout/untested last

**Delay colors**: green <200ms, yellow 200-500ms, red >=500ms, gray timeout.

**Data sources**: `GET /group` (list), `PUT /proxies/{group}` (select),
`GET /group/{name}/delay` (test), `GET /proxies/{name}/delay` (single test).

### 3. Connections

```
Connections (42 active)    Total ▼128M ▲12M
Filter: _
──────────────────────────────────────────────
Host           DL    UL  Chain      Rule
──────────────────────────────────────────────
❯google.com:443  1.2K  340  JP-01/Pro  DOMAIN
 github.com:443  800   120  US-01/Pro  DOMAIN-S
 cdn.npm.com:443 3.4K  56   DIRECT     GEOSITE
```

**Columns**: Host, DL speed, UL speed, Chain, Rule.

**Shortcuts**: `/` filter (host, process, destination IP), `Enter` detail
overlay (all fields), `x` close connection, `X` close all (with confirm),
`s` cycle sort (time -> DL speed -> UL speed).

**Data source**: `WS /connections` (real-time push).

**Detail overlay fields**: host, source, destination, process, processPath,
type, network, chains, rule, rulePayload, upload total, download total,
start time.

### 4. Rules

```
Rules (1,234)   Filter: _
──────────────────────────────────────────────
#    Type            Payload        Proxy
──────────────────────────────────────────────
1    DOMAIN-SUFFIX   google.com     Proxy
2    DOMAIN-SUFFIX   github.com     Proxy
3    DOMAIN-KEYWORD  openai         AI
...
1234 MATCH                          Proxy
```

**Columns**: #, Type, Payload, Proxy.

**Shortcuts**: `/` filter (payload, type), `g` top, `G` bottom.

**Data source**: `GET /rules` (read-only).

### 5. Settings

```
┌─ System ────────────────────────────────────┐
│ TUN Mode          [OFF]                     │
│ System Proxy      [ON ]                     │
│ Allow LAN         [OFF]                     │
└─────────────────────────────────────────────┘
┌─ Info ──────────────────────────────────────┐
│ Config Dir   ~/.config/clash-verge-rev      │
│ Core Dir     ~/.local/share/clash-verge..   │
│ Log Level    info                           │
└─────────────────────────────────────────────┘
```

**Shortcuts**: `Enter`/`Space` toggle switches, `Enter` on Log Level opens
select menu (debug / info / warning / error / silent).

**Data sources**: `GET /configs`, `PATCH /configs` for toggles.

## Data Flow

```
                    ┌─────────────────┐
                    │   mihomo core   │
                    │  (Unix socket)  │
                    └────┬───────┬────┘
                         │ REST  │ WS
                    ┌────┴───────┴────┐
                    │  API Client     │
                    │  (shared)       │
                    └────┬───────┬────┘
                         │       │
              tea.Cmd ───┘       └─── tea.Msg
                         │       │
                    ┌────┴───────┴────┐
                    │   RootModel     │
                    │   .Update()     │
                    └────┬────────────┘
                         │ delegates
            ┌────────────┼────────────┐
            ▼            ▼            ▼
       HomeModel    ProxiesModel  ConnectionsModel ...
```

1. Sub-models return `tea.Cmd` that call API client methods.
2. API client methods return `tea.Msg` with response data.
3. RootModel dispatches messages to the active sub-model.
4. WebSocket streams are started as long-running `tea.Cmd` at root level,
   producing `TrafficMsg` and `ConnectionsMsg` continuously.

## Project Structure

```
clash-vr-tui/
├── main.go                  # entry point
├── go.mod
├── internal/
│   ├── app/
│   │   ├── model.go         # RootModel (composes all sub-models)
│   │   ├── update.go        # root Update logic + message routing
│   │   ├── view.go          # root View (sidebar + content + bars)
│   │   └── keys.go          # global keybindings
│   ├── ui/
│   │   ├── sidebar/
│   │   │   └── sidebar.go   # sidebar navigation component
│   │   ├── statusbar/
│   │   │   └── statusbar.go # top bar with traffic speed
│   │   ├── helpbar/
│   │   │   └── helpbar.go   # bottom contextual help
│   │   ├── home/
│   │   │   └── home.go      # home page sub-model
│   │   ├── proxies/
│   │   │   └── proxies.go   # proxies page sub-model
│   │   ├── connections/
│   │   │   └── connections.go
│   │   ├── rules/
│   │   │   └── rules.go
│   │   ├── settings/
│   │   │   └── settings.go
│   │   └── overlay/
│   │       └── overlay.go   # help/detail/confirm overlays
│   ├── api/
│   │   ├── client.go        # HTTP client (Unix socket dialer)
│   │   ├── ws.go            # WebSocket client (traffic, connections)
│   │   ├── proxies.go       # proxy/group API methods
│   │   ├── connections.go   # connections API methods
│   │   ├── configs.go       # config API methods
│   │   ├── rules.go         # rules API methods
│   │   └── types.go         # shared response types
│   ├── messages/
│   │   └── messages.go      # all tea.Msg type definitions
│   └── styles/
│       └── styles.go        # lipgloss style constants
└── docs/
    └── plans/
        └── 2026-03-11-clash-vr-tui-design.md
```

## Key Technical Decisions

1. **Unix socket HTTP**: Use `net.Dial("unix", socketPath)` as custom
   `http.Transport.DialContext`. All REST calls go to `http://localhost/...`
   over the socket.

2. **WebSocket over Unix socket**: Use `gorilla/websocket.Dialer` with
   custom `NetDialContext` pointing to the socket. Connect to
   `ws://localhost/traffic` and `ws://localhost/connections`.

3. **Concurrent data refresh**: WebSocket streams run as background tea.Cmd.
   REST fetches are fire-and-forget tea.Cmd returning tea.Msg on completion.

4. **No config file for v1**: Socket path auto-detected. Future versions
   can add `~/.config/clash-vr-tui/config.yaml`.

5. **Build-time version**: Inject version via `go build -ldflags`.
