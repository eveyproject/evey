package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/domain/user/fakes"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func setupParentalTest(t *testing.T) (*http.ServeMux, *fakes.FakeProfileRepository, *fakes.FakePasswordHasher, *auth.JWTGenerator) {
	t.Helper()
	profileRepo := fakes.NewFakeProfileRepository()
	hasher := fakes.NewFakePasswordHasher()

	key, err := generateTestKey()
	if err != nil {
		t.Fatal(err)
	}
	tokenGen := auth.NewJWTGenerator(key, time.Hour)

	mux := http.NewServeMux()
	registerParentalRoutes(mux, profileRepo, hasher, tokenGen)
	return mux, profileRepo, hasher, tokenGen
}

func generateTestKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, 2048)
}

func createTestToken(t *testing.T, tokenGen *auth.JWTGenerator, profileID string, isAdmin bool) string {
	t.Helper()
	token, err := tokenGen.GenerateAccessToken(domainuser.Claims{
		UserID:    "user-1",
		Username:  "test",
		IsAdmin:   isAdmin,
		ProfileID: profileID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestGetParentalSettings(t *testing.T) {
	mux, profileRepo, _, tokenGen := setupParentalTest(t)

	profile := &domainuser.Profile{
		ID:     "prof-1",
		UserID: "user-1",
		Name:   "Test",
		Parental: &domainuser.ParentalSettings{
			MaxRating:   "PG-13",
			BlockedTags: []string{"horror"},
			PINHash:     "hashed:1234",
			Enabled:     true,
		},
	}
	profileRepo.Create(context.Background(), profile)

	token := createTestToken(t, tokenGen, "prof-1", false)
	req := httptest.NewRequest("GET", "/api/v1/profiles/prof-1/parental", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		MaxRating   string   `json:"maxRating"`
		BlockedTags []string `json:"blockedTags"`
		HasPIN      bool     `json:"hasPin"`
		Enabled     bool     `json:"enabled"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.MaxRating != "PG-13" {
		t.Errorf("maxRating = %q, want PG-13", resp.MaxRating)
	}
	if !resp.HasPIN {
		t.Error("expected hasPin = true")
	}
	if !resp.Enabled {
		t.Error("expected enabled = true")
	}
	if len(resp.BlockedTags) != 1 || resp.BlockedTags[0] != "horror" {
		t.Errorf("blockedTags = %v, want [horror]", resp.BlockedTags)
	}
}

func TestGetParentalSettings_NoParental(t *testing.T) {
	mux, profileRepo, _, tokenGen := setupParentalTest(t)

	profile := &domainuser.Profile{ID: "prof-1", UserID: "user-1", Name: "Test"}
	profileRepo.Create(context.Background(), profile)

	token := createTestToken(t, tokenGen, "prof-1", false)
	req := httptest.NewRequest("GET", "/api/v1/profiles/prof-1/parental", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Enabled bool `json:"enabled"`
		HasPIN  bool `json:"hasPin"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Enabled || resp.HasPIN {
		t.Error("expected disabled parental with no PIN")
	}
}

func TestGetParentalSettings_IDOR(t *testing.T) {
	mux, profileRepo, _, tokenGen := setupParentalTest(t)

	profileRepo.Create(context.Background(), &domainuser.Profile{ID: "prof-1", UserID: "user-1", Name: "A"})
	profileRepo.Create(context.Background(), &domainuser.Profile{ID: "prof-2", UserID: "user-2", Name: "B"})

	token := createTestToken(t, tokenGen, "prof-1", false)
	req := httptest.NewRequest("GET", "/api/v1/profiles/prof-2/parental", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403 for IDOR, got %d", w.Code)
	}
}

func TestUpdateParentalSettings(t *testing.T) {
	mux, profileRepo, _, tokenGen := setupParentalTest(t)

	profileRepo.Create(context.Background(), &domainuser.Profile{ID: "prof-1", UserID: "user-1", Name: "Test"})

	token := createTestToken(t, tokenGen, "prof-1", false)
	maxRating := "R"
	enabled := true
	pin := "9999"
	body, _ := json.Marshal(map[string]any{
		"maxRating":   maxRating,
		"blockedTags": []string{"violence"},
		"pin":         pin,
		"enabled":     enabled,
	})
	req := httptest.NewRequest("PUT", "/api/v1/profiles/prof-1/parental", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	updated, _ := profileRepo.GetByID(context.Background(), "prof-1")
	if updated.Parental == nil {
		t.Fatal("parental settings should be set")
	}
	if updated.Parental.MaxRating != "R" {
		t.Errorf("maxRating = %q, want R", updated.Parental.MaxRating)
	}
	if !updated.Parental.Enabled {
		t.Error("expected enabled = true")
	}
	if updated.Parental.PINHash != "hashed:9999" {
		t.Errorf("PIN hash = %q, want hashed:9999", updated.Parental.PINHash)
	}
}

func TestUpdateParentalSettings_RequiresPIN(t *testing.T) {
	mux, profileRepo, _, tokenGen := setupParentalTest(t)

	profileRepo.Create(context.Background(), &domainuser.Profile{
		ID:     "prof-1",
		UserID: "user-1",
		Name:   "Test",
		Parental: &domainuser.ParentalSettings{
			PINHash: "hashed:1234",
			Enabled: true,
		},
	})

	token := createTestToken(t, tokenGen, "prof-1", false)

	// without current PIN => 403
	body, _ := json.Marshal(map[string]any{"maxRating": "G"})
	req := httptest.NewRequest("PUT", "/api/v1/profiles/prof-1/parental", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("expected 403 without current PIN, got %d", w.Code)
	}

	// with correct current PIN => 200
	body, _ = json.Marshal(map[string]any{"maxRating": "G", "currentPin": "1234"})
	req = httptest.NewRequest("PUT", "/api/v1/profiles/prof-1/parental", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200 with correct PIN, got %d: %s", w.Code, w.Body.String())
	}
}

func TestVerifyPIN(t *testing.T) {
	mux, profileRepo, _, tokenGen := setupParentalTest(t)

	profileRepo.Create(context.Background(), &domainuser.Profile{
		ID:     "prof-1",
		UserID: "user-1",
		Name:   "Test",
		Parental: &domainuser.ParentalSettings{
			PINHash: "hashed:1234",
			Enabled: true,
		},
	})

	token := createTestToken(t, tokenGen, "prof-1", false)

	// correct PIN
	body, _ := json.Marshal(map[string]string{"pin": "1234"})
	req := httptest.NewRequest("POST", "/api/v1/profiles/prof-1/parental/verify-pin", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]bool
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp["valid"] {
		t.Error("expected valid = true")
	}

	// wrong PIN
	body, _ = json.Marshal(map[string]string{"pin": "0000"})
	req = httptest.NewRequest("POST", "/api/v1/profiles/prof-1/parental/verify-pin", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["valid"] {
		t.Error("expected valid = false for wrong PIN")
	}
}

func TestVerifyPIN_NoPINSet(t *testing.T) {
	mux, profileRepo, _, tokenGen := setupParentalTest(t)

	profileRepo.Create(context.Background(), &domainuser.Profile{ID: "prof-1", UserID: "user-1", Name: "Test"})

	token := createTestToken(t, tokenGen, "prof-1", false)
	body, _ := json.Marshal(map[string]string{"pin": "anything"})
	req := httptest.NewRequest("POST", "/api/v1/profiles/prof-1/parental/verify-pin", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp map[string]bool
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp["valid"] {
		t.Error("expected valid = true when no PIN set")
	}
}
