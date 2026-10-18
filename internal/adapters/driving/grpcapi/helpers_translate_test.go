package grpcapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/eveyproject/evey/internal/domain/user"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
)

// The shared helpers had no translator, so "cannot access another profile"
// reached a French viewer in English. With the server's translator stored they
// answer in the caller's language; without one they keep the English text.
func TestHelperError_LocalisedOnlyWhenATranslatorIsSet(t *testing.T) {
	claims := &user.Claims{UserID: "u1", ProfileID: "p1"}

	_, err := resolveProfileID(context.Background(), claims, "p2")
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.Equal(t, "cannot access another profile", status.Convert(err).Message(), "no translator: English fallback")

	tr, trErr := eveyi18n.NewFromDir("../../../../i18n")
	require.NoError(t, trErr)
	helperTranslator.Store(tr)
	t.Cleanup(func() { helperTranslator.Store(nil) })

	_, err = resolveProfileID(context.Background(), claims, "p2")
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "the code does not change")
	assert.Equal(t, "You cannot access another profile.", status.Convert(err).Message(), "the message now comes from the catalogue")
}
