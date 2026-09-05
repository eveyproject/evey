package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	appactivity "github.com/evey-project/evey/internal/app/activity"
	"github.com/evey-project/evey/internal/pkg/bgtasks"
	"github.com/evey-project/evey/internal/pkg/procfork"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// backgroundTaskView is one registry entry plus what its forked processes
// cost: right now (the children still running under it) and since the server
// started. A worker's own goroutine cost is not attributable and is not
// claimed; the media tools it forks are where the CPU and memory go.
type backgroundTaskView struct {
	bgtasks.Task
	// Processes is how many forks run for this task at this instant.
	Processes int `json:"processes"`
	// RSSBytes is the summed resident memory of those forks.
	RSSBytes int64 `json:"rssBytes"`
	// CPUPercent is their summed CPU share on a single-core scale (200 = two
	// cores busy), over the interval since the previous poll.
	CPUPercent float64 `json:"cpuPercent"`
	// CPUSecondsTotal is the CPU time every fork of this task consumed since
	// the server started; Forks counts them.
	CPUSecondsTotal float64 `json:"cpuSecondsTotal"`
	Forks           int64   `json:"forks"`
}

func backgroundTaskViews() []backgroundTaskView {
	tasks := bgtasks.Snapshot()
	children := procfork.Children()
	totals := procfork.Totals()
	views := make([]backgroundTaskView, 0, len(tasks))
	for _, t := range tasks {
		v := backgroundTaskView{Task: t}
		for _, c := range children {
			if c.Task != t.Name {
				continue
			}
			v.Processes++
			v.RSSBytes += c.RSSBytes
			v.CPUPercent += c.CPUPercent
		}
		if tot, ok := totals[t.Name]; ok {
			v.CPUSecondsTotal = tot.CPUSeconds
			v.Forks = tot.Forks
		}
		views = append(views, v)
	}
	return views
}

// registerBackgroundTasksRoute mounts the background-task surface:
//
//	GET  /api/v1/admin/background-tasks               snapshot with live cost
//	POST /api/v1/admin/background-tasks/{name}/stop   pause + cancel the item in flight
//	POST /api/v1/admin/background-tasks/{name}/resume
//
// Admin-only. The snapshot is sourced from the in-process registries, so it
// is always cheap and never 5xx. Stop and resume answer 404 for a name the
// registry does not know and 409 for a worker that does not honour them.
func registerBackgroundTasksRoute(mux *http.ServeMux, tokenGen TokenVerifier, actCmdSvc *appactivity.CommandService) {
	admin := httpAdminMiddleware(tokenGen)
	//nolint:exhaustruct_v5 // only the audit dependency is needed here
	h := &adminHandler{activityCmdSvc: actCmdSvc}
	mux.HandleFunc("GET /api/v1/admin/background-tasks", admin(func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.admin.BackgroundTasks")
		defer span.End()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(backgroundTaskViews()); err != nil {
			slog.WarnContext(r.Context(), "background-tasks: encode failed", slog.Any("err", err))
		}
	}))
	control := func(action string, apply func(string) bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			_, span := tracing.StartSpan(r.Context(), "http.admin.BackgroundTask."+action)
			defer span.End()
			name := r.PathValue("name")
			known := false
			for _, t := range bgtasks.Snapshot() {
				if t.Name == name {
					known = true
					break
				}
			}
			if !known {
				http.Error(w, `{"message":"unknown background task"}`, http.StatusNotFound)
				return
			}
			if !apply(name) {
				http.Error(w, `{"message":"this background task cannot be controlled"}`, http.StatusConflict)
				return
			}
			slog.InfoContext(r.Context(), "background task "+action, slog.String("task", name))
			h.audit(r, "background_task."+action, "background_task", &name, nil)
			w.WriteHeader(http.StatusNoContent)
		}
	}
	mux.HandleFunc("POST /api/v1/admin/background-tasks/{name}/stop", admin(control("stop", bgtasks.Stop)))
	mux.HandleFunc("POST /api/v1/admin/background-tasks/{name}/resume", admin(control("resume", bgtasks.Resume)))
}
