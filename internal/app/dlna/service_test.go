package dlna

import (
	"context"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	svc := New("http://localhost:9091")
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.baseURL != "http://localhost:9091" {
		t.Errorf("expected baseURL http://localhost:9091, got %s", svc.baseURL)
	}
}

func TestFindRenderer_NotFound(t *testing.T) {
	svc := New("http://localhost")
	_, err := svc.findRenderer("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing renderer")
	}
}

func TestFindRenderer_Found(t *testing.T) {
	svc := New("http://localhost")
	svc.renderers = []Renderer{
		{ID: "r1", Name: "TV", ControlURL: "http://tv/ctrl"},
	}
	r, err := svc.findRenderer("r1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Name != "TV" {
		t.Errorf("expected name TV, got %s", r.Name)
	}
}

func TestParseLocationHeader(t *testing.T) {
	resp := "HTTP/1.1 200 OK\r\nLOCATION: http://192.168.1.10:8080/desc.xml\r\nST: urn:schemas-upnp-org:service:AVTransport:1\r\n\r\n"
	loc := parseLocationHeader(resp)
	if loc != "http://192.168.1.10:8080/desc.xml" {
		t.Errorf("expected http://192.168.1.10:8080/desc.xml, got %s", loc)
	}
}

func TestParseLocationHeader_Missing(t *testing.T) {
	resp := "HTTP/1.1 200 OK\r\nST: urn:schemas-upnp-org:service:AVTransport:1\r\n\r\n"
	loc := parseLocationHeader(resp)
	if loc != "" {
		t.Errorf("expected empty, got %s", loc)
	}
}

func TestResolveURL(t *testing.T) {
	abs, err := resolveURL("http://192.168.1.10:8080/desc.xml", "/ctrl/AVTransport")
	if err != nil {
		t.Fatal(err)
	}
	if abs != "http://192.168.1.10:8080/ctrl/AVTransport" {
		t.Errorf("expected http://192.168.1.10:8080/ctrl/AVTransport, got %s", abs)
	}
}

func TestResolveURL_Absolute(t *testing.T) {
	abs, err := resolveURL("http://192.168.1.10:8080/desc.xml", "http://other:1234/ctrl")
	if err != nil {
		t.Fatal(err)
	}
	if abs != "http://other:1234/ctrl" {
		t.Errorf("expected http://other:1234/ctrl, got %s", abs)
	}
}

func TestBuildDIDLMetadata(t *testing.T) {
	m := buildDIDLMetadata("Test <Movie>", "http://example.com/video.mp4")
	if m == "" {
		t.Fatal("expected non-empty metadata")
	}
	if !contains(m, "Test &lt;Movie&gt;") {
		t.Error("expected XML-escaped title")
	}
	if !contains(m, "http://example.com/video.mp4") {
		t.Error("expected URL in metadata")
	}
}

func TestBuildSOAPBody_SetAVTransportURI_ArgOrder(t *testing.T) {
	for range 20 {
		body := buildSOAPBody("SetAVTransportURI", []soapArg{
			{"InstanceID", "0"},
			{"CurrentURI", "http://example.com/video.mp4"},
			{"CurrentURIMetaData", "<DIDL-Lite/>"},
		})

		idxInstanceID := indexOf(body, "<InstanceID>")
		idxCurrentURI := indexOf(body, "<CurrentURI>")
		idxCurrentURIMetaData := indexOf(body, "<CurrentURIMetaData>")

		if idxInstanceID < 0 || idxCurrentURI < 0 || idxCurrentURIMetaData < 0 {
			t.Fatalf("expected all three args present in body: %s", body)
		}
		if idxInstanceID >= idxCurrentURI || idxCurrentURI >= idxCurrentURIMetaData {
			t.Fatalf("expected InstanceID < CurrentURI < CurrentURIMetaData order, got body: %s", body)
		}
	}
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// A single renderer answers M-SEARCH several times, and often from more than
// one LOCATION (multi-homed box, IPv4 + a second interface, or a description
// URL that differs only by a trailing path). Keying the dedup on LOCATION let
// the SAME physical device show up twice in the picker under the same name -
// reported by the user on their network. The UPnP identity is the UDN.
func TestDedupeRenderers_SameUDNTwoLocations(t *testing.T) {
	in := []Renderer{
		{ID: "uuid:abc", UDN: "uuid:abc", Name: "Salon", Location: "http://192.168.1.20:8060/dd.xml", ControlURL: "http://192.168.1.20:8060/ctrl"},
		{ID: "uuid:abc", UDN: "uuid:abc", Name: "Salon", Location: "http://192.168.1.20:1900/description.xml", ControlURL: "http://192.168.1.20:1900/ctrl"},
	}
	out := dedupeRenderers(in)
	if len(out) != 1 {
		t.Fatalf("want 1 renderer, got %d: %+v", len(out), out)
	}
	if out[0].Location != in[0].Location {
		t.Errorf("first LOCATION seen must win, got %q", out[0].Location)
	}
}

func TestDedupeRenderers_DistinctDevicesKept(t *testing.T) {
	in := []Renderer{
		{ID: "uuid:abc", UDN: "uuid:abc", Name: "Salon", Location: "http://192.168.1.20:8060/dd.xml"},
		{ID: "uuid:def", UDN: "uuid:def", Name: "Bedroom", Location: "http://192.168.1.21:8060/dd.xml"},
	}
	if got := len(dedupeRenderers(in)); got != 2 {
		t.Fatalf("two different UDNs must both survive, got %d", got)
	}
}

// Some renderers omit the UDN. Falling back to the name would merge two real
// TVs that ship with the same default name, so the fallback keeps LOCATION.
func TestDedupeRenderers_NoUDNFallsBackToLocation(t *testing.T) {
	in := []Renderer{
		{ID: "http://a/dd.xml", Name: "DMR", Location: "http://a/dd.xml"},
		{ID: "http://b/dd.xml", Name: "DMR", Location: "http://b/dd.xml"},
		{ID: "http://a/dd.xml", Name: "DMR", Location: "http://a/dd.xml"},
	}
	if got := len(dedupeRenderers(in)); got != 2 {
		t.Fatalf("want 2 (a and b, the repeat of a dropped), got %d", got)
	}
}

// The header comes off a UDP socket, so its bytes are whatever a device on the
// LAN chose to send. The parser used to match an upper-cased copy of the line
// and then slice the ORIGINAL at a fixed byte offset, which only lines up while
// uppercasing preserves byte length. It does not: a Turkish dotless i is two
// bytes and uppercases to a one-byte I, so "LOCATıON:" matched the prefix and
// the value came back as ": http://..." - a URL with a leading colon that
// fetches nothing, and the renderer silently drops out of discovery.
func TestParseLocationHeader_UppercasingDoesNotShiftTheValue(t *testing.T) {
	cases := []struct {
		name string
		resp string
		want string
	}{
		{"lowercase key", "location: http://192.168.1.10:8080/desc.xml\r\n", "http://192.168.1.10:8080/desc.xml"},
		{"mixed case key", "Location: http://192.168.1.10:8080/desc.xml\r\n", "http://192.168.1.10:8080/desc.xml"},
		{"space before the colon", "Location : http://192.168.1.10:8080/desc.xml\r\n", "http://192.168.1.10:8080/desc.xml"},
		{"turkish dotless i", "LOCATıON: http://192.168.1.10:8080/desc.xml\r\n", ""},
		{"colon only", "LOCATION:\r\n", ""},
		{"no colon at all", "LOCATION http://192.168.1.10:8080/desc.xml\r\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseLocationHeader(tc.resp); got != tc.want {
				t.Errorf("parseLocationHeader(%q) = %q, want %q", tc.resp, got, tc.want)
			}
		})
	}
}

// A fresh scan is served from the cache for its TTL, and the caller gets a
// copy: the picker mutating its list must not rewrite what the next caller
// sees. An empty result is never cached, so a TV switched on after a
// miss shows up on the next click rather than a minute later.
func TestDiscover_CacheServesCopy(t *testing.T) {
	s := New("http://evey.local")
	s.renderers = []Renderer{{ID: "r1", Name: "Salon", Location: "http://tv/desc.xml"}}
	s.lastScan = time.Now()

	got, err := s.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 || got[0].ID != "r1" {
		t.Fatalf("got %+v, want the cached renderer", got)
	}
	got[0].Name = "changed"
	if s.renderers[0].Name != "Salon" {
		t.Error("cache was mutated through the returned slice")
	}

	s.renderers = nil
	if time.Since(s.lastScan) >= cacheTTL || len(s.renderers) > 0 {
		t.Fatal("precondition: fresh scan with no renderers")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _ = s.Discover(ctx)
	if time.Since(start) > time.Second {
		t.Error("an empty cache must rescan bounded by the caller's deadline, not the 3s SSDP window")
	}
}
