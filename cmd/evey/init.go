package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// `evey init` writes the files a self-hoster needs to run Evey with docker
// compose, so the install is one command instead of a YAML paste and two
// secrets to invent:
//
//	docker run --rm -v "$PWD":/out gravityz/evey init --media /path/to/media
//	docker compose up -d
//
// The default output is ONE service: the image embeds Postgres and
// Meilisearch and generates their secrets on first boot, so `.env` holds
// nothing but the media path. Two opt-ins produce the operator layouts the
// repository's own compose documents: --external-postgres adds the sibling
// database service (with a generated password) for operators who want the
// database lifecycle in their own hands, and --domain adds the bundled Caddy
// reverse proxy with automatic HTTPS.
//
// It is meant to run inside the image, as root, against a mounted output
// directory: the files are handed to that directory's owner so the operator
// does not end up with root-owned files in their home. Nothing here touches a
// database or the network.

type initOptions struct {
	Media            []string // host paths, mounted as /media, /media2, /media3 ...
	Out              string
	Port             int
	Domain           string
	BehindProxy      string // name of an existing Docker network whose proxy owns TLS
	ExternalPostgres bool
	ExternalMeili    bool
	Version          string
	Force            bool
}

// initFile is one generated file; Mode is the permission it is written with.
type initFile struct {
	Name    string
	Content string
	Mode    os.FileMode
}

func runInit(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stdout)
	var o initOptions
	fs.Var((*mediaList)(&o.Media), "media", "absolute host path of a media library (required; repeat for several: /media, /media2, ...)")
	fs.StringVar(&o.Out, "out", "", "directory to write into (default: /out when mounted, else the current directory)")
	fs.IntVar(&o.Port, "port", 9091, "host port to publish the web UI and API on")
	fs.StringVar(&o.Domain, "domain", "", "public domain: adds the Caddy reverse proxy with automatic HTTPS")
	fs.StringVar(&o.BehindProxy, "behind-proxy", "", "existing Docker network of a reverse proxy that already terminates TLS (needs --domain): the server joins it, nothing is published, no Caddy")
	fs.BoolVar(&o.ExternalPostgres, "external-postgres", false, "run Postgres as a separate compose service instead of embedded")
	fs.BoolVar(&o.ExternalMeili, "external-meili", false, "run Meilisearch as a separate compose service instead of embedded")
	fs.StringVar(&o.Version, "version", "latest", "image tag to pin in .env (e.g. v0.1.0, beta-0.0.1)")
	fs.BoolVar(&o.Force, "force", false, "overwrite files that already exist")
	fs.Usage = func() {
		fmt.Fprintf(stdout, `usage: evey init --media /path/to/media [flags]

Writes docker-compose.yml for a single-container Evey install (plus a
Caddyfile with --domain, and a .env holding the shared
secrets with --external-postgres / --external-meili). Run it from the image:

  docker run --rm -v "$PWD":/out gravityz/evey init --media /path/to/media
  docker compose up -d

Flags:
`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if o.Out == "" {
		o.Out = defaultInitOut()
	}

	files, err := renderInit(o, randomSecretHex)
	if err != nil {
		return err
	}
	written, err := writeInitFiles(o.Out, files, o.Force)
	if err != nil {
		return err
	}
	printInitNextSteps(stdout, o, written)
	return nil
}

// mediaList lets --media repeat: the first path is /media, the next ones
// /media2, /media3 ... - the mount names the entrypoint's owner detection and
// the library picker already expect.
type mediaList []string

func (m *mediaList) String() string     { return strings.Join(*m, ", ") }
func (m *mediaList) Set(v string) error { *m = append(*m, v); return nil }

func mediaMount(i int) string {
	if i == 0 {
		return "/media"
	}
	return fmt.Sprintf("/media%d", i+1)
}

// defaultInitOut picks /out when the operator mounted one (the documented
// `docker run -v "$PWD":/out` form), otherwise the working directory.
func defaultInitOut() string {
	if st, err := os.Stat("/out"); err == nil && st.IsDir() {
		return "/out"
	}
	return "."
}

func randomSecretHex() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// renderInit produces the files for o. secret is injected so tests can pin
// it; production passes randomSecretHex.
func renderInit(o initOptions, secret func() (string, error)) ([]initFile, error) {
	if len(o.Media) == 0 {
		return nil, errors.New("--media is required: the absolute host path of your media library")
	}
	for _, m := range o.Media {
		if !filepath.IsAbs(m) {
			return nil, fmt.Errorf("--media must be an absolute host path (got %q): docker bind mounts do not resolve relative paths", m)
		}
		if strings.ContainsAny(m, "\n\r:") {
			return nil, fmt.Errorf("--media must not contain line breaks or ':' (got %q)", m)
		}
	}
	if o.Port < 1 || o.Port > 65535 {
		return nil, fmt.Errorf("--port must be between 1 and 65535 (got %d)", o.Port)
	}
	if strings.ContainsAny(o.Domain, " /:\n\r") {
		return nil, fmt.Errorf("--domain must be a bare host name (got %q)", o.Domain)
	}
	if o.Version == "" || strings.ContainsAny(o.Version, " \n\r") {
		return nil, fmt.Errorf("--version must be an image tag (got %q)", o.Version)
	}
	if o.BehindProxy != "" {
		if o.Domain == "" {
			return nil, errors.New("--behind-proxy needs --domain: the upstream proxy routes a name, and the server must know its public URL")
		}
		if strings.ContainsAny(o.BehindProxy, " \n\r\"'") {
			return nil, fmt.Errorf("--behind-proxy must be a Docker network name (got %q)", o.BehindProxy)
		}
	}
	proxied := o.Domain != ""

	var b strings.Builder
	b.WriteString("# Generated by `evey init`. Safe to edit; `docker compose up -d` applies changes.\n")
	b.WriteString("# Every runtime setting (transcoding, hardware acceleration, libraries, plugins)\n")
	b.WriteString("# lives in Admin -> Server config once the server is up, not in this file.\n")
	b.WriteString("services:\n")
	b.WriteString("  evey:\n")
	// ${EVEY_VERSION:-tag}: the tag is pinned right here, and an operator can
	// still override it for one run (EVEY_VERSION=x docker compose up -d) -
	// what scripts/evey-update.sh and evey-rollback.sh rely on.
	fmt.Fprintf(&b, "    image: gravityz/evey:${EVEY_VERSION:-%s}\n", o.Version)
	b.WriteString("    container_name: evey-server\n")
	if !proxied {
		fmt.Fprintf(&b, "    ports:\n      - \"%d:9091\"\n", o.Port)
	} else if o.BehindProxy != "" {
		fmt.Fprintf(&b, "    # No host port: your reverse proxy on %s reaches this container as\n", o.BehindProxy)
		b.WriteString("    # evey-server:9091 (streaming needs no buffering and no timeout there, see\n")
		b.WriteString("    # docs/reverse-proxy.md). Publish 9091 as well if LAN clients should bypass it.\n")
	} else {
		b.WriteString("    # No host port: Caddy below is the only way in (HTTPS on your domain).\n")
		b.WriteString("    # Publish 9091 here as well if LAN clients should bypass the proxy.\n")
	}
	// A mapping that holds only comments is a YAML null, which compose
	// rejects ("environment must be a mapping"): open the block only when a
	// real key goes in, and keep the hints commented out otherwise.
	if proxied || o.ExternalPostgres || o.ExternalMeili {
		b.WriteString("    environment:\n")
	} else {
		b.WriteString("    # environment:\n")
	}
	switch {
	case o.BehindProxy != "":
		fmt.Fprintf(&b, "      EVEY_SERVER_BASE_URL: https://%s\n", o.Domain)
		b.WriteString("      # Your proxy sits on the Docker network: trust it for client IPs (narrow\n")
		b.WriteString("      # to its subnet once you know it) and for the https scheme it forwards.\n")
		b.WriteString("      EVEY_TRUSTED_PROXIES: 172.16.0.0/12\n")
		b.WriteString("      EVEY_FORWARDED_PROTO: \"true\"\n")
	case o.Domain != "":
		fmt.Fprintf(&b, "      EVEY_SERVER_BASE_URL: https://%s\n", o.Domain)
		b.WriteString("      # Caddy terminates TLS on the compose network: trust it for client IPs.\n")
		b.WriteString("      EVEY_TRUSTED_PROXIES: 172.16.0.0/12\n")
		b.WriteString("      EVEY_FORWARDED_PROTO: \"true\"\n")
	default:
		b.WriteString("    #   # Public URL, used for e-mail links, OIDC redirects and cast/device URLs:\n")
		fmt.Fprintf(&b, "    #   EVEY_SERVER_BASE_URL: http://<your-ip>:%d\n", o.Port)
	}
	if o.ExternalPostgres {
		b.WriteString("      EVEY_DB_HOST: postgres\n")
		b.WriteString("      EVEY_DB_SSLMODE: disable   # private compose network; the bundled Postgres has no TLS\n")
	}
	if o.ExternalMeili {
		b.WriteString("      EVEY_EMBEDDED_MEILI: \"false\"\n")
		b.WriteString("      EVEY_MEILI_URL: http://meilisearch:7700\n")
	}
	if proxied || o.ExternalPostgres || o.ExternalMeili {
		b.WriteString("      # Optional first-boot seeds (then managed in Admin -> Server config -> API keys;\n")
		b.WriteString("      # TMDB works out of the box with a built-in key, set yours to replace it):\n")
		b.WriteString("      # EVEY_TMDB_API_KEY: \"\"\n      # EVEY_OMDB_API_KEY: \"\"\n")
	} else {
		b.WriteString("    #   # Optional first-boot seeds (then managed in Admin -> Server config -> API keys;\n")
		b.WriteString("    #   # TMDB works out of the box with a built-in key, set yours to replace it):\n")
		b.WriteString("    #   EVEY_TMDB_API_KEY: \"\"\n    #   EVEY_OMDB_API_KEY: \"\"\n")
	}
	withEnvFile := o.ExternalPostgres || o.ExternalMeili
	if withEnvFile {
		b.WriteString("    env_file: .env          # the secrets shared with the sibling services\n")
	}
	b.WriteString("    volumes:\n")
	switch {
	case o.ExternalPostgres && o.ExternalMeili:
		b.WriteString("      - ./evey-data:/data     # JWT key, logs, backups, plugins\n")
	case o.ExternalPostgres:
		b.WriteString("      - ./evey-data:/data     # JWT key, generated secrets, search index, logs, backups\n")
	case o.ExternalMeili:
		b.WriteString("      - ./evey-data:/data     # database, JWT key, generated secrets, logs, backups\n")
	default:
		b.WriteString("      - ./evey-data:/data     # database, JWT key, generated secrets, search index, logs, backups\n")
	}
	b.WriteString("      - evey-cache:/cache     # transcode + trickplay cache (safe to prune)\n")
	b.WriteString("      # Read-write so the repair features (seek-index rebuild) work in place;\n")
	b.WriteString("      # append `:ro` for a read-only library and accept that repairs fail.\n")
	for i, m := range o.Media {
		fmt.Fprintf(&b, "      - %s:%s\n", m, mediaMount(i))
	}
	b.WriteString("    # The embedded database is stopped cleanly after the server on `docker stop`;\n")
	b.WriteString("    # a large catalogue needs more than Docker's 10s default before the kill.\n")
	b.WriteString("    stop_grace_period: 60s\n")
	if o.ExternalPostgres || o.ExternalMeili {
		b.WriteString("    depends_on:\n")
		if o.ExternalPostgres {
			b.WriteString("      postgres: { condition: service_healthy }\n")
		}
		if o.ExternalMeili {
			b.WriteString("      meilisearch: { condition: service_healthy }\n")
		}
	}
	b.WriteString("    restart: unless-stopped\n")

	if o.ExternalPostgres {
		b.WriteString(`
  # Kept as a sibling service rather than embedded: Postgres major upgrades
  # (16->17) need a dump+restore step, and a separate container makes that
  # lifecycle explicit and lets you swap in a managed Postgres later.
  postgres:
    image: postgres:16-alpine
    container_name: evey-postgres
    environment:
      POSTGRES_DB: ${EVEY_DB_NAME:-evey}
      POSTGRES_USER: ${EVEY_DB_USER:-evey}
      POSTGRES_PASSWORD: ${EVEY_DB_PASSWORD:?EVEY_DB_PASSWORD must be set in .env}
    volumes:
      - ./postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${EVEY_DB_USER:-evey}"]
      interval: 5s
      timeout: 3s
      retries: 5
    restart: unless-stopped
`)
	}
	if o.ExternalMeili {
		b.WriteString(`
  # Search engine as its own service: separate lifecycle and volume, and
  # "docker compose restart meilisearch" resets just the index (it is rebuilt
  # from Postgres, so ./meili-data is expendable).
  meilisearch:
    image: getmeili/meilisearch:v1.12
    container_name: evey-meilisearch
    environment:
      MEILI_MASTER_KEY: ${MEILI_MASTER_KEY:?MEILI_MASTER_KEY must be set in .env}
      MEILI_NO_ANALYTICS: "true"
    volumes:
      - ./meili-data:/meili_data
    healthcheck:
      test: ["CMD", "curl", "-sf", "http://localhost:7700/health"]
      interval: 5s
      timeout: 3s
      retries: 5
    restart: unless-stopped
`)
	}
	if o.Domain != "" && o.BehindProxy == "" {
		b.WriteString(`
  # Reverse proxy with automatic HTTPS (Let's Encrypt) for EVEY_DOMAIN.
  # Ports 80 and 443 must reach this host from the internet for the
  # certificate to be issued. Replace with your own proxy if you have one:
  # docs/reverse-proxy.md has Nginx and Traefik equivalents.
  caddy:
    image: caddy:2.9-alpine
    container_name: evey-caddy
    ports:
      - "80:80"
      - "443:443"
    environment:
      EVEY_DOMAIN: `)
		b.WriteString(o.Domain)
		b.WriteString(`
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy-data:/data
      - caddy-config:/config
    depends_on:
      - evey
    restart: unless-stopped
`)
	}
	b.WriteString("\nvolumes:\n  evey-cache:\n")
	if o.Domain != "" && o.BehindProxy == "" {
		b.WriteString("  caddy-data:      # TLS certificates (re-requested if deleted)\n  caddy-config:\n")
	}
	if o.BehindProxy != "" {
		b.WriteString("\n# Every service joins the proxy's existing network instead of a private one.\n")
		fmt.Fprintf(&b, "networks:\n  default:\n    name: %s\n    external: true\n", o.BehindProxy)
	}
	files := []initFile{{Name: "docker-compose.yml", Content: b.String(), Mode: 0o644}}

	// .env exists only to hold a secret shared between services (0600); a
	// single-container install has none and gets no .env at all.
	if withEnvFile {
		var e strings.Builder
		e.WriteString("# Generated by `evey init`: secrets shared between the compose services.\n")
		e.WriteString("# Everything else is in docker-compose.yml, or in Admin -> Server config.\n\n")
		if o.ExternalPostgres {
			pw, err := secret()
			if err != nil {
				return nil, err
			}
			e.WriteString("# Database password, shared by the postgres service and the server.\n")
			fmt.Fprintf(&e, "EVEY_DB_PASSWORD=%s\n", pw)
		}
		if o.ExternalMeili {
			key, err := secret()
			if err != nil {
				return nil, err
			}
			e.WriteString("# Search master key, shared by the meilisearch service and the server.\n")
			fmt.Fprintf(&e, "MEILI_MASTER_KEY=%s\n", key)
		}
		files = append(files, initFile{Name: ".env", Content: e.String(), Mode: 0o600})
	}

	if o.Domain != "" && o.BehindProxy == "" {
		files = append(files, initFile{Name: "Caddyfile", Content: initCaddyfile, Mode: 0o644})
	}
	return files, nil
}

// writeInitFiles writes files under dir, refusing to clobber an existing
// file unless force. Running as root against a mounted directory (the
// documented form), it hands every file to the directory's owner.
func writeInitFiles(dir string, files []initFile, force bool) ([]string, error) {
	// A directory created here (--out pointing at a new subdirectory) belongs
	// to whoever owns its parent, like the files inside it.
	uid, gid := -1, -1
	if _, err := os.Stat(dir); err != nil {
		uid, gid = ownerOf(filepath.Dir(filepath.Clean(dir)))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	if uid >= 0 {
		if err := os.Chown(dir, uid, gid); err != nil {
			fmt.Fprintf(os.Stderr, "init: %s stays owned by root (chown to %d:%d failed: %v)\n", dir, uid, gid, err)
		}
	}
	if !force {
		for _, f := range files {
			if _, err := os.Stat(filepath.Join(dir, f.Name)); err == nil {
				return nil, fmt.Errorf("%s already exists in %s - pass --force to overwrite it", f.Name, dir)
			}
		}
	}
	uid, gid = ownerOf(dir)
	var written []string
	for _, f := range files {
		path := filepath.Join(dir, f.Name)
		if err := os.WriteFile(path, []byte(f.Content), f.Mode); err != nil {
			return written, fmt.Errorf("write %s: %w", path, err)
		}
		// WriteFile honours the umask on create only; an overwritten .env
		// must still end up secret-tight.
		if err := os.Chmod(path, f.Mode); err != nil {
			return written, fmt.Errorf("chmod %s: %w", path, err)
		}
		if uid >= 0 {
			// Best effort: a filesystem that refuses chown (some NAS shares)
			// still gets usable, merely root-owned, files - say so.
			if err := os.Chown(path, uid, gid); err != nil {
				fmt.Fprintf(os.Stderr, "init: %s stays owned by root (chown to %d:%d failed: %v)\n", path, uid, gid, err)
			}
		}
		written = append(written, path)
	}
	return written, nil
}

func printInitNextSteps(w io.Writer, o initOptions, written []string) {
	fmt.Fprintln(w, "Wrote:")
	for _, p := range written {
		fmt.Fprintf(w, "  %s\n", p)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Next:")
	fmt.Fprintln(w, "  docker compose up -d")
	switch {
	case o.BehindProxy != "":
		fmt.Fprintf(w, "  in your reverse proxy, route %s to evey-server:9091 (network %s) with streaming\n  buffering and timeouts off (docs/reverse-proxy.md), then open https://%s\n", o.Domain, o.BehindProxy, o.Domain)
	case o.Domain != "":
		fmt.Fprintf(w, "  point DNS for %s at this host, open ports 80/443, then open https://%s\n", o.Domain, o.Domain)
	default:
		fmt.Fprintf(w, "  open http://<your-ip>:%d and create the first account\n", o.Port)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Keep ./evey-data: it holds the database, the JWT signing key and the generated secrets.")
	fmt.Fprintln(w, "To update later: docker compose pull && docker compose up -d")
}
