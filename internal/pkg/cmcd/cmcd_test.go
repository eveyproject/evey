package cmcd

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromRequest_QueryParam_HlsJsTypical(t *testing.T) {
	// Real-ish hls.js payload, URL-encoded once (what arrives on the wire).
	raw := url.QueryEscape(`bl=12000,br=2400,d=3000,mtp=8000,ot=v,sf=h,sid="a-uuid",st=v,tb=6000,v=1`)
	r := httpReq("/hls/abc/stream_0_042.m4s?CMCD=" + raw)

	p, ok := FromRequest(r)
	require.True(t, ok)
	assert.Equal(t, 12000, p.BufferLengthMs)
	assert.Equal(t, 2400, p.EncodedKbps)
	assert.Equal(t, 3000, p.ObjectDuration)
	assert.Equal(t, 8000, p.MeasuredKbps)
	assert.Equal(t, "v", p.ObjectType)
	assert.Equal(t, "h", p.StreamingFormat)
	assert.Equal(t, "a-uuid", p.SessionID)
	assert.Equal(t, "v", p.StreamType)
	assert.Equal(t, 6000, p.TopBitrateKbps)
	assert.Equal(t, 1, p.Version)
	assert.False(t, p.BufferStarved)
	assert.False(t, p.Startup)
}

func TestFromRequest_BooleanKeys(t *testing.T) {
	// "bs" and "su" are presence-only — bare key means true.
	raw := url.QueryEscape(`bs,br=2400,su`)
	r := httpReq("/hls/abc/stream_0_000.m4s?CMCD=" + raw)

	p, ok := FromRequest(r)
	require.True(t, ok)
	assert.True(t, p.BufferStarved, "bs presence must set BufferStarved=true")
	assert.True(t, p.Startup, "su presence must set Startup=true")
	assert.Equal(t, 2400, p.EncodedKbps)
}

func TestFromRequest_QuotedStringsAndCommas(t *testing.T) {
	// CTA-5004 allows commas inside quoted values — splitter must respect them.
	raw := url.QueryEscape(`sid="abc,def",cid="movie 42, take 2",br=2400`)
	r := httpReq("/hls/x/y?CMCD=" + raw)

	p, ok := FromRequest(r)
	require.True(t, ok)
	assert.Equal(t, "abc,def", p.SessionID)
	assert.Equal(t, "movie 42, take 2", p.ContentID)
	assert.Equal(t, 2400, p.EncodedKbps)
}

func TestFromRequest_PlaybackRateFloat(t *testing.T) {
	raw := url.QueryEscape(`pr=1.5,br=2400`)
	r := httpReq("/hls/x/y?CMCD=" + raw)
	p, _ := FromRequest(r)
	assert.InDelta(t, 1.5, p.PlaybackRate, 0.001)
}

func TestFromRequest_FromHeaders_Merged(t *testing.T) {
	// CMCD-* headers split keys by category; merged shape must round-trip.
	r := httpReq("/hls/x/y")
	r.Header.Set("CMCD-Request", `bl=12000,mtp=8000`)
	r.Header.Set("CMCD-Object", `br=2400,ot=v,d=3000`)
	r.Header.Set("CMCD-Session", `sid="abc",sf=h,st=v`)

	p, ok := FromRequest(r)
	require.True(t, ok)
	assert.Equal(t, 12000, p.BufferLengthMs)
	assert.Equal(t, 8000, p.MeasuredKbps)
	assert.Equal(t, 2400, p.EncodedKbps)
	assert.Equal(t, "v", p.ObjectType)
	assert.Equal(t, 3000, p.ObjectDuration)
	assert.Equal(t, "abc", p.SessionID)
	assert.Equal(t, "h", p.StreamingFormat)
	assert.Equal(t, "v", p.StreamType)
}

func TestFromRequest_Absent_NoFalsePositive(t *testing.T) {
	r := httpReq("/hls/x/y")
	_, ok := FromRequest(r)
	assert.False(t, ok, "no CMCD query and no CMCD-* header → not present")
}

func TestFromRequest_MalformedValue_DoesNotPanic(t *testing.T) {
	// Player sends garbage in a numeric field — must not break, just stay zero.
	raw := url.QueryEscape(`bl=not-a-number,br=2400,mtp=garbage`)
	r := httpReq("/hls/x/y?CMCD=" + raw)
	p, ok := FromRequest(r)
	require.True(t, ok)
	assert.Equal(t, 0, p.BufferLengthMs, "malformed numeric stays at zero")
	assert.Equal(t, 0, p.MeasuredKbps)
	assert.Equal(t, 2400, p.EncodedKbps, "valid sibling field still parsed")
}

func TestFromRequest_UnknownKeys_LandInRaw(t *testing.T) {
	// CTA-5004 allows custom keys — we keep them in Raw without breaking.
	raw := url.QueryEscape(`br=2400,custom-key="foo",x-evey-flag=1`)
	r := httpReq("/hls/x/y?CMCD=" + raw)
	p, _ := FromRequest(r)
	assert.Equal(t, 2400, p.EncodedKbps)
	assert.Equal(t, "foo", p.Raw["custom-key"])
	assert.Equal(t, "1", p.Raw["x-evey-flag"])
}

func httpReq(target string) *http.Request {
	r, err := http.NewRequest(http.MethodGet, "http://e"+target, nil)
	if err != nil {
		panic(err)
	}
	return r
}
