package doctor

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cdcd/clash-vr-tui/internal/discover"
)

// System is the machine state doctor reads. LocalSystem returns the real
// one; tests build their own.
type System struct {
	GOOS string
	Home string
	// FS opens absolute paths written without the leading slash, as fsPath
	// converts them, so tests can pass an fstest.MapFS.
	FS     fs.FS
	Getenv func(string) string
	Now    func() time.Time
	// PS lists processes as "pid command" lines (ps -axww -o pid=,command=).
	PS func(ctx context.Context) (string, error)
	// ProxySettings returns `scutil --proxy` output; only used on darwin.
	ProxySettings func(ctx context.Context) (string, error)
	// Discover lists local controller candidates with their probe state.
	Discover func(ctx context.Context) []discover.Candidate
}

const commandTimeout = 3 * time.Second

// LocalSystem reads the machine doctor runs on.
func LocalSystem() System {
	home, _ := os.UserHomeDir()
	return System{
		GOOS:   runtime.GOOS,
		Home:   home,
		FS:     rootFS{},
		Getenv: os.Getenv,
		Now:    time.Now,
		PS: func(ctx context.Context) (string, error) {
			if runtime.GOOS == "windows" {
				return "", nil
			}
			return runCommand(ctx, "ps", "-axww", "-o", "pid=,command=")
		},
		ProxySettings: func(ctx context.Context) (string, error) {
			return runCommand(ctx, "scutil", "--proxy")
		},
		Discover: discover.Find,
	}
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}

// rootFS serves the real filesystem under fs.FS names (absolute paths with the
// leading slash removed, or "C:/..." on Windows).
type rootFS struct{}

func (rootFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if runtime.GOOS == "windows" {
		return os.Open(filepath.FromSlash(name))
	}
	return os.Open("/" + name)
}

// fsPath turns an absolute OS path into a System.FS name.
func fsPath(p string) string {
	return strings.TrimPrefix(filepath.ToSlash(p), "/")
}

// readTail returns up to the last n bytes of a file, starting at a line
// boundary when it had to skip the beginning.
func readTail(fsys fs.FS, name string, n int64) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	cut := false
	if s, ok := f.(io.Seeker); ok && info.Size() > n {
		if _, err := s.Seek(info.Size()-n, io.SeekStart); err != nil {
			return nil, err
		}
		cut = true
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > n {
		data = data[int64(len(data))-n:]
		cut = true
	}
	if cut {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		}
	}
	return data, nil
}
