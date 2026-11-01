package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	applibrary "github.com/eveyproject/evey/internal/app/library"
	"github.com/eveyproject/evey/internal/app/prunehold"
)

type fakePruneController struct {
	mu        sync.Mutex
	suspended []applibrary.PruneSuspension
	confirmed []string
	scanned   chan string
}

func (f *fakePruneController) PruneSuspensions() []applibrary.PruneSuspension {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.suspended
}

func (f *fakePruneController) ConfirmPrune(libraryID, root, snapshot string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.suspended {
		if s.LibraryID == libraryID && s.Root == root {
			if s.Snapshot != snapshot {
				return prunehold.ErrSnapshotChanged
			}
			f.confirmed = append(f.confirmed, libraryID+":"+root)
			return nil
		}
	}
	return prunehold.ErrNothingSuspended
}

func (f *fakePruneController) ScanLibrary(_ context.Context, libraryID string) (int, int, error) {
	f.scanned <- libraryID
	return 0, 0, nil
}

func newPruneController() *fakePruneController {
	return &fakePruneController{
		suspended: []applibrary.PruneSuspension{{LibraryID: "lib-1", Root: "/pool", Missing: 30, Total: 90, Snapshot: "snap-1"}},
		scanned:   make(chan string, 1),
	}
}

func confirmReq(token, id string) (*httptest.ResponseRecorder, *http.Request) {
	return confirmReqBody(token, id, `{"root":"/pool","snapshot":"snap-1"}`)
}

func confirmReqBody(token, id, body string) (*httptest.ResponseRecorder, *http.Request) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/libraries/"+id+"/prune/confirm", strings.NewReader(body))
	req.SetPathValue("id", id)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return httptest.NewRecorder(), req
}

func TestPruneConfirmHandler_NonAdmin_403(t *testing.T) {
	gen, _, userTok := segAdminTokens(t)
	ctrl := newPruneController()
	h := httpAdminMiddleware(gen)(pruneConfirmHandler(ctrl, context.Background(), nil))
	w, req := confirmReq(userTok, "lib-1")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", w.Code)
	}
	if len(ctrl.confirmed) != 0 {
		t.Fatalf("a non-admin armed the override")
	}
}

func TestPruneConfirmHandler_Admin_ArmsAndScans(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	ctrl := newPruneController()
	h := httpAdminMiddleware(gen)(pruneConfirmHandler(ctrl, context.Background(), nil))
	w, req := confirmReq(adminTok, "lib-1")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d", w.Code)
	}
	select {
	case id := <-ctrl.scanned:
		if id != "lib-1" {
			t.Fatalf("scanned %q", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("scan was not triggered")
	}
	if len(ctrl.confirmed) != 1 || ctrl.confirmed[0] != "lib-1:/pool" {
		t.Fatalf("override not armed: %v", ctrl.confirmed)
	}
}

func TestPruneConfirmHandler_NothingSuspended_409(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	ctrl := newPruneController()
	h := httpAdminMiddleware(gen)(pruneConfirmHandler(ctrl, context.Background(), nil))
	w, req := confirmReq(adminTok, "lib-2")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d", w.Code)
	}
	select {
	case <-ctrl.scanned:
		t.Fatal("scan triggered with nothing to confirm")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestPruneConfirmHandler_StaleSnapshotOrOtherRoot_409NoScan(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	for name, body := range map[string]string{
		"stale snapshot": `{"root":"/pool","snapshot":"snap-old"}`,
		"other root":     `{"root":"/other","snapshot":"snap-1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			ctrl := newPruneController()
			h := httpAdminMiddleware(gen)(pruneConfirmHandler(ctrl, context.Background(), nil))
			w, req := confirmReqBody(adminTok, "lib-1", body)
			h.ServeHTTP(w, req)
			if w.Code != http.StatusConflict {
				t.Fatalf("want 409, got %d", w.Code)
			}
			if len(ctrl.confirmed) != 0 {
				t.Fatalf("armed: %v", ctrl.confirmed)
			}
			select {
			case <-ctrl.scanned:
				t.Fatal("scan triggered on a rejected confirmation")
			case <-time.After(50 * time.Millisecond):
			}
		})
	}
}

func TestPruneConfirmHandler_NoSnapshot_400(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	ctrl := newPruneController()
	h := httpAdminMiddleware(gen)(pruneConfirmHandler(ctrl, context.Background(), nil))
	w, req := confirmReqBody(adminTok, "lib-1", `{}`)
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
	if len(ctrl.confirmed) != 0 {
		t.Fatalf("armed without a snapshot")
	}
}

func TestPruneSuspensionsHandler_AdminOnly_ListsRoots(t *testing.T) {
	gen, adminTok, userTok := segAdminTokens(t)
	h := httpAdminMiddleware(gen)(pruneSuspensionsHandler(newPruneController()))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/libraries/prune-suspended", nil)
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/libraries/prune-suspended", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var body struct {
		Suspensions []struct {
			LibraryID string `json:"libraryId"`
			Root      string `json:"root"`
			Missing   int    `json:"missing"`
			Total     int    `json:"total"`
			Snapshot  string `json:"snapshot"`
		} `json:"suspensions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Suspensions) != 1 || body.Suspensions[0].Root != "/pool" || body.Suspensions[0].Missing != 30 || body.Suspensions[0].Total != 90 || body.Suspensions[0].LibraryID != "lib-1" || body.Suspensions[0].Snapshot != "snap-1" {
		t.Fatalf("unexpected body %s", w.Body.String())
	}
}
