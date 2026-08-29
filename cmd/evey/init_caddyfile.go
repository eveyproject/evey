package main

// initCaddyfile is the bundled reverse-proxy config `evey init --domain`
// writes next to the compose file. It MUST stay byte-identical to
// docker/Caddyfile (the one the repository's own compose builds into its Caddy
// image): TestInitCaddyfileMatchesBundled fails otherwise. Two copies exist
// only because go:embed cannot reach outside cmd/evey.
const initCaddyfile = `# Bundled reverse proxy. The site address is injected from the environment:
# set EVEY_DOMAIN in your .env (compose passes it through) and Caddy obtains
# a Let's Encrypt certificate for it automatically. Default: localhost.
{$EVEY_DOMAIN:localhost} {
	encode gzip

	header {
		X-Content-Type-Options nosniff
		X-Frame-Options DENY
		Referrer-Policy strict-origin-when-cross-origin
		# The canonical Evey CSP for a curating proxy - keep in sync with
		# docs/reverse-proxy.md (Security headers & CSP). Load-bearing tokens:
		#   'unsafe-inline'           plugin UIs (bundle inlined in a srcdoc iframe,
		#                             which inherits THIS policy - see helpers.go)
		#   'wasm-unsafe-eval'        browser AC-3/E-AC-3 decoder (Wasm compile only)
		#   https://www.gstatic.com   Google Cast sender SDK (script + frame)
		#   media-src blob:           MSE playback (hls.js)
		#   worker-src blob:          hls.js demux worker
		Content-Security-Policy "default-src 'self'; img-src 'self' https: data: blob:; media-src 'self' blob:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com; connect-src 'self' https: wss:; worker-src 'self' blob:; font-src 'self' data:; frame-src https://www.gstatic.com"
		Strict-Transport-Security "max-age=31536000; includeSubDomains"
	}

	# Streaming endpoints need no timeout
	@streaming {
		path /api/v1/media/*/transcode /api/v1/media/*/hls/* /api/v1/media/*/stream /api/v1/libraries/*/scan
	}
	handle @streaming {
		reverse_proxy evey:9091 {
			transport http {
				read_timeout 0
				write_timeout 0
			}
			flush_interval -1
			# Exactly ONE CSP must reach the browser (two = the browser
			# enforces the intersection, silently). This proxy curates the
			# policy above, so the app's own header is stripped.
			header_down -Content-Security-Policy
		}
	}

	# Everything else goes to the backend (which serves both API and frontend)
	handle {
		reverse_proxy evey:9091 {
			flush_interval -1
			header_down -Content-Security-Policy
		}
	}
}
`
