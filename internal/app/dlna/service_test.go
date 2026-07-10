package dlna

import (
	"testing"
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
