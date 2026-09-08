# What Evey is trying to be

Five things drive the design. When a change has to give something up, this is
the order in which it gives it up.

## Safe to put on the internet

Evey is meant to be reachable, not hidden on a LAN. So the defaults assume a
hostile network: remote access is inferred from whether you published a URL,
authentication fails closed when the database is slow rather than letting a
request through, plugin tokens are refused by default and only unlock what the
manifest declared, and the server never runs as root.

## Installable in a few lines

One `docker-compose.yml`, no secret to invent, no second container to wire.
The image generates its own credentials on first boot and runs its database and
search index inside itself. Everything an operator might want to change later
lives in the admin panel, not in environment variables. See
[ADR-0028](adr/0028-install-is-one-compose-file.md).

## Extensible in whatever language you write

A plugin is a process the server launches and talks to over gRPC on a local
socket. That is the whole contract, so the language is yours: pre-generated SDK
stubs ship for seven of them, and anything the entrypoint can launch works
without one. Extensions are confined by the kernel rather than trusted, which is
what makes running someone else's code a reasonable proposition.

## Fast, and honest about it

Playback prefers to send the file untouched, transcodes only what the client
cannot handle, and decides once per request rather than recomputing. The
measurements that drive those choices are taken, not assumed - a scan probe that
looked like a good idea was kept only after being proven equivalent to the tool
it replaces on real files.

## For many viewers at once, on modest hardware

The target is a NAS serving a household and its friends, not a server rack. So
concurrency is bounded on purpose: encoders are reused across requests, sessions
are admitted against a budget, telemetry is dropped rather than allowed to slow
a segment down, and the server sizes its worker pools to the machine it finds.

---

Two things Evey deliberately is not. It is not a catalogue of every media type
done adequately - playback is what it owns. And it is not a product that asks
you to trust it: what the server does, and what it still cannot prevent, is
written down in [SECURITY.md](SECURITY.md) rather than implied.
