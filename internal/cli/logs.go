package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/styles"
)

const logsReconnectDelay = 2 * time.Second

// cmdLogs streams WS /logs until Ctrl+C or --for elapses. A dropped stream,
// such as a core restart, is retried; a stream that never connects fails.
func cmdLogs(c *api.Client, opts options) error {
	level := opts.level
	if level == "" {
		level = "info"
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if opts.forDur > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.forDur)
		defer cancel()
	}
	p := logPrinter{json: opts.json, color: !opts.json && useColor(os.Stdout), grep: strings.ToLower(opts.grep)}

	everConnected := false
	for {
		connected, err := streamLogsOnce(ctx, c, level, p.print)
		everConnected = everConnected || connected
		if ctx.Err() != nil {
			return nil
		}
		if !everConnected {
			return fmt.Errorf("logs stream: %w", err)
		}
		if connected {
			fmt.Fprintf(os.Stderr, "logs stream lost (%s); reconnecting\n", styles.SafeText(err.Error()))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(logsReconnectDelay):
		}
	}
}

// streamLogsOnce runs one WebSocket session and reports whether any entry
// arrived before it ended.
func streamLogsOnce(ctx context.Context, c *api.Client, level string, emit func(api.LogEntry)) (bool, error) {
	ch := make(chan api.LogEntry, 64)
	errCh := make(chan error, 1)
	go func() { errCh <- c.StreamLogs(ctx, level, ch) }()
	got := false
	for {
		select {
		case entry, ok := <-ch:
			if !ok {
				return got, <-errCh
			}
			got = true
			emit(entry)
		case err := <-errCh:
			// StreamLogs closes ch only after a successful dial; drain what
			// is buffered without waiting on a channel that may never close.
			for {
				select {
				case entry, ok := <-ch:
					if !ok {
						return got, err
					}
					got = true
					emit(entry)
				default:
					return got, err
				}
			}
		}
	}
}

type logPrinter struct {
	json  bool
	color bool
	grep  string
}

func (p logPrinter) print(e api.LogEntry) {
	if p.grep != "" && !strings.Contains(strings.ToLower(e.Payload), p.grep) {
		return
	}
	now := time.Now()
	if p.json {
		line, _ := json.Marshal(map[string]string{"time": now.Format(time.RFC3339Nano), "level": e.Type, "payload": e.Payload})
		fmt.Println(string(line))
		return
	}
	label := strings.ToUpper(e.Type)
	if label == "WARNING" {
		label = "WARN"
	}
	label = fmt.Sprintf("%-5s", label)
	if p.color {
		label = levelColor(e.Type) + label + ansiReset
	}
	fmt.Printf("%s %s %s\n", now.Format("15:04:05"), label, styles.SafeText(e.Payload))
}

const ansiReset = "\x1b[0m"

func levelColor(level string) string {
	switch level {
	case "error":
		return "\x1b[31m"
	case "warning":
		return "\x1b[33m"
	case "debug":
		return "\x1b[2m"
	}
	return "\x1b[36m"
}

// useColor reports whether f is a terminal and NO_COLOR is unset.
func useColor(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
