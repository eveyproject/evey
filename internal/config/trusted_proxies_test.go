package config

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evey-project/evey/internal/pkg/network"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The production value: the reverse proxy is a container on the Docker bridge,
// so the operator listed that network and nothing else.
const prodTrustedProxies = "172.16.0.0/12"

func TestTrustedProxies_LoopbackStaysTrustedWhenNotListed(t *testing.T) {
	t.Setenv("EVEY_TRUSTED_PROXIES", prodTrustedProxies)

	cfg, err := Load()
	require.NoError(t, err)

	assert.Contains(t, cfg.Server.TrustedProxies, "127.0.0.1/32")
	assert.Contains(t, cfg.Server.TrustedProxies, "::1/128")
	assert.Contains(t, cfg.Server.TrustedProxies, "172.16.0.0/12")
}

func TestTrustedProxies_OperatorEntriesAdded(t *testing.T) {
	t.Setenv("EVEY_TRUSTED_PROXIES", " 10.0.0.0/8 ,,192.168.0.0/16 , ")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, []string{"127.0.0.1/32", "::1/128", "10.0.0.0/8", "192.168.0.0/16"}, cfg.Server.TrustedProxies)
}

func TestTrustedProxies_DuplicatesCollapse(t *testing.T) {
	t.Setenv("EVEY_TRUSTED_PROXIES", "127.0.0.1/32,10.0.0.0/8,127.0.0.1,10.0.0.0/8,::1/128")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, []string{"127.0.0.1/32", "::1/128", "10.0.0.0/8"}, cfg.Server.TrustedProxies)
}

// A bare address is the notation operators actually type for a single proxy.
// It used to abort the boot from deep inside the HTTP wiring (NewProxyTrust),
// with no mention of the variable that caused it.
func TestTrustedProxies_BareIPBecomesSingleHostPrefix(t *testing.T) {
	t.Setenv("EVEY_TRUSTED_PROXIES", "172.18.0.2,fd00::1")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, []string{"127.0.0.1/32", "::1/128", "172.18.0.2/32", "fd00::1/128"}, cfg.Server.TrustedProxies)
}

func TestTrustedProxies_MalformedEntryRefusesBoot(t *testing.T) {
	t.Setenv("EVEY_TRUSTED_PROXIES", "10.0.0.0/8,not-a-cidr")

	_, err := Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "EVEY_TRUSTED_PROXIES")
	assert.Contains(t, err.Error(), "not-a-cidr")
}

// The security consequence, not the parsing. With the operator's list REPLACING
// the loopback default, the reverse proxy talking to Evey over loopback was not
// a trusted proxy: its X-Forwarded-For was dropped, the client IP fell back to
// 127.0.0.1, and detector.DetectIP classifies loopback as LAN. Every visitor
// from the internet was then treated as if sitting on the home network, which
// is exactly what the remote-access gate exists to prevent.
func TestTrustedProxies_InternetVisitorBehindLoopbackProxyStaysWAN(t *testing.T) {
	t.Setenv("EVEY_TRUSTED_PROXIES", prodTrustedProxies)

	cfg, err := Load()
	require.NoError(t, err)

	proxyTrust, err := network.NewProxyTrust(cfg.Server.TrustedProxies, cfg.Server.RealIPHeader, cfg.Server.ForwardedProto)
	require.NoError(t, err)
	detector := network.NewDetector(cfg.RemoteAccess.LANNetworks)

	var verdict network.NetworkType
	var seenIP string
	handler := proxyTrust.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		verdict = detector.Detect(r)
		seenIP = network.ClientIP(r).String()
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	assert.Equal(t, "203.0.113.7", seenIP)
	assert.Equal(t, network.WAN, verdict, "an internet visitor forwarded over loopback must stay WAN")
}

// The gRPC side reads the same list through ratelimit.IsTrustedProxy, and the
// server's own HTTP->gRPC gateway dials the gRPC listener over loopback: if
// 127.0.0.1 is not trusted there, EVERY gateway-forwarded request loses its
// client IP, whatever the HTTP layer resolved.
func TestTrustedProxies_GatewayLoopbackPeerIsTrusted(t *testing.T) {
	t.Setenv("EVEY_TRUSTED_PROXIES", prodTrustedProxies)

	cfg, err := Load()
	require.NoError(t, err)
	ratelimit.SetTrustedProxies(cfg.Server.TrustedProxies)
	t.Cleanup(func() { ratelimit.SetTrustedProxies(nil) })

	assert.True(t, ratelimit.IsTrustedProxy(net.ParseIP("127.0.0.1")))
	assert.True(t, ratelimit.IsTrustedProxy(net.ParseIP("::1")))
}

// The other consequence: one shared rate-limit bucket for the whole household.
func TestTrustedProxies_RateLimitKeysPerVisitorBehindLoopbackProxy(t *testing.T) {
	t.Setenv("EVEY_TRUSTED_PROXIES", prodTrustedProxies)

	cfg, err := Load()
	require.NoError(t, err)
	ratelimit.SetTrustedProxies(cfg.Server.TrustedProxies)
	t.Cleanup(func() { ratelimit.SetTrustedProxies(nil) })

	keyFor := func(forwarded string) string {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "127.0.0.1:54321"
		r.Header.Set("X-Forwarded-For", forwarded)
		return ratelimit.ClientIP(r)
	}

	assert.Equal(t, "203.0.113.7", keyFor("203.0.113.7"))
	assert.NotEqual(t, keyFor("203.0.113.7"), keyFor("198.51.100.9"), "two visitors must not share one bucket")
}
