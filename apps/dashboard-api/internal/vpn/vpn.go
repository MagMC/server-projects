// Package vpn restarts the gluetun VPN container and then every container that
// shares its network namespace (network_mode: container:gluetun, which compose
// writes for service:gluetun). Those containers keep the old, dead namespace
// when gluetun restarts and lose their network until they are restarted too,
// so the order matters: gluetun first, wait until it is healthy, then them.
package vpn

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"

	"github.com/magmc/server-projects/dashboard-api/internal/model"
)

const (
	stopTimeoutSec = 20              // grace period before docker kills a container
	gluetunWait    = 3 * time.Minute // VPN connect + healthcheck can be slow
	dependentWait  = 2 * time.Minute // per dependent, after its restart
	jobTimeout     = 8 * time.Minute // whole job, as a backstop
	pollEvery      = 2 * time.Second
)

// ErrBusy means a restart is already in progress.
var ErrBusy = errors.New("a restart is already running")

// Restarter owns the docker client and the state of the latest restart job.
type Restarter struct {
	cli     *client.Client
	gluetun string // container name

	mu  sync.Mutex
	job model.VPNJob
}

func New(gluetun string) (*Restarter, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Restarter{cli: cli, gluetun: gluetun, job: model.VPNJob{State: "idle", Steps: []model.VPNJobStep{}}}, nil
}

// Status reports gluetun, its dependents and the latest job.
func (r *Restarter) Status(ctx context.Context) model.VPNStatus {
	st := model.VPNStatus{Dependents: []model.VPNContainer{}, Job: r.snapshot()}
	g, err := r.cli.ContainerInspect(ctx, r.gluetun)
	if err != nil {
		st.Error = err.Error()
		return st
	}
	gc := summarize(g)
	st.Gluetun = &gc
	deps, err := r.dependents(ctx, g)
	if err != nil {
		st.Error = err.Error()
		return st
	}
	for _, d := range deps {
		st.Dependents = append(st.Dependents, summarize(d))
	}
	return st
}

// Start begins a restart in the background. It returns ErrBusy if one is
// already running; progress is read through Status.
func (r *Restarter) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.job.State == "running" {
		return ErrBusy
	}
	r.job = model.VPNJob{
		State:     "running",
		Step:      "finding containers",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		Steps:     []model.VPNJobStep{},
	}
	go r.run()
	return nil
}

func (r *Restarter) run() {
	ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
	defer cancel()

	g, err := r.cli.ContainerInspect(ctx, r.gluetun)
	if err != nil {
		r.finish(fmt.Errorf("inspect %s: %w", r.gluetun, err))
		return
	}
	deps, err := r.dependents(ctx, g)
	if err != nil {
		r.finish(fmt.Errorf("list containers: %w", err))
		return
	}

	// Only restart dependents that were up; a stopped one was stopped on
	// purpose (or is broken for its own reasons) and is left alone.
	var todo []types.ContainerJSON
	r.update(func(j *model.VPNJob) {
		j.Steps = append(j.Steps, model.VPNJobStep{Name: name(g), State: "pending"})
		for _, d := range deps {
			step := model.VPNJobStep{Name: name(d), State: "pending"}
			if d.State == nil || !(d.State.Running || d.State.Restarting) {
				step.State, step.Note = "skipped", "was not running"
			} else {
				todo = append(todo, d)
			}
			j.Steps = append(j.Steps, step)
		}
	})

	// 1. gluetun, and wait until its own healthcheck says the VPN is up.
	r.setStep(name(g), "restarting", "")
	r.update(func(j *model.VPNJob) { j.Step = "restarting " + name(g) })
	if err := r.restart(ctx, g.ID); err != nil {
		r.setStep(name(g), "failed", err.Error())
		r.finish(fmt.Errorf("restart %s: %w", name(g), err))
		return
	}
	r.setStep(name(g), "waiting", "")
	r.update(func(j *model.VPNJob) { j.Step = "waiting for " + name(g) + " to be healthy" })
	if err := r.waitReady(ctx, g.ID, gluetunWait); err != nil {
		r.setStep(name(g), "failed", err.Error())
		r.finish(fmt.Errorf("%s: %w — dependents not restarted", name(g), err))
		return
	}
	r.setStep(name(g), "ok", "")

	// 2. Everything on its network, in parallel; each rejoins the new namespace.
	r.update(func(j *model.VPNJob) { j.Step = fmt.Sprintf("restarting %d dependents", len(todo)) })
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failed []string
	for _, d := range todo {
		wg.Add(1)
		go func(d types.ContainerJSON) {
			defer wg.Done()
			n := name(d)
			r.setStep(n, "restarting", "")
			err := r.restart(ctx, d.ID)
			if err == nil {
				r.setStep(n, "waiting", "")
				err = r.waitReady(ctx, d.ID, dependentWait)
			}
			if err != nil {
				r.setStep(n, "failed", err.Error())
				mu.Lock()
				failed = append(failed, n)
				mu.Unlock()
				return
			}
			r.setStep(n, "ok", "")
		}(d)
	}
	wg.Wait()
	if len(failed) > 0 {
		r.finish(fmt.Errorf("failed: %s", strings.Join(failed, ", ")))
		return
	}
	r.finish(nil)
}

func (r *Restarter) restart(ctx context.Context, id string) error {
	t := stopTimeoutSec
	return r.cli.ContainerRestart(ctx, id, container.StopOptions{Timeout: &t})
}

// waitReady polls until the container is healthy (or, with no healthcheck,
// still running on two polls in a row). An exit fails at once; "unhealthy" is
// tolerated until the deadline, since gluetun can flap while it connects.
func (r *Restarter) waitReady(ctx context.Context, id string, limit time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	last, upTicks := "starting", 0
	for {
		c, err := r.cli.ContainerInspect(ctx, id)
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("not ready after %s (last: %s)", limit, last)
			}
			return err
		}
		s := c.State
		switch {
		case s == nil:
			return errors.New("no state reported")
		case !s.Running && !s.Restarting:
			return fmt.Errorf("%s (exit code %d)", s.Status, s.ExitCode)
		case s.Health != nil:
			last = s.Health.Status
			if last == "healthy" {
				return nil
			}
		case s.Running && !s.Restarting:
			last = "running"
			if upTicks++; upTicks >= 2 {
				return nil
			}
		default:
			last = s.Status
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("not ready after %s (last: %s)", limit, last)
		case <-time.After(pollEvery):
		}
	}
}

// dependents returns every container (running or not) whose network mode
// points at gluetun, by ID or by name.
func (r *Restarter) dependents(ctx context.Context, g types.ContainerJSON) ([]types.ContainerJSON, error) {
	list, err := r.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}
	want := map[string]bool{"container:" + g.ID: true, "container:" + name(g): true}
	var out []types.ContainerJSON
	for _, c := range list {
		if !want[c.HostConfig.NetworkMode] {
			continue
		}
		d, err := r.cli.ContainerInspect(ctx, c.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func (r *Restarter) snapshot() model.VPNJob {
	r.mu.Lock()
	defer r.mu.Unlock()
	j := r.job
	j.Steps = append(make([]model.VPNJobStep, 0, len(r.job.Steps)), r.job.Steps...)
	return j
}

func (r *Restarter) update(f func(*model.VPNJob)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f(&r.job)
}

func (r *Restarter) setStep(n, state, note string) {
	r.update(func(j *model.VPNJob) {
		for i := range j.Steps {
			if j.Steps[i].Name == n {
				j.Steps[i].State, j.Steps[i].Note = state, note
			}
		}
	})
}

func (r *Restarter) finish(err error) {
	r.update(func(j *model.VPNJob) {
		j.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		if err != nil {
			j.State, j.Step = "failed", err.Error()
		} else {
			j.State, j.Step = "done", "all containers back up"
		}
	})
}

func name(c types.ContainerJSON) string {
	if c.ContainerJSONBase == nil {
		return ""
	}
	return strings.TrimPrefix(c.Name, "/")
}

func summarize(c types.ContainerJSON) model.VPNContainer {
	out := model.VPNContainer{Name: name(c)}
	if c.ContainerJSONBase != nil && c.State != nil {
		out.State = c.State.Status
		if c.State.Health != nil {
			out.Health = c.State.Health.Status
		}
	}
	return out
}
