package app

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/messages"
	"github.com/cdcd/clash-vr-tui/internal/ui/connections"
	"github.com/cdcd/clash-vr-tui/internal/ui/helpbar"
	"github.com/cdcd/clash-vr-tui/internal/ui/home"
	"github.com/cdcd/clash-vr-tui/internal/ui/overlay"
	"github.com/cdcd/clash-vr-tui/internal/ui/proxies"
	"github.com/cdcd/clash-vr-tui/internal/ui/rules"
	"github.com/cdcd/clash-vr-tui/internal/ui/settings"
	"github.com/cdcd/clash-vr-tui/internal/ui/sidebar"
	"github.com/cdcd/clash-vr-tui/internal/ui/statusbar"
)

type Model struct {
	// Layout
	sidebar   sidebar.Model
	statusbar statusbar.Model
	helpbar   helpbar.Model
	overlay   overlay.Model

	// Pages
	home        home.Model
	proxies     proxies.Model
	connections connections.Model
	rules       rules.Model
	settings    settings.Model

	// State
	activePage    messages.Page
	client        *api.Client
	trafficCancel context.CancelFunc
	connsCancel   context.CancelFunc
	width         int
	height        int
	ready         bool
	statusSeq     int
}

// Timing constants for stream reconnection and transient status messages.
const (
	reconnectDelay = 3 * time.Second
	statusTTL      = 4 * time.Second
)

func NewModel(client *api.Client) Model {
	return Model{
		sidebar:     sidebar.New(),
		statusbar:   statusbar.New(),
		helpbar:     helpbar.New(),
		overlay:     overlay.New(),
		home:        home.New(client),
		proxies:     proxies.New(client),
		connections: connections.New(client),
		rules:       rules.New(client),
		settings:    settings.New(client),
		activePage:  messages.PageHome,
		client:      client,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.home.Init(),
		m.proxies.Init(),
		m.rules.Init(),
		m.settings.Init(),
		m.startTrafficStream(),
		m.startConnectionsStream(),
	)
}

// cancelAll cancels both WebSocket stream contexts.
func (m *Model) cancelAll() {
	if m.trafficCancel != nil {
		m.trafficCancel()
	}
	if m.connsCancel != nil {
		m.connsCancel()
	}
}

func (m Model) startTrafficStream() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		ch := make(chan api.TrafficData, 1)
		errCh := make(chan error, 1)
		go func() {
			errCh <- m.client.StreamTraffic(ctx, ch)
		}()
		select {
		case data := <-ch:
			return trafficStarted{cancel: cancel, ch: ch, first: data}
		case err := <-errCh:
			cancel()
			return trafficDown{err: fmt.Errorf("traffic stream: %w", err)}
		case <-time.After(10 * time.Second):
			cancel()
			return trafficDown{err: fmt.Errorf("traffic stream: connection timeout")}
		}
	}
}

func (m Model) startConnectionsStream() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		ch := make(chan api.ConnectionsSnapshot, 1)
		errCh := make(chan error, 1)
		go func() {
			errCh <- m.client.StreamConnections(ctx, ch)
		}()
		select {
		case snap := <-ch:
			return connsStarted{cancel: cancel, ch: ch, first: snap}
		case err := <-errCh:
			cancel()
			return connsDown{err: fmt.Errorf("connections stream: %w", err)}
		case <-time.After(10 * time.Second):
			cancel()
			return connsDown{err: fmt.Errorf("connections stream: connection timeout")}
		}
	}
}

// Internal messages for WS stream initialization
type trafficStarted struct {
	cancel context.CancelFunc
	ch     <-chan api.TrafficData
	first  api.TrafficData
}

type connsStarted struct {
	cancel context.CancelFunc
	ch     <-chan api.ConnectionsSnapshot
	first  api.ConnectionsSnapshot
}

type trafficTick struct {
	ch   <-chan api.TrafficData
	data api.TrafficData
}

type connsTick struct {
	ch   <-chan api.ConnectionsSnapshot
	data api.ConnectionsSnapshot
}

func waitForTraffic(ch <-chan api.TrafficData) tea.Cmd {
	return func() tea.Msg {
		data, ok := <-ch
		if !ok {
			return trafficDown{err: fmt.Errorf("traffic stream closed")}
		}
		return trafficTick{ch: ch, data: data}
	}
}

func waitForConns(ch <-chan api.ConnectionsSnapshot) tea.Cmd {
	return func() tea.Msg {
		snap, ok := <-ch
		if !ok {
			return connsDown{err: fmt.Errorf("connections stream closed")}
		}
		return connsTick{ch: ch, data: snap}
	}
}

// --- Stream death / reconnect messages ---

// trafficDown / connsDown signal that a stream ended (error or closed). The
// root model schedules a reconnect after reconnectDelay.
type trafficDown struct{ err error }
type connsDown struct{ err error }

// trafficReconnect / connsReconnect fire after the backoff delay to re-dial.
type trafficReconnect struct{}
type connsReconnect struct{}

// clearStatus clears a transient status line if it is still the latest one.
type clearStatus struct{ seq int }

// flash sets a transient status message and returns a command that clears it
// after statusTTL (unless superseded by a newer flash).
func (m *Model) flash(text string, isErr bool) tea.Cmd {
	m.statusSeq++
	seq := m.statusSeq
	m.statusbar = m.statusbar.SetStatus(text, isErr)
	return tea.Tick(statusTTL, func(time.Time) tea.Msg { return clearStatus{seq: seq} })
}
