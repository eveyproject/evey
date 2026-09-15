package dlna

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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
	anon := serve(`<root><device><serviceList><service><serviceType>urn:AVTransport:1</serviceType><controlURL>http://10.0.0.9:8080/avt</controlURL></service></serviceList></device></root>`)
	defer anon.Close()
	r, err := fetchDeviceDescription(context.Background(), anon.URL)
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != anon.URL || r.ID != anon.URL {
		t.Errorf("Name/ID = %q/%q, want the location", r.Name, r.ID)
	}
	if r.ControlURL != "http://10.0.0.9:8080/avt" {
		t.Errorf("an absolute control URL is kept as is: %q", r.ControlURL)
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
