package plugin

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The download link is the admin's input. Through a proxy the SSRF-safe
// dialer would only ever see the proxy, never the real target, so the
// default download client takes no proxy from the environment.
func TestInstallerDefaultClient_TakesNoProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://proxy.example:3128")
	s := NewInstallerService(nil, nil, nil, nil)
	tr, ok := s.httpClient.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Nil(t, tr.Proxy)
}
