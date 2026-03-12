package api

// Proxy represents a single proxy node.
type Proxy struct {
	Name    string         `json:"name"`
	Type    string         `json:"type"`
	History []DelayHistory `json:"history,omitempty"`
	All     []string       `json:"all,omitempty"`
	Now     string         `json:"now,omitempty"`
	UDP     bool           `json:"udp"`
	Alive   bool           `json:"alive"`
}

// ProxiesResponse is the response from GET /proxies.
type ProxiesResponse struct {
	Proxies map[string]Proxy `json:"proxies"`
}

// Group represents a proxy group with its nodes.
type Group struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Now     string   `json:"now"`
	All     []string `json:"all"`
	History []DelayHistory `json:"history,omitempty"`
	Hidden  bool     `json:"hidden,omitempty"`
}

// DelayHistory holds a single delay test result.
type DelayHistory struct {
	Delay int `json:"delay"`
}

// GroupsResponse is the response from GET /group.
type GroupsResponse struct {
	Proxies []Group `json:"proxies"`
}

// DelayResult is the response from delay testing.
type DelayResult struct {
	Delay   int    `json:"delay"`
	Message string `json:"message,omitempty"`
}

// GroupDelayResult maps proxy names to their delay values.
type GroupDelayResult map[string]int

// Connection represents a single active connection.
type Connection struct {
	ID       string         `json:"id"`
	Metadata ConnMetadata   `json:"metadata"`
	Upload   int64          `json:"upload"`
	Download int64          `json:"download"`
	Start    string         `json:"start"`
	Chains   []string       `json:"chains"`
	Rule     string         `json:"rule"`
	RulePayload string      `json:"rulePayload"`
}

// ConnMetadata holds connection metadata.
type ConnMetadata struct {
	Network     string `json:"network"`
	Type        string `json:"type"`
	SrcIP       string `json:"sourceIP"`
	SrcPort     string `json:"sourcePort"`
	DstIP       string `json:"destinationIP"`
	DstPort     string `json:"destinationPort"`
	Host        string `json:"host"`
	Process     string `json:"process"`
	ProcessPath string `json:"processPath"`
}

// ConnectionsSnapshot is the response from GET /connections or WS /connections.
type ConnectionsSnapshot struct {
	DownloadTotal int64        `json:"downloadTotal"`
	UploadTotal   int64        `json:"uploadTotal"`
	Connections   []Connection `json:"connections"`
	Memory        int64        `json:"memory,omitempty"`
}

// TrafficData is a single traffic tick from WS /traffic.
type TrafficData struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

// Rule represents a single rule entry.
type Rule struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
	Proxy   string `json:"proxy"`
	Size    int    `json:"size,omitempty"`
}

// RulesResponse is the response from GET /rules.
type RulesResponse struct {
	Rules []Rule `json:"rules"`
}

// Config represents mihomo runtime config.
type Config struct {
	Port           int    `json:"port"`
	SocksPort      int    `json:"socks-port"`
	MixedPort      int    `json:"mixed-port"`
	AllowLan       bool   `json:"allow-lan"`
	Mode           string `json:"mode"`
	LogLevel       string `json:"log-level"`
	TUN            TUNConfig `json:"tun"`
}

// TUNConfig holds TUN settings.
type TUNConfig struct {
	Enable bool   `json:"enable"`
	Stack  string `json:"stack,omitempty"`
	Device string `json:"device,omitempty"`
}

// VersionInfo is the response from GET /version.
type VersionInfo struct {
	Meta    bool   `json:"meta"`
	Version string `json:"version"`
}

// ConfigPatch is used for PATCH /configs.
type ConfigPatch struct {
	Port      *int    `json:"port,omitempty"`
	SocksPort *int    `json:"socks-port,omitempty"`
	MixedPort *int    `json:"mixed-port,omitempty"`
	AllowLan  *bool   `json:"allow-lan,omitempty"`
	Mode      *string `json:"mode,omitempty"`
	LogLevel  *string `json:"log-level,omitempty"`
	TUN       *TUNConfig `json:"tun,omitempty"`
}
