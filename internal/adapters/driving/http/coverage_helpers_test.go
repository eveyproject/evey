package httpapi

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/auth"
)

// --- shared helpers ---

func genTokens(t *testing.T) (*auth.JWTGenerator, string, string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	admin, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true, ProfileID: "prof-admin"})
	user, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", IsAdmin: false, ProfileID: "prof-1"})
	return gen, admin, user
}
