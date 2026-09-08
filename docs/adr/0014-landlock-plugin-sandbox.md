# ADR-0014: Landlock plugin sandbox, applied by a launcher shim

- Status: Accepted
- Date: 2026-05-24

## Context

Plugins are third-party processes on someone's home server. Confining them must
work in an ordinary unprivileged container, because that is the only environment
we can guarantee: anything needing `--privileged`, a relaxed seccomp profile or
a two-step install would simply not be applied.

## Decision

A small launcher is prepended to the plugin command line and does its work
before handing over to the plugin. It drops to an unprivileged uid when asked,
applies a Landlock filesystem allowlist - the plugin's own directory and its
socket, system libraries read-only, everything else including `/data` and
sibling plugins denied - then installs a kernel seccomp denylist, and only then
executes the plugin.

Landlock confines the calling thread from inside the process. It needs no mount,
no namespace and no privilege, so it runs under Docker's default seccomp and
AppArmor with nothing for the operator to configure.

Seccomp is installed **before** the exec, which is what makes it apply to
compiled plugins too. A preloaded library cannot: a static binary never loads it.

The default mode resolves once, strongest first: Landlock, then bubblewrap where
Landlock is absent and namespaces are available, then a basic mode. Basic is not
"no isolation" - uid separation, seccomp and the resource monitor still apply;
it loses per-path filesystem confinement, and says so in the admin panel.

**No plugin has direct network access, whatever its manifest says.** A plugin
that needs the network asks the host over its local socket, and the host
performs the request: allow-list enforced per host, GET and HEAD only, private
and link-local addresses refused after DNS resolution so redirects are
re-checked. The earlier design filtered by hostname and port, which a literal IP
walked straight through - cloud metadata included.

## Consequences

- Works with zero operator setup in a non-privileged container, and degrades
  visibly rather than silently on kernels too old for Landlock.
- Fail-closed where it counts: an explicit `landlock` mode refuses to start a
  plugin it cannot confine.
- The egress broker is now the security-critical chokepoint, which is why an
  operator should still block link-local and internal ranges at the network
  layer. Two independent layers fail in different ways; one does not.
- This is a trust-but-verify boundary for community code on your own machine. It
  is not a jail against an attacker with a kernel exploit, and it is not sold
  as one.
