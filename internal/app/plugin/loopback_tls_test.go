package plugin_test

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
)

// loopbackHTTPS is a client that trusts the certificate every
// httptest.NewTLSServer serves: plugins are downloaded over https only, and
// the SSRF-safe default would refuse the loopback test servers anyway.
func loopbackHTTPS() *http.Client {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	cert := srv.Certificate()
	srv.Close()
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}
}
