package messages

import "github.com/cdcd/clash-vr-tui/internal/api"

// Page identifies which page is active.
type Page int

const (
	PageHome Page = iota
	PageProxies
	PageConnections
	PageRules
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
	case PageSettings:
		return "About"
	default:
		return "Unknown"
	}
}

// Pages returns all pages in order.
func Pages() []Page {
	return []Page{PageHome, PageProxies, PageConnections, PageRules, PageSettings}
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

// GroupDelayMsg carries delay test results for a group.
type GroupDelayMsg struct {
	Group  string
	Result api.GroupDelayResult
	Err    error
}

// ProxyDelayMsg carries delay test result for a single proxy.
type ProxyDelayMsg struct {
	Name  string
	Delay int
	Err   error
}

// ProxySelectedMsg signals a proxy was selected in a group.
type ProxySelectedMsg struct {
	Group string
	Proxy string
	Err   error
}

// ConfigPatchedMsg signals a config patch completed.
type ConfigPatchedMsg struct {
	Err error
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

// WSReconnectMsg signals that a WebSocket should reconnect.
type WSReconnectMsg struct{ Stream string }
