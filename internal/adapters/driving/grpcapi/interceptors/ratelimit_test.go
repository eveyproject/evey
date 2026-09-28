package interceptors_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

const testMethod = "/evey.v1.UserService/Login"

func ipCtx(ip string) context.Context {
	md := metadata.Pairs("x-forwarded-for", ip)
	ctx := metadata.NewIncomingContext(context.Background(), md)
	// run through ClientInfoUnary so ClientIPFromCtx works
	ci := interceptors.ClientInfoUnary()
	var out context.Context
	h := func(c context.Context, _ any) (any, error) { out = c; return nil, nil }
	_, _ = ci(ctx, nil, &grpc.UnaryServerInfo{FullMethod: testMethod}, h)
	return out
}

func TestRateLimitUnary_AllowsUnderLimit(t *testing.T) {
	limiters := map[string]*interceptors.MethodLimiter{
		testMethod: {Limiter: ratelimit.NewWithBurst(3, 3), RetryAfterSecs: 10},
	}
	interceptor := interceptors.RateLimitUnary(limiters)
	handler := func(ctx context.Context, req any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{FullMethod: testMethod}

	ctx := ipCtx("1.2.3.4")
	for i := 0; i < 3; i++ {
		resp, err := interceptor(ctx, nil, info, handler)
		require.NoError(t, err, "call %d", i)
		assert.Equal(t, "ok", resp)
	}
}

func TestRateLimitUnary_BlocksAfterBurst(t *testing.T) {
	limiters := map[string]*interceptors.MethodLimiter{
		testMethod: {Limiter: ratelimit.NewWithBurst(2, 0.01), RetryAfterSecs: 60},
	}
	interceptor := interceptors.RateLimitUnary(limiters)
	handler := func(ctx context.Context, req any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{FullMethod: testMethod}

	ctx := ipCtx("5.5.5.5")
	_, err := interceptor(ctx, nil, info, handler)
	require.NoError(t, err)
	_, err = interceptor(ctx, nil, info, handler)
	require.NoError(t, err)

	_, err = interceptor(ctx, nil, info, handler)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}

func TestRateLimitUnary_PerIPIsolation(t *testing.T) {
	limiters := map[string]*interceptors.MethodLimiter{
		testMethod: {Limiter: ratelimit.NewWithBurst(1, 0.01), RetryAfterSecs: 60},
	}
	interceptor := interceptors.RateLimitUnary(limiters)
	handler := func(ctx context.Context, req any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{FullMethod: testMethod}

	_, err := interceptor(ipCtx("1.1.1.1"), nil, info, handler)
	require.NoError(t, err)
	_, err = interceptor(ipCtx("1.1.1.1"), nil, info, handler)
	require.Error(t, err)
	// different IP still allowed
	_, err = interceptor(ipCtx("2.2.2.2"), nil, info, handler)
	require.NoError(t, err)
}

func TestRateLimitUnary_TokenRefill(t *testing.T) {
	// 60 req/min = 1 token/s
	limiters := map[string]*interceptors.MethodLimiter{
		testMethod: {Limiter: ratelimit.NewWithBurst(1, 60), RetryAfterSecs: 1},
	}
	interceptor := interceptors.RateLimitUnary(limiters)
	handler := func(ctx context.Context, req any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{FullMethod: testMethod}

	ctx := ipCtx("7.7.7.7")
	_, err := interceptor(ctx, nil, info, handler)
	require.NoError(t, err)
	_, err = interceptor(ctx, nil, info, handler)
	require.Error(t, err)

	time.Sleep(1100 * time.Millisecond)
	_, err = interceptor(ctx, nil, info, handler)
	require.NoError(t, err)
}

func TestRateLimitUnary_UnregisteredMethodPasses(t *testing.T) {
	limiters := map[string]*interceptors.MethodLimiter{
		testMethod: {Limiter: ratelimit.NewWithBurst(1, 0.01), RetryAfterSecs: 60},
	}
	interceptor := interceptors.RateLimitUnary(limiters)
	handler := func(ctx context.Context, req any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.UserService/Me"}

	ctx := ipCtx("9.9.9.9")
	for i := 0; i < 5; i++ {
		_, err := interceptor(ctx, nil, info, handler)
		require.NoError(t, err)
	}
}

func TestRateLimitStream_Blocks(t *testing.T) {
	limiters := map[string]*interceptors.MethodLimiter{
		testMethod: {Limiter: ratelimit.NewWithBurst(1, 0.01), RetryAfterSecs: 60},
	}
	interceptor := interceptors.RateLimitStream(limiters)
	handler := func(srv any, ss grpc.ServerStream) error { return nil }
	info := &grpc.StreamServerInfo{FullMethod: testMethod}

	ctx := ipCtx("8.8.8.8")
	ss := &fakeServerStream{ctx: ctx}
	require.NoError(t, interceptor(nil, ss, info, handler))
	err := interceptor(nil, ss, info, handler)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}

func TestDefaultPublicMethodLimiters_CoversAudit(t *testing.T) {
	m := interceptors.DefaultPublicMethodLimiters()
	required := []string{
		"/evey.v1.UserService/Login",
		"/evey.v1.UserService/RefreshToken",
		"/evey.v1.UserService/RequestPasswordReset",
		"/evey.v1.UserService/ResetPassword",
		"/evey.v1.UserService/RegisterWithInvite",
		"/evey.v1.UserService/InitiateQuickConnect",
		"/evey.v1.UserService/CheckQuickConnect",
		// AuthorizeQuickConnect must be capped too - otherwise an
		// authenticated attacker can brute-force the 24-bit short code
		// and hijack a victim's pending device.
		"/evey.v1.UserService/AuthorizeQuickConnect",
	}
	for _, k := range required {
		_, ok := m[k]
		assert.True(t, ok, "missing limiter for %s", k)
	}
}

// The refusal line is the ONLY evidence an operator gets about a throttled
// login. On 2026-07-29 production logged every external client under
// ip=127.0.0.1 and the line could not say why: "the proxy is untrusted so its
// forwarded address was dropped" and "no forwarded address ever arrived" need
// different fixes and looked identical. These fields separate them.
func TestRateLimitUnary_RefusalLogCarriesResolutionReasoning(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	// A trusted peer forwarding a real client: the honored path.
	ratelimit.SetTrustedProxies([]string{"10.0.0.0/8"})
	t.Cleanup(func() { ratelimit.SetTrustedProxies(defaultTrustedProxies) })

	limiters := map[string]*interceptors.MethodLimiter{
		testMethod: {Limiter: ratelimit.NewWithBurst(1, 0.06), RetryAfterSecs: 10},
	}
	interceptor := interceptors.RateLimitUnary(limiters)
	handler := func(_ context.Context, _ any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{FullMethod: testMethod}

	md := metadata.Pairs("x-forwarded-for", "198.51.100.7")
	base := peer.NewContext(metadata.NewIncomingContext(context.Background(), md),
		&peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("10.0.0.9"), Port: 5000}})
	var ctx context.Context
	_, _ = interceptors.ClientInfoUnary()(base, nil, info,
		func(c context.Context, _ any) (any, error) { ctx = c; return nil, nil })
	require.NotNil(t, ctx)

	_, err := interceptor(ctx, nil, info, handler) // consumes the single token
	require.NoError(t, err)
	_, err = interceptor(ctx, nil, info, handler) // refused, and logged
	require.Error(t, err)

	var line map[string]any
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	for {
		var l map[string]any
		if err := dec.Decode(&l); err != nil {
			break
		}
		if l["msg"] == "ratelimit: request rejected" {
			line = l
		}
	}
	require.NotNil(t, line, "no refusal line logged (raw=%q)", buf.String())

	for field, want := range map[string]any{
		"ip":           "198.51.100.7",
		"peer_ip":      "10.0.0.9",
		"peer_trusted": true,
		"xff":          "198.51.100.7",
		"xff_honored":  true,
	} {
		assert.Equal(t, want, line[field], "field %q", field)
	}
}

// A caller whose IP cannot be established must still be rate-limited. The
// interceptor maps an empty IP to the shared "unknown" bucket; mutation
// (2026-08-01) showed nothing pinned that, so the branch could be turned into a
// skip and every test stayed green - handing a bypass to anyone able to strip
// or spoof away their client IP on the login endpoint.
func TestRateLimitUnary_UnidentifiableCallerIsStillLimited(t *testing.T) {
	limiters := map[string]*interceptors.MethodLimiter{
		testMethod: {Limiter: ratelimit.NewWithBurst(2, 0.01), RetryAfterSecs: 60},
	}
	interceptor := interceptors.RateLimitUnary(limiters)
	handler := func(ctx context.Context, req any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{FullMethod: testMethod}

	// No client IP in the context at all.
	ctx := context.Background()

	for i := range 2 {
		_, err := interceptor(ctx, nil, info, handler)
		require.NoError(t, err, "call %d should be within the burst", i)
	}

	_, err := interceptor(ctx, nil, info, handler)
	require.Error(t, err, "an unidentifiable caller must hit the limit like any other; "+
		"skipping the limiter when the IP is unknown is a bypass for anyone who can "+
		"strip their client IP")
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
}

// The stream interceptor buckets by client IP exactly like the unary one, but
// only the unary path had an isolation test - so mutation (2026-08-01) showed
// the stream side's IP resolution could be inverted with the suite still green.
// Inverted, every caller lands in one shared bucket and the first viewer to
// exhaust it locks out everyone else: a one-client denial of service on the
// long-lived RPCs.
func TestRateLimitStream_PerIPIsolationAndUnknownCaller(t *testing.T) {
	limiters := map[string]*interceptors.MethodLimiter{
		testMethod: {Limiter: ratelimit.NewWithBurst(1, 0.01), RetryAfterSecs: 60},
	}
	interceptor := interceptors.RateLimitStream(limiters)
	handler := func(srv any, ss grpc.ServerStream) error { return nil }
	info := &grpc.StreamServerInfo{FullMethod: testMethod}

	require.NoError(t, interceptor(nil, &fakeServerStream{ctx: ipCtx("3.3.3.3")}, info, handler))
	require.Error(t, interceptor(nil, &fakeServerStream{ctx: ipCtx("3.3.3.3")}, info, handler),
		"the same caller must be limited on the stream path too")

	// A different caller keeps its own budget: one exhausted client must not
	// take the endpoint down for everyone.
	require.NoError(t, interceptor(nil, &fakeServerStream{ctx: ipCtx("4.4.4.4")}, info, handler),
		"a second caller was refused because of the first: all callers share one bucket")

	// And a caller with no resolvable IP is still limited, not skipped.
	noIP := &fakeServerStream{ctx: context.Background()}
	require.NoError(t, interceptor(nil, noIP, info, handler))
	err := interceptor(nil, noIP, info, handler)
	require.Error(t, err, "an unidentifiable stream caller must hit the limit like any other")
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
}
