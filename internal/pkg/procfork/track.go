package procfork

import (
	"os/exec"
	"sort"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/pkg/sysinfo"
)

// Child is one forked process still running, with what it costs right now.
// Task is the background task it works for (bgtasks.TaskFromContext on the
// context it was forked under), "" for a fork nobody attributed (a viewer's
// transcode, a probe on a request path).
type Child struct {
	Site       string    `json:"site"`
	Class      string    `json:"class"`
	Task       string    `json:"task,omitempty"`
	PID        int       `json:"pid"`
	StartedAt  time.Time `json:"startedAt"`
	RSSBytes   int64     `json:"rssBytes"`
	CPUPercent float64   `json:"cpuPercent"`
}

// TaskTotals is what a task's forks have cost since the server started.
type TaskTotals struct {
	Forks      int64   `json:"forks"`
	CPUSeconds float64 `json:"cpuSeconds"`
}

type flight struct {
	cmd     *exec.Cmd
	site    string
	class   Class
	task    string
	started time.Time
}

// cpuSample is the last CPU-time reading of one pid, so the next Children
// call can turn the delta into a percentage.
type cpuSample struct {
	seconds float64
	at      time.Time
}

// tracker is the live registry of forks: added by Command, dropped by Finish.
// The pid is read lazily off exec.Cmd, which only knows it after Start - the
// callers own the Start, so there is no hook to catch it earlier.
type tracker struct {
	mu      sync.Mutex
	flights map[*exec.Cmd]*flight
	cpu     map[int]cpuSample
	totals  map[string]*TaskTotals
}

var track = &tracker{
	flights: make(map[*exec.Cmd]*flight),
	cpu:     make(map[int]cpuSample),
	totals:  make(map[string]*TaskTotals),
}

func (t *tracker) add(cmd *exec.Cmd, f *flight) {
	t.mu.Lock()
	t.flights[cmd] = f
	t.mu.Unlock()
}

// remove drops the flight and charges its CPU time to its task.
func (t *tracker) remove(cmd *exec.Cmd, cpuSeconds float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	f := t.flights[cmd]
	delete(t.flights, cmd)
	if f == nil {
		return
	}
	if cmd.Process != nil {
		delete(t.cpu, cmd.Process.Pid)
	}
	tot := t.totals[f.task]
	if tot == nil {
		tot = &TaskTotals{}
		t.totals[f.task] = tot
	}
	tot.Forks++
	tot.CPUSeconds += cpuSeconds
}

// children samples every running fork. CPU percent is over the interval
// since the previous call for that pid (0 on the first sight of a pid), on a
// single-core scale: 200 means two cores busy.
func (t *tracker) children(now time.Time) []Child {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Child, 0, len(t.flights))
	for cmd, f := range t.flights {
		if cmd.Process == nil {
			continue // built, not started yet
		}
		pid := cmd.Process.Pid
		c := Child{Site: f.site, Class: f.class.String(), Task: f.task, PID: pid, StartedAt: f.started}
		if rss, err := sysinfo.ProcRSSBytes(pid); err == nil {
			c.RSSBytes = rss
		}
		if secs, err := sysinfo.ProcCPUSeconds(pid); err == nil {
			if prev, ok := t.cpu[pid]; ok {
				if dt := now.Sub(prev.at).Seconds(); dt > 0 && secs >= prev.seconds {
					c.CPUPercent = (secs - prev.seconds) / dt * 100
				}
			}
			t.cpu[pid] = cpuSample{seconds: secs, at: now}
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	return out
}

func (t *tracker) taskTotals() map[string]TaskTotals {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]TaskTotals, len(t.totals))
	for k, v := range t.totals {
		out[k] = *v
	}
	return out
}

// taskCPUSeconds is, per task tag, the CPU time of its finished forks plus
// what its live forks have consumed so far: a counter that only grows, so
// two readings give the task's CPU rate - short forks included, which a live
// sample of the running children never sees.
func (t *tracker) taskCPUSeconds() map[string]float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]float64, len(t.totals))
	for k, v := range t.totals {
		out[k] = v.CPUSeconds
	}
	for cmd, f := range t.flights {
		if cmd.Process == nil {
			continue
		}
		if secs, err := sysinfo.ProcCPUSeconds(cmd.Process.Pid); err == nil {
			out[f.task] += secs
		}
	}
	return out
}

// TaskCPUSeconds reports the monotonic CPU time of every task's forks.
func TaskCPUSeconds() map[string]float64 { return track.taskCPUSeconds() }

// Children lists the forks running right now with their live cost.
func Children() []Child { return track.children(time.Now()) }

// Totals reports, per task tag, the forks completed and the CPU seconds they
// consumed since the server started. The "" key holds the unattributed forks.
func Totals() map[string]TaskTotals { return track.taskTotals() }
