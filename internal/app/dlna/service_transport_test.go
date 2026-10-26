package dlna

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// The description fetch and the AVTransport control path, exercised against a
// loopback renderer that records what it was sent.

const deviceXML = `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <device>
    <friendlyName>Living room TV</friendlyName>
    <UDN> uuid:1234-abcd </UDN>
    <serviceList>
      <service><serviceType>urn:schemas-upnp-org:service:RenderingControl:1</serviceType><controlURL>/rc</controlURL></service>
      <service><serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType><controlURL>/upnp/control/avt</controlURL></service>
    </serviceList>
  </device>
</root>`

func TestFetchDeviceDescription_ResolvesControlURLAndIdentity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, deviceXML)
	}))
	defer srv.Close()

	r, err := fetchDeviceDescription(context.Background(), srv.URL+"/desc.xml")
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "Living room TV" {
		t.Errorf("Name = %q", r.Name)
	}
	if r.UDN != "uuid:1234-abcd" || r.ID != "uuid:1234-abcd" {
		t.Errorf("UDN/ID = %q/%q, want the trimmed UDN as the stable id", r.UDN, r.ID)
	}
	if r.ControlURL != srv.URL+"/upnp/control/avt" {
		t.Errorf("ControlURL = %q, want the AVTransport control path resolved against the description URL", r.ControlURL)
	}
	if r.Location != srv.URL+"/desc.xml" {
		t.Errorf("Location = %q", r.Location)
	}
}

func TestFetchDeviceDescription_FallbacksAndRefusals(t *testing.T) {
	serve := func(body string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, body)
		}))
	}
	// No name, no UDN: the location stands in for both.
	// An absolute control URL on the device itself (another port) is kept.
	anon := serve(`<root><device><serviceList><service><serviceType>urn:AVTransport:1</serviceType><controlURL>http://127.0.0.1:8080/avt</controlURL></service></serviceList></device></root>`)
	defer anon.Close()
	r, err := fetchDeviceDescription(context.Background(), anon.URL)
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != anon.URL || r.ID != anon.URL {
		t.Errorf("Name/ID = %q/%q, want the location", r.Name, r.ID)
	}
	if r.ControlURL != "http://127.0.0.1:8080/avt" {
		t.Errorf("an absolute control URL on the same host is kept as is: %q", r.ControlURL)
	}

	// One on another host would turn every Play into a POST anywhere.
	elsewhere := serve(`<root><device><serviceList><service><serviceType>urn:AVTransport:1</serviceType><controlURL>http://169.254.169.254/latest</controlURL></service></serviceList></device></root>`)
	defer elsewhere.Close()
	if _, err := fetchDeviceDescription(context.Background(), elsewhere.URL); err == nil {
		t.Error("a control URL on another host must be refused")
	}

	// A device without AVTransport cannot be played to.
	noAVT := serve(`<root><device><friendlyName>Printer</friendlyName><serviceList><service><serviceType>urn:Printer:1</serviceType><controlURL>/p</controlURL></service></serviceList></device></root>`)
	defer noAVT.Close()
	if _, err := fetchDeviceDescription(context.Background(), noAVT.URL); err == nil {
		t.Error("a device without AVTransport must be refused")
	}

	garbage := serve(`not xml at all`)
	defer garbage.Close()
	if _, err := fetchDeviceDescription(context.Background(), garbage.URL); err == nil {
		t.Error("unparseable description must be refused")
	}

	if _, err := fetchDeviceDescription(context.Background(), "http://127.0.0.1:1/unreachable"); err == nil {
		t.Error("an unreachable device must be an error")
	}
}

// recordingRenderer captures each SOAP action (header + body) it receives.
type recordingRenderer struct {
	mu      sync.Mutex
	actions []string
	bodies  []string
	status  int
}

func (r *recordingRenderer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	r.actions = append(r.actions, req.Header.Get("SOAPAction"))
	r.bodies = append(r.bodies, string(body))
	r.mu.Unlock()
	if r.status != 0 {
		w.WriteHeader(r.status)
		_, _ = io.WriteString(w, "<fault/>")
	}
}

func serviceWithRenderer(controlURL string) *Service {
	s := New("http://evey.test")
	s.renderers = []Renderer{{ID: "tv", Name: "TV", ControlURL: controlURL}}
	return s
}

func TestPlay_SendsSetURIThenPlay(t *testing.T) {
	rec := &recordingRenderer{}
	srv := httptest.NewServer(rec)
	defer srv.Close()
	s := serviceWithRenderer(srv.URL + "/avt")

	if err := s.Play(context.Background(), "tv", "http://evey.test/m.mp4?a=1&b=2", "Film <1>"); err != nil {
		t.Fatal(err)
	}
	if len(rec.actions) != 2 {
		t.Fatalf("got %d SOAP calls, want SetAVTransportURI then Play: %v", len(rec.actions), rec.actions)
	}
	if !strings.HasSuffix(rec.actions[0], `#SetAVTransportURI"`) || !strings.HasSuffix(rec.actions[1], `#Play"`) {
		t.Errorf("actions = %v", rec.actions)
	}
	// The URI and the title travel escaped, never raw, inside the envelope.
	if !strings.Contains(rec.bodies[0], "<CurrentURI>http://evey.test/m.mp4?a=1&amp;b=2</CurrentURI>") {
		t.Errorf("SetAVTransportURI body = %s", rec.bodies[0])
	}
	if strings.Contains(rec.bodies[0], "<1>") || !strings.Contains(rec.bodies[0], "Film &amp;lt;1&amp;gt;") {
		t.Errorf("title must be escaped inside the (escaped) DIDL metadata: %s", rec.bodies[0])
	}
	if !strings.Contains(rec.bodies[1], "<Speed>1</Speed>") {
		t.Errorf("Play body = %s", rec.bodies[1])
	}
}

func TestStopAndPause_SendTheirActions(t *testing.T) {
	rec := &recordingRenderer{}
	srv := httptest.NewServer(rec)
	defer srv.Close()
	s := serviceWithRenderer(srv.URL + "/avt")

	if err := s.Stop(context.Background(), "tv"); err != nil {
		t.Fatal(err)
	}
	if err := s.Pause(context.Background(), "tv"); err != nil {
		t.Fatal(err)
	}
	if len(rec.actions) != 2 || !strings.HasSuffix(rec.actions[0], `#Stop"`) || !strings.HasSuffix(rec.actions[1], `#Pause"`) {
		t.Errorf("actions = %v", rec.actions)
	}
}

func TestTransport_RendererErrorsSurface(t *testing.T) {
	rec := &recordingRenderer{status: http.StatusInternalServerError}
	srv := httptest.NewServer(rec)
	defer srv.Close()
	s := serviceWithRenderer(srv.URL + "/avt")

	if err := s.Play(context.Background(), "tv", "http://evey.test/m.mp4", "Film"); err == nil || !strings.Contains(err.Error(), "SetAVTransportURI") {
		t.Errorf("a SOAP fault must name the failing action: %v", err)
	}
	if len(rec.actions) != 1 {
		t.Errorf("Play must not be sent after SetAVTransportURI failed: %v", rec.actions)
	}
	if err := s.Stop(context.Background(), "tv"); err == nil {
		t.Error("Stop fault must surface")
	}
	if err := s.Pause(context.Background(), "ghost"); err == nil {
		t.Error("an unknown renderer must be refused before any network call")
	}
}

// The description is only fetched from the device that answered the M-SEARCH:
// any LAN host could otherwise make the server GET an arbitrary URL.
func TestLocationFromResponder(t *testing.T) {
	src := net.ParseIP("192.168.1.40")
	for loc, want := range map[string]bool{
		"http://192.168.1.40:49152/desc.xml": true,
		"https://192.168.1.40/desc.xml":      true,
		"http://192.168.1.41:49152/desc.xml": false,
		"http://127.0.0.1:5432/":             false,
		"http://tv.local:49152/desc.xml":     false,
		"file:///etc/passwd":                 false,
		"gopher://192.168.1.40/":             false,
		"":                                   false,
	} {
		if got := locationFromResponder(loc, src); got != want {
			t.Errorf("%q from %s: got %v, want %v", loc, src, got, want)
		}
	}
}

// A renderer is a LAN device: a responder on a public or loopback address is
// never one, and fetching from it would turn discovery into a request relay.
func TestLocationFromResponder_PrivateRangesOnly(t *testing.T) {
	for loc, want := range map[string]bool{
		"http://10.0.0.7:49152/desc.xml":    true,
		"http://172.16.4.2:49152/desc.xml":  true,
		"http://192.168.0.9:49152/desc.xml": true,
		"http://169.254.10.1/desc.xml":      true,
		"http://[fd00::7]:49152/desc.xml":   true,
		"http://203.0.113.5:49152/desc.xml": false,
		"http://8.8.8.8/desc.xml":           false,
		"http://127.0.0.1:7700/":            false,
		"http://[2001:db8::1]/desc.xml":     false,
	} {
		u, _ := url.Parse(loc)
		src := net.ParseIP(u.Hostname())
		if got := locationFromResponder(loc, src); got != want {
			t.Errorf("%q from itself: got %v, want %v", loc, got, want)
		}
	}
}

func TestPlay_RefusesANonHTTPMediaURL(t *testing.T) {
	rec := &recordingRenderer{}
	srv := httptest.NewServer(rec)
	defer srv.Close()
	s := serviceWithRenderer(srv.URL + "/avt")
	for _, bad := range []string{"file:///etc/passwd", "/relative/m.mp4", "", "ftp://h/m.mp4"} {
		if err := s.Play(context.Background(), "tv", bad, "t"); err == nil {
			t.Errorf("media url %q must be refused", bad)
		}
	}
	if len(rec.actions) != 0 {
		t.Fatalf("a refused url reached the renderer: %v", rec.actions)
	}
}
