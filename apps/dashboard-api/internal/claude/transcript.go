package claude

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/magmc/server-projects/dashboard-api/internal/model"
)

// A session started with `claude remote-control` only shows the bridge's status
// line in its tmux pane; the conversations it spawns are headless. What they're
// doing is read from Claude Code's own transcripts instead: one JSONL file per
// conversation under <projects>/<encoded cwd>/. This is strictly read-only.

// ErrNotFound means the session has no conversation with the requested id.
var ErrNotFound = errors.New("not found")

const (
	maxConversations = 6    // newest conversations listed per session
	maxEvents        = 200  // newest events kept per conversation
	maxText          = 3000 // runes kept of a prompt or reply
	maxToolText      = 240  // runes kept of a tool call summary
	maxTitle         = 80
	maxLast          = 160
	// A turn that hasn't written anything for this long is treated as dead
	// (interrupted, crashed) rather than still working.
	busyWindow = 10 * time.Minute
)

// Conversation ids are UUIDs; anything else never reaches the filesystem.
var conversationID = regexp.MustCompile(`^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$`)

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]`)

// transcript is one parsed conversation file, cached until the file changes.
type transcript struct {
	size   int64
	mod    time.Time
	title  string
	branch string
	open   bool // the last turn hasn't finished
	events []model.ClaudeEvent
}

type record struct {
	Type        string  `json:"type"`
	Subtype     string  `json:"subtype"`
	Timestamp   string  `json:"timestamp"`
	ParentUUID  *string `json:"parentUuid"`
	IsMeta      bool    `json:"isMeta"`
	IsSidechain bool    `json:"isSidechain"`
	AITitle     string  `json:"aiTitle"`
	GitBranch   string  `json:"gitBranch"`
	Message     *struct {
		Content    json.RawMessage `json:"content"`
		StopReason string          `json:"stop_reason"`
	} `json:"message"`
}

type block struct {
	Type      string                     `json:"type"`
	Text      string                     `json:"text"`
	ID        string                     `json:"id"`
	Name      string                     `json:"name"`
	Input     map[string]json.RawMessage `json:"input"`
	ToolUseID string                     `json:"tool_use_id"`
	IsError   bool                       `json:"is_error"`
}

// projectDirs returns the transcript directories belonging to this session's
// workdir: the directory itself plus any worktrees Claude created under it.
// Claude names them after the cwd with every non-alphanumeric turned into "-".
func (c *Controller) projectDirs() []string {
	if c.workdir == "" || c.projectsDir == "" {
		return nil
	}
	enc := nonAlnum.ReplaceAllString(c.workdir, "-")
	entries, err := os.ReadDir(c.projectsDir)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && (e.Name() == enc || strings.HasPrefix(e.Name(), enc+"--claude-worktrees-")) {
			dirs = append(dirs, filepath.Join(c.projectsDir, e.Name()))
		}
	}
	return dirs
}

type transcriptFile struct {
	path string
	id   string
	size int64
	mod  time.Time
}

func (c *Controller) transcriptFiles() []transcriptFile {
	var files []transcriptFile
	for _, dir := range c.projectDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			id, ok := strings.CutSuffix(e.Name(), ".jsonl")
			if !ok || !conversationID.MatchString(id) {
				continue
			}
			info, err := e.Info()
			if err != nil || info.Size() == 0 {
				continue
			}
			files = append(files, transcriptFile{filepath.Join(dir, e.Name()), id, info.Size(), info.ModTime()})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	return files
}

// load returns the parsed transcript, re-reading the file only if it changed.
func (c *Controller) load(f transcriptFile) (*transcript, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t := c.transcripts[f.path]; t != nil && t.size == f.size && t.mod.Equal(f.mod) {
		return t, nil
	}
	t, err := parseTranscript(f.path)
	if err != nil {
		return nil, err
	}
	t.size, t.mod = f.size, f.mod
	c.transcripts[f.path] = t
	return t, nil
}

func (t *transcript) busy() bool { return t.open && time.Since(t.mod) < busyWindow }

func (t *transcript) summary(id string) model.ClaudeConversation {
	conv := model.ClaudeConversation{
		ID:        id,
		Title:     t.title,
		Branch:    t.branch,
		UpdatedAt: t.mod.UTC().Format(time.RFC3339),
		Busy:      t.busy(),
	}
	if n := len(t.events); n > 0 {
		e := t.events[n-1]
		conv.Last = e.Text
		if e.Kind == "tool" {
			conv.Last = strings.TrimSuffix(e.Tool+": "+e.Text, ": ")
		}
		conv.Last = clip(oneLine(conv.Last), maxLast)
	}
	return conv
}

// Conversations lists the session's most recently active conversations.
func (c *Controller) Conversations() []model.ClaudeConversation {
	files := c.transcriptFiles()
	if len(files) > maxConversations {
		files = files[:maxConversations]
	}
	out := make([]model.ClaudeConversation, 0, len(files))
	keep := make(map[string]bool, len(files))
	for _, f := range files {
		keep[f.path] = true
		t, err := c.load(f)
		if err != nil {
			continue
		}
		out = append(out, t.summary(f.id))
	}
	// Drop cached transcripts that fell off the list.
	c.mu.Lock()
	for p := range c.transcripts {
		if !keep[p] {
			delete(c.transcripts, p)
		}
	}
	c.mu.Unlock()
	return out
}

// Conversation returns one conversation with its recent events.
func (c *Controller) Conversation(id string) (model.ClaudeConversationDetail, error) {
	if !conversationID.MatchString(id) {
		return model.ClaudeConversationDetail{}, fmt.Errorf("%w: conversation %q", ErrNotFound, id)
	}
	for _, f := range c.transcriptFiles() {
		if f.id != id {
			continue
		}
		t, err := c.load(f)
		if err != nil {
			return model.ClaudeConversationDetail{}, err
		}
		return model.ClaudeConversationDetail{ClaudeConversation: t.summary(id), Events: t.events}, nil
	}
	return model.ClaudeConversationDetail{}, fmt.Errorf("%w: conversation %q", ErrNotFound, id)
}

func parseTranscript(path string) (*transcript, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	t := &transcript{events: []model.ClaudeEvent{}}
	var firstPrompt string
	toolIdx := map[string]int{} // tool_use id -> index in t.events

	// Lines can be megabytes (tool output), so no bufio.Scanner line limit.
	r := bufio.NewReaderSize(file, 256*1024)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			var rec record
			// A line still being written fails to parse; it's picked up next time.
			if json.Unmarshal(line, &rec) == nil {
				t.apply(&rec, toolIdx, &firstPrompt)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}

	if t.title == "" {
		t.title = clip(oneLine(firstPrompt), maxTitle)
	}
	if n := len(t.events); n > maxEvents {
		t.events = t.events[n-maxEvents:]
	}
	return t, nil
}

func (t *transcript) apply(rec *record, toolIdx map[string]int, firstPrompt *string) {
	if rec.AITitle != "" {
		t.title = rec.AITitle
	}
	if rec.GitBranch != "" {
		t.branch = rec.GitBranch
	}
	if rec.IsSidechain {
		return
	}
	switch rec.Type {
	case "system":
		if rec.Subtype == "turn_duration" {
			t.open = false
		}
	case "assistant":
		if rec.Message == nil {
			return
		}
		var blocks []block
		_ = json.Unmarshal(rec.Message.Content, &blocks)
		for _, b := range blocks {
			switch b.Type {
			case "text":
				if s := strings.TrimSpace(b.Text); s != "" {
					t.add(rec, "text", clip(s, maxText), "")
				}
			case "tool_use":
				toolIdx[b.ID] = len(t.events)
				t.add(rec, "tool", clip(oneLine(toolSummary(b.Input)), maxToolText), b.Name)
			}
		}
		switch rec.Message.StopReason {
		case "", "tool_use", "pause_turn":
			t.open = true
		default:
			t.open = false
		}
	case "user":
		if rec.Message == nil {
			return
		}
		var text string
		if json.Unmarshal(rec.Message.Content, &text) != nil {
			var blocks []block
			_ = json.Unmarshal(rec.Message.Content, &blocks)
			var parts []string
			for _, b := range blocks {
				switch b.Type {
				case "text":
					parts = append(parts, b.Text)
				case "tool_result":
					t.open = true
					if i, ok := toolIdx[b.ToolUseID]; ok && b.IsError {
						t.events[i].Failed = true
					}
				}
			}
			text = strings.Join(parts, "\n")
		}
		// Meta records are harness bookkeeping, except a root one: that's the
		// prompt a scheduled or relayed run was started with.
		if rec.IsMeta && rec.ParentUUID != nil {
			return
		}
		if text = userText(text); text == "" {
			return
		}
		t.open = true
		if *firstPrompt == "" {
			*firstPrompt = text
		}
		t.add(rec, "user", clip(text, maxText), "")
	}
}

func (t *transcript) add(rec *record, kind, text, tool string) {
	t.events = append(t.events, model.ClaudeEvent{At: rec.Timestamp, Kind: kind, Text: text, Tool: tool})
}

var commandName = regexp.MustCompile(`<command-name>([^<]+)</command-name>`)

// A remote-control thread delivers what was said wrapped in a <wake> envelope;
// the message that triggered the turn is the part worth showing.
var wakeMessage = regexp.MustCompile(`(?s)<message [^>]*trigger="true"[^>]*>(.*?)</message>`)

// Harness-generated "user" messages that aren't something a person said.
var harnessPrefixes = []string{"<local-command-", "<task-notification", "<system-reminder", "<relay ", "<wake "}

// userText cleans a user message for display, returning "" for harness noise.
func userText(s string) string {
	s = strings.TrimSpace(s)
	if m := commandName.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	if m := wakeMessage.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(html.UnescapeString(m[1]))
	}
	for _, p := range harnessPrefixes {
		if strings.HasPrefix(s, p) {
			return ""
		}
	}
	return s
}

// The input fields that best say what a tool call is doing, most telling first.
var toolSummaryKeys = []string{"description", "command", "file_path", "path", "pattern", "query", "url", "prompt", "skill", "subject"}

func toolSummary(input map[string]json.RawMessage) string {
	for _, k := range toolSummaryKeys {
		var s string
		if raw, ok := input[k]; ok && json.Unmarshal(raw, &s) == nil && s != "" {
			return s
		}
	}
	return ""
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// clip shortens s to at most n runes, marking the cut with an ellipsis.
func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
