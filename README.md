# clash-vr-tui

[![CI](https://github.com/lr00rl/clash-vr-tui/actions/workflows/ci.yml/badge.svg)](https://github.com/lr00rl/clash-vr-tui/actions/workflows/ci.yml)
[![Release](https://github.com/lr00rl/clash-vr-tui/actions/workflows/release.yml/badge.svg)](https://github.com/lr00rl/clash-vr-tui/actions/workflows/release.yml)

A terminal UI and command-line tool for the **mihomo** core behind
**Clash Verge Rev**, or a standalone mihomo service. It finds the running
controller by itself, gives every action a scriptable subcommand, streams core
logs, and has a `doctor` that checks your setup end to end and tells you what to
fix. It works the same over SSH, and ships as one static binary.

<p>
  <img src="assets/view-home.png" alt="Home view showing mihomo runtime status" width="900">
</p>

<p>
  <img src="assets/view-config.png" alt="Runtime config view showing toggles and editable settings" width="900">
</p>

## Quickstart

```sh
curl -fsSL https://raw.githubusercontent.com/lr00rl/clash-vr-tui/main/scripts/install.sh | sh

clash-vr-tui              # interactive TUI
clash-vr-tui status       # one-shot status, including which controller was used
clash-vr-tui doctor       # check the whole setup and explain problems
```

No flags are needed when it runs on the same machine as the core: it looks at
running core processes, their config files, and the Clash Verge Rev paths, and
uses the first controller that answers. `clash-vr-tui discover` shows what it
found. See [Connecting](#connecting) for remote cores and explicit endpoints.

## Daily use from the shell

Everything the TUI can do is also a subcommand, so you never need the TUI on a
server or in a script.

| | Command | What it does |
|---|---|---|
| Find | `discover` (`list-socket`) | Controllers on this machine, their state, and which one is used |
| Inspect | `status` | Core version, endpoint, mode, ports, memory, connections |
| | `proxies`, `nodes <group>` | Groups with their current node; nodes with last delay |
| | `conns` | Active connections |
| | `rules [--grep S]` | Routing rules |
| | `providers` | Subscriptions (node count, usage, expiry) and rule sets |
| | `config` | Runtime settings |
| Live | `logs [--level L] [--grep S] [--for DUR]` | Core log stream |
| Control | `switch <group> <node>` | Select a node |
| | `test <group> [--timeout MS]` | HTTP delay test, sorted |
| | `unpin <group>` | Return a URLTest/Fallback group to automatic selection |
| | `mode [rule\|global\|direct]` | Get or set the proxy mode |
| | `config set tun\|lan on\|off`, `config set log-level L` | Change a setting and confirm the core kept it |
| | `providers update [name...]` | Re-fetch providers (every HTTP one by default) |
| | `close <id>... \| --all \| --host S` | Close connections; `--host` matches the host and its subdomains |
| | `flush dns\|fakeip` | Clear the core's caches |
| | `restart` | Restart the core |
| Diagnose | `doctor` (`autodebug`) | End-to-end check with next steps |

`clash-vr-tui help` prints the same list with every flag.

### Logs

```sh
clash-vr-tui logs                                  # info and above, until Ctrl+C
clash-vr-tui logs --level warning --grep timeout   # only matching warnings and errors
clash-vr-tui logs --level debug --grep example.com # which rule and node a site uses
clash-vr-tui logs --json --for 1m | jq -r 'select(.level=="error") | .payload'
```

`logs` reads the core's `WS /logs` stream and reconnects if the core restarts.
`--level` is debug, info, warning, or error; mihomo logs the rule and outbound
of each connection at debug. Subscription URLs in log lines are shortened to
scheme and host so tokens do not end up in your terminal or files.

### Doctor

`doctor` (alias `autodebug`) checks the path from controller to internet and
prints one finding per problem, each with the next step:

- the controller and how it was found, and whether a second core is running;
- runtime settings that commonly cause trouble: direct mode, LAN exposure
  without authentication, debug logging, TUN off with no system proxy;
- the system proxy, including the PAC URL Clash Verge Rev serves on loopback;
- a real request through the mixed port, and DNS resolution through the core;
- groups whose selected node failed its last test (`--probe` runs fresh tests);
- subscriptions that are expired, close to expiry, or near their traffic quota;
- connections that fell through to the final `MATCH` rule, which are
  candidates for new rules, plus top hosts and exit nodes by traffic;
- warnings and errors in a short log capture, grouped by pattern;
- recent errors in the Clash Verge Rev logs, and other Clash clients running at
  the same time.

```text
$ clash-vr-tui doctor
...
Runtime config
  info  Mode rule, mixed port 7897, TUN on, log level warning
  warn  allow-lan is on with no authentication
        Inbound ports listen on * (all interfaces), so any device on the network can use this proxy
        -> Turn off Allow LAN in Clash Verge Rev (clash-vr-tui config set lan off) unless you share the proxy on purpose
Egress
  ok    Internet reachable through the mixed port in 412ms
Routing
  info  3 of 62 connections (5%) fell through to MATCH, sent to Proxy
        -> Add rules for hosts that should not take the MATCH policy
...
7 ok, 6 info, 2 warn, 0 fail
```

The exit status is 1 when any check fails. A run takes about as long as the log
capture (`--logs`, default 10s; `--logs 0` skips it). To keep a record or hand
it to someone else:

```sh
clash-vr-tui doctor --logs 60s --out ~/cvt-reports
```

This writes `report.md`, `report.json`, `logs.ndjson`, `connections.json`, and
`runtime-config.json` into a new timestamped directory, readable only by you.
Credentials and fields that can hold secrets are left out, URLs keep only
scheme and host, and profile files are never copied.

### Scripting

Commands that print data accept `--json` (`logs --json` prints one object per
line). Exit status is 0 on success, 1 on a runtime failure, and 2 on a usage
error. Use `--` before a positional argument that starts with `-`.

```sh
clash-vr-tui status --json | jq .mode
clash-vr-tui switch Proxy 'JP-Tokyo-01'
clash-vr-tui test Proxy --timeout 3000
clash-vr-tui config set tun on && clash-vr-tui doctor --logs 0
```

## The TUI

Six pages, keyboard only, sized for real SSH panes down to 72x24.

- **Home**: core status, version, mode, ports, live memory and connection count;
  toggle TUN (`t`), cycle mode (`m`), restart the core (`R`).
- **Proxies**: groups and nodes side by side, or one panel at a time in panes
  narrower than about 80 columns (`Enter` opens a group, `Esc` returns). Select
  a node (`Enter`), sort by default, name, or delay (`o`), filter by name or
  delay (`delay<200`, `delay>500`, `delay=timeout`), and unpin an automatic
  group (`u`). Three latency modes cycle with `T`:
  - **HTTP**: mihomo's delay test through the tunnel, using the group's test URL
    or the configured `test-url`;
  - **TCP**: a raw connect to the node's `server:port`;
  - **ICMP**: an echo round-trip to the node's server.

  Every test reports each node as a delay, `timeout`, `skipped` (no known
  server, such as a sub-group), or `error`, with a count in the header such as
  `HTTP Auto: 16/18 ok, 2 timeout`. Results are kept per mode.
- **Connections**: live table with per-connection speed, sort, filter, detail
  view (`Enter`), close one (`x`) or all (`X`).
- **Rules**: searchable routing rules.
- **Logs**: live stream with level (`l`) and text (`/`) filters; `space`
  freezes the view while collection continues, and overflow is counted.
- **Config**: TUN, LAN exposure, and log level. Mixed port and mode are shown
  read-only here.

Enabling LAN, turning TUN off, and switching to global or direct mode ask for
`y` first, and every change is read back from the core so the screen shows what
actually held. The status bar shows `LIVE` while streams are current and
`STALE` with the last update time while they reconnect.

TCP and ICMP modes need each node's server address, which the mihomo API does
not expose, so they parse the running mihomo config. Nodes that only resolve
through the tunnel will not answer them; the HTTP test is the most reliable
signal.

### Keys

| Scope | Keys |
|-------|------|
| Global | `Tab`/`Shift+Tab` page · `1` to `6` jump · `?` help · `r` refresh · `R` restart core · `q`/`Ctrl+C` quit |
| Proxies | `←→`/`hl` panel · `Esc` back to groups · `j`/`k` move · `Ctrl+d`/`Ctrl+u` half-page · `g`/`G` top/bottom · `Enter` select · `d`/`D` test · `T` test mode · `o` sort · `u` unpin · `/` filter |
| Connections | `Enter` detail · `x` close · `X` close all · `s` sort · `/` filter |
| Rules | `g`/`G` top/bottom · `/` filter |
| Logs | `space` freeze · `l` level · `c` clear · `/` filter |
| Config | `Enter` edit · `Space` toggle · `j`/`k` move |

While a filter input is open, every key (including `q`) types into it and `Esc`
closes it. `Ctrl+C` always quits. The `?` overlay scrolls with `j`/`k`,
`Ctrl+d`/`Ctrl+u`, and `g`/`G`.

## Connecting

### Automatic

Without an explicit endpoint, `clash-vr-tui` checks these in order and uses the
first one that answers `GET /version`:

1. running mihomo or clash processes: their `-ext-ctl-unix`, `-ext-ctl`, and
   `-ext-ctl-pipe` flags, and the config file they loaded;
2. config files in the Clash Verge Rev app directory and the usual mihomo,
   clash, and clash-meta locations (`external-controller-unix`,
   `external-controller`, `secret`);
3. known paths: `/var/run/clash-verge-service/users/<uid>/verge-mihomo.sock`
   (Clash Verge Rev service mode), `/tmp/verge/verge-mihomo.sock`, and
   `$TMPDIR/verge-mihomo.sock`; on Windows the `verge-mihomo` and `mihomo` pipes.

Processes come first because Clash Verge Rev in service mode passes the socket
on the command line while its generated `clash-verge.yaml` names an older path.

```text
$ clash-vr-tui discover
  ADDRESS                                                   KIND    STATE    VERSION   SOURCE
* /var/run/clash-verge-service/users/501/verge-mihomo.sock  socket  ok       v1.19.32  process verge-mihomo -ext-ctl-unix (+1)
  127.0.0.1:9097                                            tcp     refused  -         config ~/Library/Application Support/io.github.clash-verge-rev.clash-verge-rev/config.yaml
```

### Explicit endpoint

```sh
clash-vr-tui --socket /run/mihomo/mihomo.sock         # external-controller-unix
clash-vr-tui --server 127.0.0.1:9090 --secret mypw    # external-controller
```

These flags work with every subcommand too. For a standalone mihomo service,
enable a controller in its config:

```yaml
external-controller: 127.0.0.1:9090
secret: "mypw"
```

`--config-path /etc/mihomo/config.yaml` helps the TCP/ICMP latency modes find
node addresses when the config is not in a usual place; nothing else needs it.

### Remote cores and secrets

The controller API is plain HTTP and WebSocket, so `clash-vr-tui` refuses to
send a secret to a non-loopback address. Forward the remote controller over SSH
and connect to the local end:

```sh
ssh -N -L 9090:127.0.0.1:9090 user@router
clash-vr-tui --server 127.0.0.1:9090 --secret-file ~/.config/mihomo/secret
```

`--secret-file` reads the secret from a file so it stays out of shell history
and the process list.

## Configuration

The controller resolves from **flags > environment > config file > automatic
detection > `/tmp/verge/verge-mihomo.sock`**. The endpoint and its secret come
from the same tier, so a secret in the config file is never sent to a server
given on the command line.

| Setting | Flag | Environment | Config file |
|---|---|---|---|
| Unix socket or pipe | `--socket` | `CLASH_VR_TUI_SOCKET` | `socket` |
| TCP controller | `--server` | `CLASH_VR_TUI_SERVER` | `server` |
| Secret | `--secret` | `CLASH_VR_TUI_SECRET` | `secret` |
| Secret from a file | `--secret-file` | `CLASH_VR_TUI_SECRET_FILE` | `secret-file` |
| mihomo config for TCP/ICMP | `--config-path` | `CLASH_VR_TUI_CONFIG_PATH` | `config-path` |
| Delay-test URL | `--url` (CLI) | `CLASH_VR_TUI_TEST_URL` | `test-url` |

The config file is `config.yaml` in `~/.config/clash-vr-tui/` on Linux,
`~/Library/Application Support/clash-vr-tui/` on macOS, and
`%AppData%\clash-vr-tui\` on Windows:

```yaml
# server: 127.0.0.1:9090
# secret-file: /etc/mihomo/controller.secret   # absolute path
config-path: ""   # mihomo config for TCP/ICMP server lookup
test-url: ""      # delay-test URL for groups without their own testUrl
```

A malformed file, an unknown key, `socket` together with `server`, or a secret
without an endpoint stops startup with the file path and the reason. An empty
file is fine.

## Install

The release installer downloads the archive for your platform, verifies it
against `checksums.txt`, and installs to `~/.local/bin`. It stops without
installing if the checksum is missing or wrong, or if neither `sha256sum` nor
`shasum` is available.

```sh
curl -fsSL https://raw.githubusercontent.com/lr00rl/clash-vr-tui/main/scripts/install.sh | sh
BINDIR=/usr/local/bin sh -c "$(curl -fsSL https://raw.githubusercontent.com/lr00rl/clash-vr-tui/main/scripts/install.sh)"
VERSION=v0.2.0 sh -c "$(curl -fsSL https://raw.githubusercontent.com/lr00rl/clash-vr-tui/main/scripts/install.sh)"
```

Or download an archive from
[GitHub Releases](https://github.com/lr00rl/clash-vr-tui/releases) and put
`clash-vr-tui` on your `PATH`:

| Platform | Asset |
|----------|-------|
| Linux x86_64 / amd64 | `clash-vr-tui_linux_amd64.tar.gz` |
| Linux aarch64 / arm64 | `clash-vr-tui_linux_arm64.tar.gz` |
| macOS Intel | `clash-vr-tui_darwin_amd64.tar.gz` |
| macOS Apple Silicon | `clash-vr-tui_darwin_arm64.tar.gz` |
| Windows x86_64 | `clash-vr-tui_windows_amd64.zip` |
| Windows arm64 | `clash-vr-tui_windows_arm64.zip` |

To build from source (Go 1.25 or newer):

```sh
make build          # -> ./clash-vr-tui
make install        # -> /usr/local/bin/clash-vr-tui (PREFIX overridable)
make dist           # cross-compile release binaries into dist/
```

## Platforms

| Platform | Status | Notes |
|----------|--------|-------|
| Linux amd64 / arm64 | Primary | Best fit for SSH use. Unix socket and TCP controllers. |
| macOS amd64 / arm64 | Primary | Detects Clash Verge Rev in user and service mode. |
| Windows amd64 / arm64 | Secondary | Named pipes (`\\.\pipe\verge-mihomo`) and TCP controllers; less tested. |

Linux builds are static (`CGO_ENABLED=0`), so pick the archive by CPU
architecture, not by distro: `x86_64` is `linux_amd64` and `aarch64` is
`linux_arm64`, on Ubuntu, Debian, Fedora, RHEL and its rebuilds, Arch, Amazon
Linux, Alibaba Cloud Linux, and OpenAnolis alike. ICMP probes may need root or
`CAP_NET_RAW`; everything else runs as a normal user that can reach the
controller.

## Development

Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and Lip Gloss;
REST and WebSocket over a Unix socket, named pipe, or TCP. Design notes live in
`docs/plans/` and `DESIGN.md`.

CI runs `gofmt`, `go vet`, `go test`, the installer checksum tests, a host
build, the `make dist` cross-build, and a GoReleaser config check. A pushed tag
publishes a release:

```sh
git tag -a v0.2.0 -m "v0.2.0"
git push origin v0.2.0
```

GoReleaser builds Linux, macOS, and Windows archives for amd64 and arm64 and
uploads them with `checksums.txt`, which is what the installer reads. For a
local dry run, `make release-check` and `make snapshot` need the GoReleaser v2
CLI; normal pull requests do not.
