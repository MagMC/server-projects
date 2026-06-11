package collect

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"

	"github.com/magmc/server-projects/dashboard-api/internal/model"
)

// DockerCollector lists running containers and computes per-container CPU/mem.
type DockerCollector struct {
	cli *client.Client
}

func NewDockerCollector() (*DockerCollector, error) {
	// FromEnv honours DOCKER_HOST (the unix socket).
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &DockerCollector{cli: cli}, nil
}

// statsRaw is just the subset of docker's stats JSON we need, so we don't depend
// on the SDK's churning stats struct names across versions.
type statsRaw struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs  uint32 `json:"online_cpus"`
	} `json:"cpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
		Stats struct {
			Cache        uint64 `json:"cache"`
			InactiveFile uint64 `json:"inactive_file"`
		} `json:"stats"`
	} `json:"memory_stats"`
}

func (d *DockerCollector) Collect(ctx context.Context) ([]model.ContainerInfo, error) {
	list, err := d.cli.ContainerList(ctx, container.ListOptions{All: false})
	if err != nil {
		return nil, err
	}

	ids := make([]string, len(list))
	for i, c := range list {
		ids[i] = c.ID
	}

	// Two snapshots 250ms apart -> valid CPU delta without waiting for the
	// 1s stats stream tick.
	a := d.statsAll(ctx, ids)
	time.Sleep(250 * time.Millisecond)
	b := d.statsAll(ctx, ids)

	out := make([]model.ContainerInfo, 0, len(list))
	for _, c := range list {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		info := model.ContainerInfo{
			ID:     c.ID[:min(12, len(c.ID))],
			Name:   name,
			Image:  c.Image,
			State:  c.State,
			Status: c.Status,
		}
		if s1, ok := a[c.ID]; ok {
			if s2, ok2 := b[c.ID]; ok2 {
				if pct, ok := cpuPct(s1, s2); ok {
					info.CPUPct = &pct
				}
				used := memUsed(s2)
				info.MemUsedBytes = &used
				if s2.MemoryStats.Limit > 0 {
					lim := s2.MemoryStats.Limit
					info.MemLimitBytes = &lim
				}
			}
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (d *DockerCollector) statsAll(ctx context.Context, ids []string) map[string]statsRaw {
	res := make(map[string]statsRaw, len(ids))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			resp, err := d.cli.ContainerStatsOneShot(ctx, id)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			var s statsRaw
			if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
				return
			}
			mu.Lock()
			res[id] = s
			mu.Unlock()
		}(id)
	}
	wg.Wait()
	return res
}

func cpuPct(a, b statsRaw) (float64, bool) {
	cpuDelta := float64(b.CPUStats.CPUUsage.TotalUsage) - float64(a.CPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(b.CPUStats.SystemUsage) - float64(a.CPUStats.SystemUsage)
	if sysDelta <= 0 || cpuDelta < 0 {
		return 0, false
	}
	cpus := float64(b.CPUStats.OnlineCPUs)
	if cpus == 0 {
		cpus = 1
	}
	return round1(cpuDelta / sysDelta * cpus * 100), true
}

func memUsed(s statsRaw) uint64 {
	// Exclude page cache so the number matches "real" usage. cgroup v1 reports
	// "cache"; cgroup v2 reports "inactive_file".
	cache := s.MemoryStats.Stats.Cache
	if cache == 0 {
		cache = s.MemoryStats.Stats.InactiveFile
	}
	if s.MemoryStats.Usage > cache {
		return s.MemoryStats.Usage - cache
	}
	return s.MemoryStats.Usage
}
