package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime"
	"sort"

	"github.com/evey-project/evey/internal/pkg/procfork"
	"github.com/evey-project/evey/internal/pkg/sysinfo"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// systemMetrics is the finer-grained picture the health page adds to the
// whole-host CPU and memory percentages: how loaded the box is (load
// average against the CPUs evey may use), what evey's own process weighs,
// and what the media tools it forked cost right now, by class.
type systemMetrics struct {
	Load    loadMetrics    `json:"load"`
	Memory  memoryMetrics  `json:"memory"`
	Process processMetrics `json:"process"`
	Forks   forkMetrics    `json:"forks"`
}

type loadMetrics struct {
	Avg1  float64 `json:"avg1"`
	Avg5  float64 `json:"avg5"`
	Avg15 float64 `json:"avg15"`
	// Known is false when /proc/loadavg is unreadable; the averages are then 0.
	Known bool `json:"known"`
	// CPUs is what the host has, CPUBudget what the cgroup lets evey use.
	CPUs      int `json:"cpus"`
	CPUBudget int `json:"cpuBudget"`
}

type memoryMetrics struct {
	TotalBytes     int64 `json:"totalBytes"`
	AvailableBytes int64 `json:"availableBytes"`
	// BudgetBytes is the cgroup limit when the container is capped below the
	// host, else 0.
	BudgetBytes int64 `json:"budgetBytes"`
}

type processMetrics struct {
	RSSBytes     int64  `json:"rssBytes"`
	HeapInUse    uint64 `json:"heapInUseBytes"`
	HeapSys      uint64 `json:"heapSysBytes"`
	Goroutines   int    `json:"goroutines"`
	GCCount      uint32 `json:"gcCount"`
	GCPauseTotal uint64 `json:"gcPauseTotalNs"`
}

type forkMetrics struct {
	Count      int              `json:"count"`
	RSSBytes   int64            `json:"rssBytes"`
	CPUPercent float64          `json:"cpuPercent"`
	ByClass    []forkClassGroup `json:"byClass"`
	Children   []procfork.Child `json:"children"`
}

type forkClassGroup struct {
	Class      string  `json:"class"`
	Count      int     `json:"count"`
	RSSBytes   int64   `json:"rssBytes"`
	CPUPercent float64 `json:"cpuPercent"`
}

func collectSystemMetrics() systemMetrics {
	var m systemMetrics
	m.Load.Avg1, m.Load.Avg5, m.Load.Avg15, m.Load.Known = sysinfo.LoadAverage()
	m.Load.CPUs = runtime.NumCPU()
	m.Load.CPUBudget = sysinfo.CPUBudget()

	mi := sysinfo.ReadMemInfo()
	m.Memory = memoryMetrics{TotalBytes: mi.TotalBytes, AvailableBytes: mi.AvailableBytes, BudgetBytes: mi.BudgetBytes}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	m.Process = processMetrics{
		RSSBytes:     sysinfo.SelfRSSBytes(),
		HeapInUse:    ms.HeapInuse,
		HeapSys:      ms.HeapSys,
		Goroutines:   runtime.NumGoroutine(),
		GCCount:      ms.NumGC,
		GCPauseTotal: ms.PauseTotalNs,
	}

	children := procfork.Children()
	m.Forks.Children = children
	m.Forks.Count = len(children)
	groups := map[string]*forkClassGroup{}
	for _, c := range children {
		m.Forks.RSSBytes += c.RSSBytes
		m.Forks.CPUPercent += c.CPUPercent
		g := groups[c.Class]
		if g == nil {
			g = &forkClassGroup{Class: c.Class}
			groups[c.Class] = g
		}
		g.Count++
		g.RSSBytes += c.RSSBytes
		g.CPUPercent += c.CPUPercent
	}
	m.Forks.ByClass = make([]forkClassGroup, 0, len(groups))
	for _, g := range groups {
		m.Forks.ByClass = append(m.Forks.ByClass, *g)
	}
	sort.Slice(m.Forks.ByClass, func(i, j int) bool { return m.Forks.ByClass[i].Class < m.Forks.ByClass[j].Class })
	return m
}

// registerSystemMetricsRoute mounts GET /api/v1/admin/system/metrics, the
// admin-only, read-only, in-process metrics behind the health page. Nothing
// here touches the database or forks anything, so it is safe to poll.
func registerSystemMetricsRoute(mux *http.ServeMux, tokenGen TokenVerifier) {
	admin := httpAdminMiddleware(tokenGen)
	mux.HandleFunc("GET /api/v1/admin/system/metrics", admin(func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.admin.SystemMetrics")
		defer span.End()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(collectSystemMetrics()); err != nil {
			slog.WarnContext(r.Context(), "system-metrics: encode failed", slog.Any("err", err))
		}
	}))
}
