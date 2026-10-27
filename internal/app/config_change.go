package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/messages"
)

// configChangeWarning explains the impact of a change that alters where traffic
// goes or who can reach the proxy. An empty result means no confirmation.
func configChangeWarning(p api.ConfigPatch) string {
	switch {
	case p.AllowLan != nil && *p.AllowLan:
		return "Expose the proxy ports to every device on the local network?\nAnyone on the LAN can then route traffic through this core."
	case p.TUN != nil && !p.TUN.Enable:
		return "Turn TUN off?\nApps that ignore the system proxy will bypass the core and connect directly."
	case p.Mode != nil && *p.Mode == "direct":
		return "Switch to direct mode?\nAll traffic will skip proxies and rules and leave this machine directly."
	case p.Mode != nil && *p.Mode == "global":
		return "Switch to global mode?\nAll traffic will go through the GLOBAL group and rules will be ignored."
	}
	return ""
}

// applyConfigChange sends the patch, then reads the config back so the UI shows
// what the controller actually holds rather than what was requested.
func applyConfigChange(client *api.Client, change messages.ConfigChangeMsg) tea.Cmd {
	return func() tea.Msg {
		msg := messages.ConfigPatchedMsg{Label: change.Label, Patch: change.Patch}
		if msg.Err = client.PatchConfig(change.Patch); msg.Err != nil {
			return msg
		}
		msg.Config, msg.ReadErr = client.GetConfig()
		return msg
	}
}

func configPatchedStatus(msg messages.ConfigPatchedMsg) (string, bool) {
	switch {
	case msg.Err != nil:
		return "Config update failed: " + msg.Err.Error(), true
	case msg.ReadErr != nil:
		return msg.Label + " sent, but reading it back failed: " + msg.ReadErr.Error(), true
	case msg.Config != nil && !patchHeld(msg.Patch, msg.Config):
		return "Controller did not keep " + msg.Label, true
	}
	return "Applied " + msg.Label, false
}

// patchHeld reports whether every field the patch set matches the read-back
// config.
func patchHeld(p api.ConfigPatch, c *api.Config) bool {
	return (p.AllowLan == nil || *p.AllowLan == c.AllowLan) &&
		(p.Mode == nil || strings.EqualFold(*p.Mode, c.Mode)) &&
		(p.LogLevel == nil || strings.EqualFold(*p.LogLevel, c.LogLevel)) &&
		(p.TUN == nil || p.TUN.Enable == c.TUN.Enable) &&
		(p.Port == nil || *p.Port == c.Port) &&
		(p.SocksPort == nil || *p.SocksPort == c.SocksPort) &&
		(p.MixedPort == nil || *p.MixedPort == c.MixedPort)
}
