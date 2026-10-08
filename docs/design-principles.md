# Design principles

Seven constraints the code follows, each with the pages and decision records
where it shows. Read this page to understand why Evey behaves as it does
before reading how it is built ([Architecture](ARCHITECTURE.md)) or why a
structural choice was made ([decision records](adr/README.md)). When two
principles conflict, the earlier one in this list wins.

| Principle | Where it shows | Decided by |
| --- | --- | --- |
| 1. Access is refused when it cannot be checked | parental control, plugin install, remote access | [ADR-0026](adr/0026-remote-access-inferred-from-a-published-url.md) |
| 2. Third-party code is contained, not trusted | plugin sandbox | [ADR-0012](adr/0012-plugin-token-deny-by-default.md), [ADR-0014](adr/0014-landlock-plugin-sandbox.md) |
| 3. The original is delivered whenever it can be | playback decision | [ADR-0030](adr/0030-one-plan-one-decision.md) |
| 4. Decisions rest on what was proved, not on names | casting, hardware acceleration | [ADR-0029](adr/0029-transcoding-routes-are-measured-not-assumed.md) |
| 5. One playback, one decision | playback plan, Stream info | [ADR-0030](adr/0030-one-plan-one-decision.md), [ADR-0024](adr/0024-playback-plan-resolver.md) |
| 6. The server sizes itself to the machine | calibration, background work | - |
| 7. Few settings, each in one place | install, admin panel | [ADR-0028](adr/0028-install-is-one-compose-file.md) |

## 1. Access is refused when it cannot be checked

Where a decision grants access, an error resolves to the most restrictive
answer. A profile whose parental settings cannot be read gets no content; a
plugin whose code cannot be analysed is not installed; remote access follows
an explicit choice once one was made, and says loudly when it refuses
everyone. Remote access and authentication are separate: the first is a
deployment choice, the second is what protects the library.

See [Users, profiles and parental control](administration/users.md),
[Remote access and HTTPS](administration/remote-access.md),
[ADR-0026](adr/0026-remote-access-inferred-from-a-published-url.md).

## 2. Third-party code is contained, not trusted

A plugin gets the permissions its manifest declares and nothing else, is
analysed before installation, runs in a sandbox with no direct network access,
and holds a token that is denied by default. What the sandbox still cannot
prevent is written down rather than implied.

See [Plugin sandbox and threat model](SECURITY.md),
[ADR-0012](adr/0012-plugin-token-deny-by-default.md),
[ADR-0014](adr/0014-landlock-plugin-sandbox.md).

## 3. The original is delivered whenever it can be

The preferred outcome is the file as it is, then the same tracks in another
container, then a transcode of only the tracks that need it. Quality is not
something the viewer has to choose before playing: the server keeps the
source's quality unless a real constraint (the device, the connection, a
limit) prevents it.

See [How playback is decided](playback/how-playback-is-decided.md).

## 4. Decisions rest on what was proved, not on names

A player's capabilities come from what its device reports, not from a list of
known devices. A graphics card is used for what it passed during measurement,
not for what its model should support. When a capability cannot be proved,
the server takes the conservative answer: an unknown cast receiver gets the
capabilities of the oldest one, and pixel formats players do not yet report
follow a fixed compatibility rule. Assuming too much gives a black screen or
wrong colours; assuming too little costs a transcode.

See [Casting](playback/casting.md), [Hardware acceleration](administration/hardware-acceleration.md),
[ADR-0029](adr/0029-transcoding-routes-are-measured-not-assumed.md).

## 5. One playback, one decision

A playback is decided once, when it starts, and every manifest and segment of
the session is served from that decision. The player never receives segments
from two encodings, and what Stream info reports is derived from the same
serving plan the playback pipeline uses, checked by tests in both directions.

See [Playback pipeline](playback-serving.md),
[ADR-0030](adr/0030-one-plan-one-decision.md),
[ADR-0024](adr/0024-playback-plan-resolver.md).

## 6. The server sizes itself to the machine

The server measures how much it can transcode and admits no more: a small
machine plays fewer transcoded streams, rather than playing all of them badly.
Playback comes first; background work pauses while someone watches. The same
image runs on a NAS and on a server with several graphics cards.

See [Transcoding and server capacity](playback/transcoding.md),
[Background work and server load](operations/background-work.md).

## 7. Few settings, each in one place

What must exist before the server starts (storage, secrets, the public
address) is in the compose file; everything that can change later is in the
admin panel, stored in the database, and applies without a restart wherever
nothing requires it. Detection and measurement replace settings wherever they
can: an installation is one compose file, libraries and users.

See [Install](installation.md), [Configuration reference](configuration.md),
[ADR-0028](adr/0028-install-is-one-compose-file.md).

## Decision records

Structural choices are recorded as architecture decision records, with the
context, the options weighed and the consequences. They are kept with the
code: [docs/adr](adr/README.md).

<!-- sources
fail closed: internal/adapters/driving/http/parental.go - BlockedByParental ; internal/app/plugin/install.go
contained plugins: docs/SECURITY.md ; docs/adr/0012-plugin-token-deny-by-default.md ; docs/adr/0014-landlock-plugin-sandbox.md
original first, one decision: internal/app/transcoding/decision.go ; internal/app/transcoding/route_tonemap_report_test.go ; docs/adr/0030-one-plan-one-decision.md
proved capabilities: frontend/packages/media-caps/src/castUrl.ts - castProfileFor ; docs/adr/0029-transcoding-routes-are-measured-not-assumed.md
sizing: internal/app/transcoding/calibration_service.go ; i18n admin.serverConfig.cpuBudget.globalDesc
-->
