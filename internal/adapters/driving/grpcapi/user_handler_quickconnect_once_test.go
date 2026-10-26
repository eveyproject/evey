package grpcapi

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/user"
)

// An authorized pairing hands out ONE session: two pollers racing on the same
// code (or a replay after the device collected) must not both get tokens.
func TestCheckQuickConnect_TokensAreIssuedOnce(t *testing.T) {
	env := setupUserHandler()
	seedAdultProfile(t, env, "prf-1", "usr-1")
	seedUser(t, env, &user.User{ID: "usr-1", Username: "tester"}, "pass")
	initResp, err := env.handler.InitiateQuickConnect(context.Background(), &eveyv1.InitiateQuickConnectRequest{})
	require.NoError(t, err)
	_, err = env.handler.AuthorizeQuickConnect(userCtx("usr-1", "prf-1"), &eveyv1.AuthorizeQuickConnectRequest{Code: initResp.Code})
	require.NoError(t, err)

	var wg sync.WaitGroup
	var mu sync.Mutex
	issued := 0
	start := make(chan struct{})
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			resp, cErr := env.handler.CheckQuickConnect(context.Background(), &eveyv1.CheckQuickConnectRequest{Code: initResp.Code})
			if cErr == nil && resp.GetAuth().GetAccessToken() != "" {
				mu.Lock()
				issued++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()
	assert.Equal(t, 1, issued)

	resp, err := env.handler.CheckQuickConnect(context.Background(), &eveyv1.CheckQuickConnectRequest{Code: initResp.Code})
	if err == nil {
		assert.Empty(t, resp.GetAuth().GetAccessToken(), "a replay gets no tokens")
	}
}
