package grpcapi

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"
)

// loopbackHTTPS trusts the certificate every httptest.NewTLSServer serves:
// the API takes plugin sources as https links only, and the SSRF-safe default
// client would refuse the loopback test servers anyway.
func loopbackHTTPS() *http.Client {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	cert := srv.Certificate()
	srv.Close()
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}
}

// servedZip serves content as a plugin archive over loopback https and
// returns its link.
func servedZip(t *testing.T, content string) string {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(content))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/p.zip"
}
