package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Only the host-enforced permissions leave through the public list; token
// scopes and backend permissions stay private.
func TestGrantedHostPermissions_OnlyHostOnes(t *testing.T) {
	assert.Equal(t, []string{"media.trash"}, GrantedHostPermissions([]string{"public", "media.files", "media.trash", "network"}))
	assert.Equal(t, []string{}, GrantedHostPermissions(nil))
}
