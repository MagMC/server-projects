package collect

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/magmc/server-projects/dashboard-api/internal/config"
	"github.com/magmc/server-projects/dashboard-api/internal/model"
)

// HostCollector reads host stats from sysfs/procfs mounts.
type HostCollector struct {
	cfg config.Config
	cpu *CPUSampler
}

func NewHostCollector(cfg config.Config, cpu *CPUSampler) *HostCollector {
	return &HostCollector{cfg: cfg, cpu: cpu}
}

func (h *HostCollector) Collect() model.HostStats {
	mem := h.readMem()
	zones, cpuTemp := h.readThermal()
	return model.HostStats{
		CPUPct:       round1(h.cpu.Pct()),
		Mem:          mem,
		Disks:        h.readDisks(),
		UptimeSec:    h.readUptime(),
		CPUTempC:     cpuTemp,
		ThermalZones: zones,
		CollectedAt:  time.Now().UTC().Format(time.RFC3339),
	}
}

func (h *HostCollector) readMem() model.Mem {
	vals := map[string]uint64{}
	f, err := os.Open(filepath.Join(h.cfg.HostProc, "meminfo"))
	if err != nil {
		return model.Mem{}
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.Fields(sc.Text())
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSuffix(parts[0], ":")
		kb, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			continue
		}
		vals[key] = kb * 1024 // meminfo is in kB
	}
	total := vals["MemTotal"]
	avail := vals["MemAvailable"]
	used := uint64(0)
	if total > avail {
		used = total - avail
	}
	var pct float64
	if total > 0 {
		pct = round1(100 * float64(used) / float64(total))
	}
	return model.Mem{TotalBytes: total, UsedBytes: used, UsedPct: pct}
}

func (h *HostCollector) readUptime() int64 {
	b, err := os.ReadFile(filepath.Join(h.cfg.HostProc, "uptime"))
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return 0
	}
	secs, _ := strconv.ParseFloat(fields[0], 64)
	return int64(secs)
}

func (h *HostCollector) readDisks() []model.DiskUsage {
	out := make([]model.DiskUsage, 0, len(h.cfg.DiskMounts))
	for _, m := range h.cfg.DiskMounts {
		var st syscall.Statfs_t
		if err := syscall.Statfs(m, &st); err != nil {
			continue
		}
		bs := uint64(st.Bsize)
		total := st.Blocks * bs
		free := st.Bavail * bs
		used := total - st.Bfree*bs
		var pct float64
		if total > 0 {
			pct = round1(100 * float64(used) / float64(total))
		}
		out = append(out, model.DiskUsage{
			Mount:      diskLabel(m),
			TotalBytes: total,
			UsedBytes:  used,
			FreeBytes:  free,
			UsedPct:    pct,
		})
	}
	return out
}

// readThermal returns all zones and, separately, the x86_pkg_temp (CPU package)
// reading if present.
func (h *HostCollector) readThermal() ([]model.ThermalZone, *float64) {
	base := filepath.Join(h.cfg.HostSys, "class", "thermal")
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, nil
	}
	var zones []model.ThermalZone
	var cpuTemp *float64
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "thermal_zone") {
			continue
		}
		zoneDir := filepath.Join(base, e.Name())
		typ := strings.TrimSpace(readFileStr(filepath.Join(zoneDir, "type")))
		milli, err := strconv.ParseInt(strings.TrimSpace(readFileStr(filepath.Join(zoneDir, "temp"))), 10, 64)
		if err != nil {
			continue
		}
		c := round1(float64(milli) / 1000)
		zones = append(zones, model.ThermalZone{Type: typ, TempC: c})
		if typ == "x86_pkg_temp" {
			v := c
			cpuTemp = &v
		}
	}
	return zones, cpuTemp
}

// diskLabel turns the in-container mount path back into a host-friendly label,
// e.g. /host/root -> "/", /host/data -> "/data".
func diskLabel(mount string) string {
	if mount == "/host/root" {
		return "/"
	}
	if s := strings.TrimPrefix(mount, "/host"); s != mount && s != "" {
		return s
	}
	return mount
}

func readFileStr(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func round1(f float64) float64 {
	return float64(int64(f*10+0.5)) / 10
}
