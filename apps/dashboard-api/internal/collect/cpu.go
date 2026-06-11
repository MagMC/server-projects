package collect

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CPUSampler keeps a rolling host CPU% by re-reading /proc/stat in the
// background, so HTTP handlers never block on a two-sample delta.
type CPUSampler struct {
	statPath string
	mu       sync.RWMutex
	pct      float64
	haveLast bool
	lastIdle uint64
	lastBusy uint64
}

func NewCPUSampler(hostProc string) *CPUSampler {
	return &CPUSampler{statPath: filepath.Join(hostProc, "stat")}
}

// Run samples every interval until ctx-less stop; intended to run in a goroutine
// for the life of the process.
func (s *CPUSampler) Run(interval time.Duration) {
	s.sample() // prime
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		s.sample()
	}
}

// Pct returns the most recent host CPU utilisation percentage.
func (s *CPUSampler) Pct() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pct
}

func (s *CPUSampler) sample() {
	idle, busy, ok := readCPULine(s.statPath)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.haveLast {
		dIdle := float64(idle - s.lastIdle)
		dBusy := float64(busy - s.lastBusy)
		total := dIdle + dBusy
		if total > 0 {
			s.pct = 100 * dBusy / total
		}
	}
	s.lastIdle, s.lastBusy, s.haveLast = idle, busy, true
}

// readCPULine parses the aggregate "cpu " line of /proc/stat into idle and busy
// jiffies. Fields: user nice system idle iowait irq softirq steal guest...
func readCPULine(path string) (idle, busy uint64, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)[1:]
		var vals []uint64
		for _, fld := range fields {
			v, err := strconv.ParseUint(fld, 10, 64)
			if err != nil {
				break
			}
			vals = append(vals, v)
		}
		if len(vals) < 5 {
			return 0, 0, false
		}
		// idle = idle + iowait (fields 3,4); busy = everything else.
		idle = vals[3] + vals[4]
		var total uint64
		for _, v := range vals {
			total += v
		}
		busy = total - idle
		return idle, busy, true
	}
	return 0, 0, false
}
