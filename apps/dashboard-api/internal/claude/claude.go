// Package claude controls the Claude Code tmux session on the host. It talks to
// the host's tmux server over its socket (visible through the read-only / mount)
// and only ever runs a fixed set of tmux commands: status, capture, start, stop.
// There is deliberately no way to send arbitrary keys through it.
package claude

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/magmc/server-projects/dashboard-api/internal/model"
)

// ErrConflict means the requested action doesn't fit the session's current state.
var ErrConflict = errors.New("conflict")

// Shells we're willing to type the start command into. Anything else in the
// foreground (vim, a running claude, ...) must not receive keystrokes.
var shells = map[string]bool{"bash": true, "zsh": true, "sh": true, "fish": true, "dash": true}

type Controller struct {
	socket   string
	session  string
	workdir  string
	startCmd string
}

func NewController(socket, session, workdir, startCmd string) *Controller {
	return &Controller{socket: socket, session: session, workdir: workdir, startCmd: startCmd}
}

// tmux runs one tmux command against the host server. -N stops tmux from
// spawning a server inside this container when the host's isn't running.
func (c *Controller) tmux(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "tmux", append([]string{"-N", "-S", c.socket}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", errors.New(msg)
		}
		return "", err
	}
	return stdout.String(), nil
}

// sessionTarget matches the session name exactly (no prefix matching);
// paneTarget is that session's active pane.
func (c *Controller) sessionTarget() string { return "=" + c.session }
func (c *Controller) paneTarget() string    { return "=" + c.session + ":" }

func (c *Controller) hasSession(ctx context.Context) bool {
	_, err := c.tmux(ctx, "has-session", "-t", c.sessionTarget())
	return err == nil
}

func (c *Controller) Status(ctx context.Context) model.ClaudeStatus {
	st := model.ClaudeStatus{Session: c.session}
	if _, err := c.tmux(ctx, "list-sessions"); err != nil {
		st.Error = "tmux server not reachable: " + err.Error()
		return st
	}
	st.Available = true
	if !c.hasSession(ctx) {
		return st
	}
	// tmux rewrites control chars like \t in its output, so split on "|" with
	// the path last (it's the only field that could contain one).
	out, err := c.tmux(ctx, "display-message", "-p", "-t", c.paneTarget(),
		"#{session_created}|#{session_attached}|#{pane_current_command}|#{pane_width}|#{pane_height}|#{pane_current_path}")
	if err != nil {
		st.Error = err.Error()
		return st
	}
	f := strings.SplitN(strings.TrimRight(out, "\n"), "|", 6)
	if len(f) != 6 {
		st.Error = "unexpected tmux output"
		return st
	}
	st.Exists = true
	st.CreatedAt, _ = strconv.ParseInt(f[0], 10, 64)
	st.Attached, _ = strconv.Atoi(f[1])
	st.Command = f[2]
	st.Width, _ = strconv.Atoi(f[3])
	st.Height, _ = strconv.Atoi(f[4])
	st.Path = f[5]
	st.Running = st.Command == "claude"
	return st
}

// Capture returns the visible pane plus up to `history` lines of scrollback,
// with ANSI colour escapes preserved.
func (c *Controller) Capture(ctx context.Context, history int) (string, error) {
	if !c.hasSession(ctx) {
		return "", fmt.Errorf("%w: session %q is not running", ErrConflict, c.session)
	}
	out, err := c.tmux(ctx, "capture-pane", "-p", "-e", "-t", c.paneTarget(), "-S", strconv.Itoa(-history))
	if err != nil {
		return "", err
	}
	return strings.TrimRight(out, "\n "), nil
}

// Start creates the session if needed and types the start command into its
// shell, mirroring how the session is used by hand (quitting claude leaves the
// shell behind).
func (c *Controller) Start(ctx context.Context) error {
	if !c.hasSession(ctx) {
		args := []string{"new-session", "-d", "-s", c.session, "-x", "120", "-y", "40"}
		if c.workdir != "" {
			args = append(args, "-c", c.workdir)
		}
		if _, err := c.tmux(ctx, args...); err != nil {
			return err
		}
	} else if st := c.Status(ctx); st.Running {
		return fmt.Errorf("%w: claude is already running", ErrConflict)
	} else if !shells[st.Command] {
		return fmt.Errorf("%w: pane is busy running %q", ErrConflict, st.Command)
	}
	if _, err := c.tmux(ctx, "send-keys", "-t", c.paneTarget(), "-l", c.startCmd); err != nil {
		return err
	}
	_, err := c.tmux(ctx, "send-keys", "-t", c.paneTarget(), "Enter")
	return err
}

// Stop kills the whole session. It first turns off exit-empty so the host tmux
// server survives even if this was its last session; otherwise Start would have
// no server to talk to afterwards.
func (c *Controller) Stop(ctx context.Context) error {
	if !c.hasSession(ctx) {
		return nil
	}
	if _, err := c.tmux(ctx, "set-option", "-s", "exit-empty", "off"); err != nil {
		return err
	}
	_, err := c.tmux(ctx, "kill-session", "-t", c.sessionTarget())
	return err
}
