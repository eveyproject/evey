# ADR 0026 - Remote access inferred from a published URL, unless the operator chose

Status: Accepted

## Context

Evey has a network gate: when remote access is off, any client whose address is
not inside `EVEY_REMOTE_ACCESS_LAN_NETWORKS` is refused on every route. It is a
deployment choice ("this instance is for my own network"), not a security layer
- authentication is what protects the library.

The gate defaulted to off, and `false` carried two meanings that nothing in the
system could tell apart: *the operator wants a LAN-only instance*, and *nobody
ever touched this setting*. On a server published behind a reverse proxy the
second reading is almost always the true one.

That ambiguity took a household offline for two days. The gate had been
dormant because the proxy did not forward the real client
address, so every request looked private and was allowed. Adding
`trusted_proxies static private_ranges` to the Caddy configuration made client
addresses truthful - and every remote client immediately started getting `403`
on every route. No application setting had changed. The failure produced:

- no server-side log at all, so the operator had nothing to search for;
- an app rendering raw i18n keys, because the translation catalogue was gated
  too, which reads as "the app is broken" rather than "the server refused me";
- a symptom ("it works on your phone, not on mine") that points at the client.

## Decision

1. **Infer the default from whether the server is published.** At boot,
   `config.ResolveRemoteAccess` enables remote access when
   `EVEY_REMOTE_ACCESS_EXTERNAL_URL` or `EVEY_SERVER_BASE_URL` is a publicly
   reachable address. A server you published is meant to be reached.
2. **Never override an explicit choice.** `RemoteAccessConfig.EnabledExplicit`
   records that a human decided - the environment variable was set (to either
   value), or the admin panel saved it. Inference only fills a blank. Without
   this flag the ambiguity above simply moves rather than disappears.
3. **The gate must never be silent.** The decision is logged at boot, with a
   WARN naming the remedy when a published server is refusing everyone;
   refusals are logged with the client address, path and user agent, rate
   limited per address; `evey admin doctor` reports "published but refusing" as
   a FAIL.
4. **A refusal must stay explainable to the client.** `/healthz`,
   `/api/v1/setup/status` and `/api/v1/i18n/` are exempt from the gate: none
   serves library content or accepts credentials, and gating the catalogue
   destroys the only channel left to explain the refusal. The response carries a
   stable `{"code":"remote_access_disabled"}` and an `X-Evey-Remote-Access`
   header, which the web, mobile and TV clients turn into a named message.

## Consequences

- A published instance whose operator never touched the setting now serves
  remote clients by default. This is a **change of default towards openness**,
  and it is deliberate: the gate is a deployment preference, and every route
  behind it still requires authentication. An operator wanting a LAN-only
  published server states it once, and the inference then leaves it alone
  forever.
- `EnabledExplicit` must be persisted and set on both write paths (environment
  parsing and the admin panel). Dropping it while "simplifying" would silently
  restore the ambiguity that caused the incident - see the tests in
  `internal/config/remote_access_resolve_test.go`.
- The exempt list is a security-relevant surface. Adding a path to it must be
  argued on the same grounds used here: it serves no library content, accepts no
  credentials, and gating it breaks the ability to explain a refusal.
- Diagnosing the reverse-proxy half still requires the operator: correct client
  addresses depend on the proxy forwarding them and on `EVEY_TRUSTED_PROXIES`
  covering the proxy. `docs/reverse-proxy.md` documents that turning real client
  IPs on is exactly what wakes this gate up.
