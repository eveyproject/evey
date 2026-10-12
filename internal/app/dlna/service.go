package dlna

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/pkg/tracing"
)

const (
	ssdpAddr    = "239.255.255.250:1900"
	ssdpST      = "urn:schemas-upnp-org:service:AVTransport:1"
	cacheTTL    = 60 * time.Second
	ssdpTimeout = 3 * time.Second
)

type Renderer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// UDN is the UPnP Unique Device Name ("uuid:..."), the device's stable
	// identity. It is what distinguishes ONE renderer answering from several
	// LOCATIONs from two distinct renderers - see dedupeRenderers.
	UDN        string `json:"udn,omitempty"`
	Location   string `json:"location"`
	ControlURL string `json:"controlUrl"`
}

// dedupeRenderers collapses the several answers one renderer gives to a single
// M-SEARCH. SSDP devices repeat their reply (UDP is lossy by design) and a
// multi-homed box answers from more than one interface, each with its own
// LOCATION URL - which is why deduping on LOCATION alone still showed the same
// TV twice in the picker. Key on the UDN when the description carries one.
//
// The fallback stays LOCATION, deliberately NOT the name: two identical TVs
// ship with the same default friendly name, and merging them would hide a real
// device instead of a duplicate.
func dedupeRenderers(in []Renderer) []Renderer {
	seen := make(map[string]struct{}, len(in))
	out := make([]Renderer, 0, len(in))
	for _, r := range in {
		key := r.UDN
		if key == "" {
			key = r.Location
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, r)
	}
	return out
}

type Service struct {
	mu        sync.RWMutex
	renderers []Renderer
	lastScan  time.Time
	baseURL   string
}

func New(baseURL string) *Service {
	return &Service{baseURL: baseURL}
}

func (s *Service) Discover(ctx context.Context) ([]Renderer, error) {
	ctx, span := tracing.StartSpan(ctx, "app.dlna.Discover")
	defer span.End()

	s.mu.RLock()
	if time.Since(s.lastScan) < cacheTTL && len(s.renderers) > 0 {
		r := make([]Renderer, len(s.renderers))
		copy(r, s.renderers)
		s.mu.RUnlock()
		return r, nil
	}
	s.mu.RUnlock()

	renderers, err := s.discover(ctx)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.renderers = renderers
	s.lastScan = time.Now()
	s.mu.Unlock()

	return renderers, nil
}

func (s *Service) discover(ctx context.Context) ([]Renderer, error) {
	addr, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return nil, fmt.Errorf("resolve ssdp addr: %w", err)
	}
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, fmt.Errorf("listen udp: %w", err)
	}
	defer conn.Close()

	msg := "M-SEARCH * HTTP/1.1\r\n" +
		"HOST: " + ssdpAddr + "\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 3\r\n" +
		"ST: " + ssdpST + "\r\n" +
		"\r\n"

	if _, err := conn.WriteToUDP([]byte(msg), addr); err != nil {
		return nil, fmt.Errorf("send m-search: %w", err)
	}

	deadline := time.Now().Add(ssdpTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		slog.WarnContext(ctx, "dlna set read deadline failed", slog.Any("err", err))
	}

	seen := make(map[string]struct{})
	var renderers []Renderer
	buf := make([]byte, 4096)
	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			break
		}
		location := parseLocationHeader(string(buf[:n]))
		if location == "" {
			continue
		}
		if !locationFromResponder(location, src.IP) {
			slog.DebugContext(ctx, "dlna: skip a location the responder does not host",
				slog.String("location", location), slog.String("from", src.IP.String()))
			continue
		}
		if _, ok := seen[location]; ok {
			continue
		}
		seen[location] = struct{}{}

		r, err := fetchDeviceDescription(ctx, location)
		if err != nil {
			slog.DebugContext(ctx, "dlna: skip device", slog.String("location", location), slog.Any("err", err))
			continue
		}
		renderers = append(renderers, *r)
	}
	renderers = dedupeRenderers(renderers)
	slog.InfoContext(ctx, "dlna: discovery complete", slog.Int("count", len(renderers)))
	return renderers, nil
}

// Splits on the line's own colon rather than matching an upper-cased copy and
// then slicing the original at a fixed offset: ToUpper does not preserve byte
// length for every input, so the two can disagree about where the value starts.
// "LOCATıON: http://host/desc.xml" (Turkish dotless i, two bytes, uppercasing
// to a one-byte I) matched the prefix and then returned ": http://host/desc.xml"
// - a URL with a leading colon, which fetches nothing. This also accepts
// "Location :" and any other spacing, which the fixed offset did not.
func parseLocationHeader(resp string) string {
	for _, line := range strings.Split(resp, "\r\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(key), "LOCATION") {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// XML structures for device description parsing
type deviceRoot struct {
	XMLName xml.Name `xml:"root"`
	Device  device   `xml:"device"`
}

type device struct {
	FriendlyName string      `xml:"friendlyName"`
	UDN          string      `xml:"UDN"`
	ServiceList  serviceList `xml:"serviceList"`
}

type serviceList struct {
	Services []service `xml:"service"`
}

type service struct {
	ServiceType string `xml:"serviceType"`
	ControlURL  string `xml:"controlURL"`
}

func fetchDeviceDescription(ctx context.Context, location string) (*Renderer, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, err
	}

	var root deviceRoot
	if err := xml.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("parse device xml: %w", err)
	}

	var controlPath string
	for _, svc := range root.Device.ServiceList.Services {
		if strings.Contains(svc.ServiceType, "AVTransport") {
			controlPath = svc.ControlURL
			break
		}
	}
	if controlPath == "" {
		return nil, errors.New("no AVTransport service found")
	}

	controlURL, err := resolveURL(location, controlPath)
	if err != nil {
		return nil, fmt.Errorf("resolve control url: %w", err)
	}
	if !sameHost(location, controlURL) {
		return nil, fmt.Errorf("control url %q is not on the device %q", controlURL, location)
	}

	name := root.Device.FriendlyName
	if name == "" {
		name = location
	}

	udn := strings.TrimSpace(root.Device.UDN)
	// ID keeps the UDN when there is one: it survives a DHCP lease change or a
	// reboot that moves the description port, so a cast target picked earlier
	// still resolves to the same device.
	id := udn
	if id == "" {
		id = location
	}
	return &Renderer{
		ID:         id,
		Name:       name,
		UDN:        udn,
		Location:   location,
		ControlURL: controlURL,
	}, nil
}

// locationFromResponder reports whether an SSDP LOCATION points at the host
// that sent it, as an http(s) URL on its IP. Anything else (another host, a
// name, another scheme) would let any LAN machine make the server fetch it.
func locationFromResponder(location string, src net.IP) bool {
	u, err := url.Parse(location)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	return ip != nil && src != nil && ip.Equal(src)
}

// sameHost reports whether two URLs name the same host (any port).
func sameHost(a, b string) bool {
	ua, errA := url.Parse(a)
	ub, errB := url.Parse(b)
	return errA == nil && errB == nil && ua.Hostname() != "" && strings.EqualFold(ua.Hostname(), ub.Hostname()) &&
		(ub.Scheme == "http" || ub.Scheme == "https")
}

func resolveURL(base, ref string) (string, error) {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref, nil
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	r, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	return u.ResolveReference(r).String(), nil
}

func (s *Service) findRenderer(rendererID string) (*Renderer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.renderers {
		if r.ID == rendererID {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("renderer %q not found", rendererID)
}

func (s *Service) Play(ctx context.Context, rendererID, mediaURL, title string) error {
	ctx, span := tracing.StartSpan(ctx, "app.dlna.Play")
	defer span.End()

	r, err := s.findRenderer(rendererID)
	if err != nil {
		return err
	}
	metadata := buildDIDLMetadata(title, mediaURL)
	if err := sendSOAPAction(ctx, r.ControlURL, "SetAVTransportURI", []soapArg{
		{"InstanceID", "0"},
		{"CurrentURI", mediaURL},
		{"CurrentURIMetaData", metadata},
	}); err != nil {
		return fmt.Errorf("SetAVTransportURI: %w", err)
	}
	if err := sendSOAPAction(ctx, r.ControlURL, "Play", []soapArg{
		{"InstanceID", "0"},
		{"Speed", "1"},
	}); err != nil {
		return fmt.Errorf("Play: %w", err)
	}
	slog.InfoContext(ctx, "dlna: playing", slog.String("renderer", r.Name), slog.String("url", mediaURL))
	return nil
}

func (s *Service) Stop(ctx context.Context, rendererID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.dlna.Stop")
	defer span.End()

	r, err := s.findRenderer(rendererID)
	if err != nil {
		return err
	}
	if err := sendSOAPAction(ctx, r.ControlURL, "Stop", []soapArg{
		{"InstanceID", "0"},
	}); err != nil {
		return fmt.Errorf("Stop: %w", err)
	}
	return nil
}

func (s *Service) Pause(ctx context.Context, rendererID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.dlna.Pause")
	defer span.End()

	r, err := s.findRenderer(rendererID)
	if err != nil {
		return err
	}
	if err := sendSOAPAction(ctx, r.ControlURL, "Pause", []soapArg{
		{"InstanceID", "0"},
	}); err != nil {
		return fmt.Errorf("Pause: %w", err)
	}
	return nil
}

func buildDIDLMetadata(title, mediaURL string) string {
	return `<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">` +
		`<item id="0" parentID="0" restricted="1">` +
		`<dc:title>` + html.EscapeString(title) + `</dc:title>` +
		`<upnp:class>object.item.videoItem</upnp:class>` +
		`<res protocolInfo="http-get:*:video/mp4:*">` + html.EscapeString(mediaURL) + `</res>` +
		`</item>` +
		`</DIDL-Lite>`
}

// soapArg is a single ordered SOAP argument. UPnP AVTransport control
// points expect action arguments in the exact order defined by the
// service WSDL (not alphabetical), so callers must pass args in a slice
// rather than a map to preserve insertion order.
type soapArg struct {
	Name  string
	Value string
}

// buildSOAPBody renders the SOAP envelope for the given AVTransport
// action, writing args in the order supplied by the caller.
func buildSOAPBody(action string, args []soapArg) string {
	var argXML strings.Builder
	for _, a := range args {
		argXML.WriteString("<" + a.Name + ">" + html.EscapeString(a.Value) + "</" + a.Name + ">")
	}

	return `<?xml version="1.0" encoding="utf-8"?>` +
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">` +
		`<s:Body>` +
		`<u:` + action + ` xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">` +
		argXML.String() +
		`</u:` + action + `>` +
		`</s:Body>` +
		`</s:Envelope>`
}

func sendSOAPAction(ctx context.Context, controlURL, action string, args []soapArg) error {
	body := buildSOAPBody(action, args)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, controlURL, bytes.NewReader([]byte(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", `"urn:schemas-upnp-org:service:AVTransport:1#`+action+`"`)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("SOAP %s returned %d: %s", action, resp.StatusCode, string(b))
	}
	return nil
}
