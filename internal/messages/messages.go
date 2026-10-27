package messages

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/probe"
)

// Page identifies which page is active.
type Page int

const (
	PageHome Page = iota
	PageProxies
	PageConnections
	PageRules
	PageLogs
	PageSettings
)

func (p Page) String() string {
	switch p {
	case PageHome:
		return "Home"
	case PageProxies:
		return "Proxy"
	case PageConnections:
		return "Conns"
	case PageRules:
		return "Rules"
	case PageLogs:
		return "Logs"
	case PageSettings:
		return "Config"
	default:
		return "Unknown"
	}
}

// Pages returns all pages in order.
func Pages() []Page {
	return []Page{PageHome, PageProxies, PageConnections, PageRules, PageLogs, PageSettings}
}

// --- Navigation messages ---

// SwitchPageMsg requests switching to a page.
type SwitchPageMsg struct{ Page Page }

// --- Traffic messages ---

// TrafficMsg carries a traffic tick from WebSocket.
type TrafficMsg struct{ Data api.TrafficData }

// --- Connections messages ---

// ConnectionsMsg carries a connections snapshot from WebSocket.
type ConnectionsMsg struct{ Data api.ConnectionsSnapshot }

// --- Data fetch result messages ---

// ConfigMsg carries config data.
type ConfigMsg struct {
	Config *api.Config
	Err    error
}

// VersionMsg carries version data.
type VersionMsg struct {
	Version *api.VersionInfo
	Err     error
}

// GroupsMsg carries proxy groups data.
type GroupsMsg struct {
	Groups []api.Group
	Err    error
}

// RulesMsg carries rules data.
type RulesMsg struct {
	Rules *api.RulesResponse
	Err   error
}

// DelayResult keeps a measurement distinct from a timeout, skipped node, or
// failed probe. Delay is meaningful only when Status is DelayOK.
type DelayStatus string

const (
	DelayOK      DelayStatus = "ok"
	DelayTimeout DelayStatus = "timeout"
	DelaySkipped DelayStatus = "skipped"
	DelayError   DelayStatus = "error"
)

type DelayResult struct {
	Delay  int
	Status DelayStatus
	Reason string
}

// GroupDelayMsg identifies the probe mode and request that produced a result.
type GroupDelayMsg struct {
	Group     string
	Mode      probe.Mode
	RequestID uint64
	Results   map[string]DelayResult
	Err       error
}

// ProxyDelayMsg carries delay test result for a single proxy.
type ProxyDelayMsg struct {
	Group     string
	Name      string
	Mode      probe.Mode
	RequestID uint64
	Result    DelayResult
	Err       error
}

// ProxySelectedMsg signals a proxy was selected in a group.
type ProxySelectedMsg struct {
	Group string
	Proxy string
	Err   error
}

// ConfigChangeMsg asks the root model to apply a config patch. The root decides
// which changes need confirmation, so a page cannot bypass that policy.
type ConfigChangeMsg struct {
	Label string // what the change does, e.g. "Allow LAN on"
	Patch api.ConfigPatch
}

// RequestConfigChange returns a command that emits a ConfigChangeMsg.
func RequestConfigChange(label string, patch api.ConfigPatch) tea.Cmd {
	return func() tea.Msg { return ConfigChangeMsg{Label: label, Patch: patch} }
}

// ConfigPatchedMsg reports a patch and the config read back after it. Err is
// the patch failure; ReadErr means the patch was sent but could not be checked.
type ConfigPatchedMsg struct {
	Label   string
	Patch   api.ConfigPatch
	Config  *api.Config
	Err     error
	ReadErr error
}

// ConnClosedMsg signals a connection was closed.
type ConnClosedMsg struct {
	ID  string
	Err error
}

// AllConnsClosedMsg signals all connections were closed.
type AllConnsClosedMsg struct {
	Err error
}

// ErrMsg carries a generic error.
type ErrMsg struct{ Err error }

// CoreRestartMsg signals a core restart attempt completed.
type CoreRestartMsg struct{ Err error }
