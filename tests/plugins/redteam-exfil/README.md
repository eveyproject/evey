# redteam-exfil

A red-team FRONTEND fixture (companion to `redteam-bridge`). Its iframe attempts
eight browser-side data-exfiltration vectors and renders `BLOCKED`/`BYPASSED`
per vector, so Evey's containment can be demonstrated on demand.

Evey's defense is containment - a strict per-document CSP plus a locked-down
iframe `sandbox` attribute - and it must hold whatever the browser underneath
does. This fixture therefore attempts the DELIVERY vectors and asserts they are
blocked; it deliberately ships **no working exploit** for any real browser CVE.
A weaponised V8/SpiderMonkey chain would be a real-world weapon usable against
any site, and it would not test Evey anyway: the question is whether an exfil
attempt escapes the sandbox, not whether a given browser has a bug. The CVE
classes are referenced by name only (renderer RCE bounded by `connect-src`;
Spectre-class timing bounded by the sandbox withholding `SharedArrayBuffer`).

## Vectors and expected verdict

| Probe | Vector | Expected | Enforced by |
|---|---|---|---|
| fetch_beacon | cross-origin `fetch` POST to a sink | **BLOCKED** | CSP `connect-src` |
| send_beacon | `navigator.sendBeacon` | **BLOCKED** | CSP `connect-src` |
| image_pixel | `new Image().src` to a sink | **BLOCKED** | CSP `img-src` |
| websocket | `new WebSocket` to an attacker host | **BLOCKED** | CSP `connect-src` (ws/wss) |
| parent_dom_access | read `window.parent.location` | **BLOCKED** | sandbox: no `allow-same-origin` (opaque origin) |
| postmessage_to_host | `postMessage` loot to the host | host ignores it | host validates origin/shape |
| top_navigation | `top.location = sink#loot` | **BLOCKED** | sandbox: no `allow-top-navigation` |
| spectre_primitive | is `SharedArrayBuffer` available | **BLOCKED** | no cross-origin isolation in the sandbox |

The in-gate proof that these hold is
`apps/web/src/features/plugins/PluginIframe.test.tsx` -> "denies the browser
exfiltration vectors at the sandbox boundary", which asserts the CSP and the
sandbox attribute the fixture depends on.
