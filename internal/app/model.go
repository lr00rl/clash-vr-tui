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
	"github.com/cdcd/clash-vr-tui/internal/ui/logs"
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
	logs        logs.Model
	settings    settings.Model

	// State
	activePage    messages.Page
	client        *api.Client
	trafficCancel context.CancelFunc
	connsCancel   context.CancelFunc
	logsCancel    context.CancelFunc
	logsStarted   bool
	trafficGen    uint64
	connsGen      uint64
	logsGen       uint64
	trafficHealth streamHealth
	connsHealth   streamHealth
	logsHealth    streamHealth
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

type streamHealth struct {
	state string
	last  time.Time
	err   error
}

func NewModel(client *api.Client, testURL string) Model {
	return Model{
		sidebar:       sidebar.New(),
		statusbar:     statusbar.New(),
		helpbar:       helpbar.New(),
		overlay:       overlay.New(),
		home:          home.New(client),
		proxies:       proxies.NewWithTestURL(client, testURL),
		connections:   connections.New(client),
		rules:         rules.New(client),
		logs:          logs.New(),
		settings:      settings.New(client),
		activePage:    messages.PageHome,
		client:        client,
		trafficGen:    1,
		connsGen:      1,
		logsGen:       1,
		trafficHealth: streamHealth{state: "connecting"},
		connsHealth:   streamHealth{state: "connecting"},
		logsHealth:    streamHealth{state: "connecting"},
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

// cancelAll cancels all WebSocket stream contexts.
func (m *Model) cancelAll() {
	if m.trafficCancel != nil {
		m.trafficCancel()
	}
	if m.connsCancel != nil {
		m.connsCancel()
	}
	if m.logsCancel != nil {
		m.logsCancel()
	}
}

func (m Model) startLogsStream() tea.Cmd {
	gen := m.logsGen
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		ch := make(chan api.LogEntry, 64)
		errCh := make(chan error, 1)
		go func() {
			errCh <- m.client.StreamLogs(ctx, "debug", ch)
		}()
		select {
		case entry := <-ch:
			return logsStarted{gen: gen, cancel: cancel, ch: ch, first: entry}
		case err := <-errCh:
			cancel()
			return logsDown{gen: gen, err: fmt.Errorf("logs stream: %w", err)}
		case <-time.After(10 * time.Second):
			cancel()
			return logsDown{gen: gen, err: fmt.Errorf("logs stream: connection timeout")}
		}
	}
}

func (m Model) startTrafficStream() tea.Cmd {
	gen := m.trafficGen
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		ch := make(chan api.TrafficData, 1)
		errCh := make(chan error, 1)
		go func() {
			errCh <- m.client.StreamTraffic(ctx, ch)
		}()
		select {
		case data := <-ch:
			return trafficStarted{gen: gen, cancel: cancel, ch: ch, first: data}
		case err := <-errCh:
			cancel()
			return trafficDown{gen: gen, err: fmt.Errorf("traffic stream: %w", err)}
		case <-time.After(10 * time.Second):
			cancel()
			return trafficDown{gen: gen, err: fmt.Errorf("traffic stream: connection timeout")}
		}
	}
}

func (m Model) startConnectionsStream() tea.Cmd {
	gen := m.connsGen
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		ch := make(chan api.ConnectionsSnapshot, 1)
		errCh := make(chan error, 1)
		go func() {
			errCh <- m.client.StreamConnections(ctx, ch)
		}()
		select {
		case snap := <-ch:
			return connsStarted{gen: gen, cancel: cancel, ch: ch, first: snap}
		case err := <-errCh:
			cancel()
			return connsDown{gen: gen, err: fmt.Errorf("connections stream: %w", err)}
		case <-time.After(10 * time.Second):
			cancel()
			return connsDown{gen: gen, err: fmt.Errorf("connections stream: connection timeout")}
		}
	}
}

// Internal messages for WS stream initialization
type trafficStarted struct {
	gen    uint64
	cancel context.CancelFunc
	ch     <-chan api.TrafficData
	first  api.TrafficData
}

type connsStarted struct {
	gen    uint64
	cancel context.CancelFunc
	ch     <-chan api.ConnectionsSnapshot
	first  api.ConnectionsSnapshot
}

type trafficTick struct {
	gen  uint64
	ch   <-chan api.TrafficData
	data api.TrafficData
}

type connsTick struct {
	gen  uint64
	ch   <-chan api.ConnectionsSnapshot
	data api.ConnectionsSnapshot
}

type logsStarted struct {
	gen    uint64
	cancel context.CancelFunc
	ch     <-chan api.LogEntry
	first  api.LogEntry
}

type logsTick struct {
	gen   uint64
	ch    <-chan api.LogEntry
	entry api.LogEntry
}

func waitForTraffic(gen uint64, ch <-chan api.TrafficData) tea.Cmd {
	return func() tea.Msg {
		data, ok := <-ch
		if !ok {
			return trafficDown{gen: gen, err: fmt.Errorf("traffic stream closed")}
		}
		return trafficTick{gen: gen, ch: ch, data: data}
	}
}

func waitForConns(gen uint64, ch <-chan api.ConnectionsSnapshot) tea.Cmd {
	return func() tea.Msg {
		snap, ok := <-ch
		if !ok {
			return connsDown{gen: gen, err: fmt.Errorf("connections stream closed")}
		}
		return connsTick{gen: gen, ch: ch, data: snap}
	}
}

func waitForLogs(gen uint64, ch <-chan api.LogEntry) tea.Cmd {
	return func() tea.Msg {
		entry, ok := <-ch
		if !ok {
			return logsDown{gen: gen, err: fmt.Errorf("logs stream closed")}
		}
		return logsTick{gen: gen, ch: ch, entry: entry}
	}
}

// --- Stream death / reconnect messages ---

// trafficDown / connsDown / logsDown signal that a stream ended (error or
// closed). The root model schedules a reconnect after reconnectDelay.
type trafficDown struct {
	gen uint64
	err error
}
type connsDown struct {
	gen uint64
	err error
}
type logsDown struct {
	gen uint64
	err error
}

// *Reconnect messages fire after the backoff delay to re-dial.
type trafficReconnect struct{ gen uint64 }
type connsReconnect struct{ gen uint64 }
type logsReconnect struct{ gen uint64 }

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

func (m Model) syncStreamState() Model {
	state := "connecting"
	if m.trafficHealth.state == "live" && m.connsHealth.state == "live" {
		state = "live"
	} else if m.trafficHealth.state == "reconnecting" || m.connsHealth.state == "reconnecting" {
		state = "reconnecting"
	}
	last := m.trafficHealth.last
	if last.IsZero() || (!m.connsHealth.last.IsZero() && m.connsHealth.last.Before(last)) {
		last = m.connsHealth.last
	}
	m.statusbar = m.statusbar.SetStreamState(state, last)
	m.home = m.home.SetStreamState(state, last)
	m.logs = m.logs.SetStreamState(m.logsHealth.state, m.logsHealth.last)
	return m
}
