package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func fixedSecret() (string, error) { return strings.Repeat("ab", 32), nil }

func fileByName(t *testing.T, files []initFile, name string) initFile {
	t.Helper()
	for _, f := range files {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("no %s among generated files", name)
	return initFile{}
}

// The default install is ONE container and ZERO secrets: the whole point of
// the embedded database. A postgres service or a password sneaking back into
// the default output would silently restore the install tax this removes.
func TestRenderInit_DefaultIsSingleContainerWithoutSecrets(t *testing.T) {
	files, err := renderInit(initOptions{Media: "/srv/media", Port: 9091, Version: "latest"}, fixedSecret)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected compose + env, got %d files", len(files))
	}
	compose := fileByName(t, files, "docker-compose.yml").Content
	env := fileByName(t, files, ".env")

	for _, forbidden := range []string{"postgres:", "caddy:", "EVEY_DB_HOST", "PASSWORD", "MASTER_KEY"} {
		if strings.Contains(compose, forbidden) || strings.Contains(env.Content, forbidden) {
			t.Errorf("default output must not contain %q", forbidden)
		}
	}
	for _, want := range []string{
		"image: gravityz/evey:${EVEY_VERSION:-latest}",
		"\"${EVEY_HTTP_PORT:-9091}:9091\"",
		"${MEDIA_DIR}:/media",
		"./evey-data:/data",
		"stop_grace_period: 60s",
	} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose lacks %q", want)
		}
	}
	if !strings.Contains(env.Content, "MEDIA_DIR=/srv/media\n") {
		t.Errorf(".env lacks the media path:\n%s", env.Content)
	}
	if env.Mode != 0o644 {
		t.Errorf(".env without a secret should be 0644, got %o", env.Mode)
	}
}

func TestRenderInit_ExternalPostgresGetsServiceAndSecret(t *testing.T) {
	files, err := renderInit(initOptions{Media: "/srv/media", Port: 9091, Version: "latest", ExternalPostgres: true}, fixedSecret)
	if err != nil {
		t.Fatal(err)
	}
	compose := fileByName(t, files, "docker-compose.yml").Content
	env := fileByName(t, files, ".env")
	for _, want := range []string{"  postgres:\n", "EVEY_DB_HOST: postgres", "condition: service_healthy", "./postgres-data:/var/lib/postgresql/data"} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose lacks %q", want)
		}
	}
	if !strings.Contains(env.Content, "EVEY_DB_PASSWORD="+strings.Repeat("ab", 32)+"\n") {
		t.Errorf(".env lacks the generated password:\n%s", env.Content)
	}
	if env.Mode != 0o600 {
		t.Errorf(".env carrying a secret must be 0600, got %o", env.Mode)
	}
}

func TestRenderInit_DomainAddsCaddyAndProxyFacts(t *testing.T) {
	files, err := renderInit(initOptions{Media: "/srv/media", Port: 9091, Version: "latest", Domain: "media.example.com"}, fixedSecret)
	if err != nil {
		t.Fatal(err)
	}
	compose := fileByName(t, files, "docker-compose.yml").Content
	env := fileByName(t, files, ".env").Content
	caddy := fileByName(t, files, "Caddyfile").Content
	for _, want := range []string{"  caddy:\n", "./Caddyfile:/etc/caddy/Caddyfile:ro", "\"443:443\""} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose lacks %q", want)
		}
	}
	if strings.Contains(compose, ":9091\"") {
		t.Error("with a domain the server must not publish a host port - the proxy is the way in")
	}
	for _, want := range []string{
		"EVEY_DOMAIN=media.example.com\n",
		"EVEY_SERVER_BASE_URL=https://media.example.com\n",
		"EVEY_TRUSTED_PROXIES=172.16.0.0/12\n",
		"EVEY_FORWARDED_PROTO=true\n",
	} {
		if !strings.Contains(env, want) {
			t.Errorf(".env lacks %q", want)
		}
	}
	if !strings.Contains(caddy, "{$EVEY_DOMAIN:localhost}") {
		t.Error("Caddyfile must take its site address from EVEY_DOMAIN")
	}
}

// The Caddyfile init ships is a copy of the repository's: the CSP in it is
// load-bearing (wasm AC-3, Cast SDK), so the two must never drift.
func TestInitCaddyfileMatchesBundled(t *testing.T) {
	bundled, err := os.ReadFile(filepath.Join("..", "..", "docker", "Caddyfile"))
	if err != nil {
		t.Fatalf("read docker/Caddyfile: %v", err)
	}
	if string(bundled) != initCaddyfile {
		t.Fatal("cmd/evey/init_caddyfile.go drifted from docker/Caddyfile - regenerate the constant from the file")
	}
}

func TestRenderInit_RejectsBadInput(t *testing.T) {
	cases := map[string]initOptions{
		"missing media":      {Port: 9091, Version: "latest"},
		"relative media":     {Media: "media", Port: 9091, Version: "latest"},
		"bad port":           {Media: "/m", Port: 70000, Version: "latest"},
		"domain with scheme": {Media: "/m", Port: 9091, Version: "latest", Domain: "https://x.example"},
		"empty version":      {Media: "/m", Port: 9091},
	}
	for name, o := range cases {
		if _, err := renderInit(o, fixedSecret); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestWriteInitFiles_RefusesToClobberWithoutForce(t *testing.T) {
	dir := t.TempDir()
	files := []initFile{{Name: ".env", Content: "A=1\n", Mode: 0o600}}
	if _, err := writeInitFiles(dir, files, false); err != nil {
		t.Fatal(err)
	}
	if _, err := writeInitFiles(dir, files, false); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected a refusal naming --force, got %v", err)
	}
	if _, err := writeInitFiles(dir, []initFile{{Name: ".env", Content: "A=2\n", Mode: 0o600}}, true); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if string(got) != "A=2\n" {
		t.Fatalf("--force did not overwrite: %q", got)
	}
	st, _ := os.Stat(filepath.Join(dir, ".env"))
	if st.Mode().Perm() != 0o600 {
		t.Fatalf(".env must stay 0600 after overwrite, got %o", st.Mode().Perm())
	}
}

func TestRunInit_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if err := runInit([]string{"--media", "/srv/media", "--out", dir, "--external-postgres"}, &out); err != nil {
		t.Fatal(err)
	}
	env, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?m)^EVEY_DB_PASSWORD=[0-9a-f]{64}$`).Match(env) {
		t.Errorf("expected a 64-hex generated password in .env:\n%s", env)
	}
	if _, err := os.Stat(filepath.Join(dir, "docker-compose.yml")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "docker compose up -d") {
		t.Errorf("next steps missing from output:\n%s", out.String())
	}
}

func TestRenderInit_BehindProxyIsPlainHTTPOnExternalNetwork(t *testing.T) {
	files, err := renderInit(initOptions{Media: "/srv/media", Port: 9091, Version: "latest", Domain: "media.example.com", BehindProxy: "proxy-net"}, fixedSecret)
	if err != nil {
		t.Fatal(err)
	}
	compose := fileByName(t, files, "docker-compose.yml").Content
	caddy := fileByName(t, files, "Caddyfile").Content
	env := fileByName(t, files, ".env").Content
	for _, want := range []string{"    expose:\n      - \"80\"\n", "networks:\n  default:\n    name: proxy-net\n    external: true\n"} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose lacks %q", want)
		}
	}
	for _, forbidden := range []string{"\"443:443\"", "\"80:80\"", ":9091\""} {
		if strings.Contains(compose, forbidden) {
			t.Errorf("behind a proxy nothing may be published, found %q", forbidden)
		}
	}
	for _, want := range []string{"auto_https off", "trusted_proxies static private_ranges", "\n:80 {\n", "Content-Security-Policy"} {
		if !strings.Contains(caddy, want) {
			t.Errorf("Caddyfile lacks %q", want)
		}
	}
	if strings.Contains(caddy, "{$EVEY_DOMAIN") || strings.Contains(caddy, "Let's Encrypt") {
		t.Error("behind a proxy Caddy must neither obtain a certificate for EVEY_DOMAIN nor claim it does")
	}
	if !strings.Contains(env, "EVEY_SERVER_BASE_URL=https://media.example.com\n") || !strings.Contains(env, "EVEY_FORWARDED_PROTO=true\n") {
		t.Errorf(".env lacks the proxy facts:\n%s", env)
	}
	if _, err := renderInit(initOptions{Media: "/srv/media", Port: 9091, Version: "latest", BehindProxy: "proxy-net"}, fixedSecret); err == nil {
		t.Error("--behind-proxy without --domain must be refused")
	}
}

func TestRenderInit_ExternalMeiliGetsServiceAndKey(t *testing.T) {
	files, err := renderInit(initOptions{Media: "/srv/media", Port: 9091, Version: "latest", ExternalMeili: true}, fixedSecret)
	if err != nil {
		t.Fatal(err)
	}
	compose := fileByName(t, files, "docker-compose.yml").Content
	env := fileByName(t, files, ".env")
	for _, want := range []string{"  meilisearch:\n", "EVEY_EMBEDDED_MEILI: \"false\"", "EVEY_MEILI_URL: http://meilisearch:7700", "meilisearch: { condition: service_healthy }"} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose lacks %q", want)
		}
	}
	if strings.Contains(compose, "  postgres:\n") {
		t.Error("external Meili alone must keep the database embedded")
	}
	if !strings.Contains(env.Content, "MEILI_MASTER_KEY="+strings.Repeat("ab", 32)+"\n") || env.Mode != 0o600 {
		t.Errorf(".env must carry the generated key at 0600:\n%s (mode %o)", env.Content, env.Mode)
	}
}

// The prod layout: everything external, behind an existing proxy network.
func TestRenderInit_OperatorLayoutBehindProxy(t *testing.T) {
	files, err := renderInit(initOptions{Media: "/srv/media", Port: 9091, Version: "beta", Domain: "media.example.com", BehindProxy: "proxy-net", ExternalPostgres: true, ExternalMeili: true}, fixedSecret)
	if err != nil {
		t.Fatal(err)
	}
	compose := fileByName(t, files, "docker-compose.yml").Content
	for _, svc := range []string{"  evey:\n", "  postgres:\n", "  meilisearch:\n", "  caddy:\n"} {
		if strings.Count(compose, svc) != 1 {
			t.Errorf("expected exactly one %q service", strings.TrimSpace(svc))
		}
	}
	env := fileByName(t, files, ".env").Content
	if strings.Contains(env, "generate") {
		t.Errorf("with everything external no secret is generated at boot, the note must not claim so:\n%s", env)
	}
}
