// Package model holds the JSON response shapes the API serves. The frontend
// hand-mirrors these in apps/dashboard/src/types.ts.
package model

// ThermalZone is one /sys/class/thermal entry.
type ThermalZone struct {
	Type  string  `json:"type"`
	TempC float64 `json:"tempC"`
}

// DiskUsage is one mounted filesystem.
type DiskUsage struct {
	Mount      string  `json:"mount"`
	Device     string  `json:"device"` // e.g. /dev/nvme0n1p1, "" if unknown
	TotalBytes uint64  `json:"totalBytes"`
	UsedBytes  uint64  `json:"usedBytes"`
	FreeBytes  uint64  `json:"freeBytes"`
	UsedPct    float64 `json:"usedPct"`
}

// Mem is host memory usage.
type Mem struct {
	TotalBytes uint64  `json:"totalBytes"`
	UsedBytes  uint64  `json:"usedBytes"`
	UsedPct    float64 `json:"usedPct"`
}

// HostStats is the /host response.
type HostStats struct {
	CPUPct       float64       `json:"cpuPct"`
	Mem          Mem           `json:"mem"`
	Disks        []DiskUsage   `json:"disks"`
	UptimeSec    int64         `json:"uptimeSec"`
	CPUTempC     *float64      `json:"cpuTempC"` // x86_pkg_temp; null if unavailable
	ThermalZones []ThermalZone `json:"thermalZones"`
	CollectedAt  string        `json:"collectedAt"`
}

// ContainerInfo is one Docker container in /docker/containers.
type ContainerInfo struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Image         string   `json:"image"`
	State         string   `json:"state"`  // running | exited | ...
	Status        string   `json:"status"` // "Up 3 hours"
	CPUPct        *float64 `json:"cpuPct"`
	MemUsedBytes  *uint64  `json:"memUsedBytes"`
	MemLimitBytes *uint64  `json:"memLimitBytes"`
}

// K3sNode is one cluster node.
type K3sNode struct {
	Name             string  `json:"name"`
	Ready            bool    `json:"ready"`
	CPUMilli         *int64  `json:"cpuMilli"`
	CPUCapacityMilli *int64  `json:"cpuCapacityMilli"`
	MemUsedBytes     *uint64 `json:"memUsedBytes"`
	MemCapacityBytes *uint64 `json:"memCapacityBytes"`
}

// K3sPod is one pod across any namespace.
type K3sPod struct {
	Namespace    string  `json:"namespace"`
	Name         string  `json:"name"`
	Phase        string  `json:"phase"`
	Ready        string  `json:"ready"` // "1/1"
	Restarts     int32   `json:"restarts"`
	CPUMilli     *int64  `json:"cpuMilli"`
	MemUsedBytes *uint64 `json:"memUsedBytes"`
}

// K3sData is the /k3s response.
type K3sData struct {
	Nodes []K3sNode `json:"nodes"`
	Pods  []K3sPod  `json:"pods"`
}

// ClaudeStatus is one entry of the /claude/status response: a Claude Code tmux
// session on the host.
type ClaudeStatus struct {
	Session        string `json:"session"`        // tmux session name
	Available      bool   `json:"available"`      // host tmux server reachable
	Exists         bool   `json:"exists"`         // session exists
	Running        bool   `json:"running"`        // claude is the pane's foreground process
	Command        string `json:"command"`        // pane's foreground process (bash, claude, ...)
	Path           string `json:"path"`           // pane's working directory
	CreatedAt      int64  `json:"createdAt"`      // unix seconds
	Attached       int    `json:"attached"`       // tmux clients attached
	Width          int    `json:"width"`          // pane columns
	Height         int    `json:"height"`         // pane rows
	ControlEnabled bool   `json:"controlEnabled"` // a control token is configured
	Error          string `json:"error,omitempty"`
}

// ClaudeConversation is one entry of /claude/{session}/conversations: a Claude
// Code conversation run from the session's workdir, read from its transcript.
type ClaudeConversation struct {
	ID        string `json:"id"`
	Title     string `json:"title"`     // Claude's own title, else the first prompt
	Branch    string `json:"branch"`    // git branch it last worked on, "" if none
	UpdatedAt string `json:"updatedAt"` // RFC 3339, last transcript write
	Busy      bool   `json:"busy"`      // a turn is in progress
	Last      string `json:"last"`      // one-line summary of the newest event
}

// ClaudeEvent is one thing that happened in a conversation.
type ClaudeEvent struct {
	At     string `json:"at"`               // RFC 3339
	Kind   string `json:"kind"`             // user | text | tool
	Text   string `json:"text"`             // prompt, reply, or what the tool call does
	Tool   string `json:"tool,omitempty"`   // tool name, for kind "tool"
	Failed bool   `json:"failed,omitempty"` // the tool call returned an error
}

// ClaudeConversationDetail is the /claude/{session}/conversations/{id} response.
type ClaudeConversationDetail struct {
	ClaudeConversation
	Events []ClaudeEvent `json:"events"` // newest last, capped
}

// ClaudeScreen is the /claude/{session}/screen response.
type ClaudeScreen struct {
	Text  string `json:"text"` // pane contents incl. scrollback, with ANSI escapes
	Width int    `json:"width"`
}

// VPNContainer is gluetun or one container sharing its network namespace
// (network_mode: container:gluetun / service:gluetun).
type VPNContainer struct {
	Name   string `json:"name"`
	State  string `json:"state"`  // running | exited | restarting | ...
	Health string `json:"health"` // healthy | unhealthy | starting | "" (no healthcheck)
}

// VPNJobStep is one container's progress through a restart.
type VPNJobStep struct {
	Name  string `json:"name"`
	State string `json:"state"` // pending | restarting | waiting | ok | failed | skipped
	Note  string `json:"note,omitempty"`
}

// VPNJob is the latest gluetun stack restart (state "idle" if none has run).
type VPNJob struct {
	State      string       `json:"state"` // idle | running | done | failed
	Step       string       `json:"step"`  // what it's doing / how it ended
	StartedAt  string       `json:"startedAt,omitempty"`
	FinishedAt string       `json:"finishedAt,omitempty"`
	Steps      []VPNJobStep `json:"steps"` // gluetun first, then its dependents
}

// VPNStatus is the /vpn response.
type VPNStatus struct {
	Gluetun        *VPNContainer  `json:"gluetun"` // null if the container is missing
	Dependents     []VPNContainer `json:"dependents"`
	Job            VPNJob         `json:"job"`
	ControlEnabled bool           `json:"controlEnabled"`
	Error          string         `json:"error,omitempty"`
}
