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
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Image       string   `json:"image"`
	State       string   `json:"state"`  // running | exited | ...
	Status      string   `json:"status"` // "Up 3 hours"
	CPUPct      *float64 `json:"cpuPct"`
	MemUsedBytes  *uint64 `json:"memUsedBytes"`
	MemLimitBytes *uint64 `json:"memLimitBytes"`
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
