---
source_sha: 4f36e08eb2ed47e2b70b77f7f85f7caa2a9229e3bb755c85c2d54804d13c4031
reviewed: true
---
# Plugin-API-Referenz

Die Schnittstelle, für die ein Plugin geschrieben wird: die Felder von
`plugin.yaml`, die zwei gRPC-Dienste, die Backend und Server austauschen,
und die Brücke `window.evey`, die ein Oberflächenmodul in seinem Frame
bekommt. Die Schritt-für-Schritt-Anleitung, die sie benutzt, ist
[Ein Plugin schreiben](dev-setup.md). Was jede Berechtigung erlaubt und was
der Installer ablehnt, steht in [Berechtigungen und Slots](security-model.md).

## Manifest

Jedes ZIP enthält auf oberster Ebene eine `plugin.yaml` (oder `plugin.yml`),
höchstens 1 MB groß.

```yaml title="plugin.yaml"
name: my-plugin              # Pflicht; eindeutig, kebab-case
version: 1.2.3               # Pflicht; semver, X.Y.Z oder X.Y.Z-label
kind: plugin                 # Pflicht; "plugin" oder "theme"
language: nodejs             # Pflicht für ein Plugin; eine Laufzeit des Images
entrypoint: node index.js    # Pflicht für ein Plugin; die Befehlszeile, die der Server startet

description: "..."           # optional
author: "..."                # optional
min_evey_version: "1.0.0"    # optional; wird festgehalten, heute nicht durchgesetzt

permissions:                 # was Backend und Oberfläche aufrufen dürfen
  - network
  - media.read
external_fetch:              # Pflicht mit "network"; nur reine Hostnamen
  - api.themoviedb.org
capabilities:                # die Rollen, die das Plugin zu erfüllen angibt
  - metadata.provider

web_root: public/            # Pflicht, wenn frontend angegeben ist; als statische Dateien ausgeliefert
frontend:
  - slot: home.section       # einer der erlaubten Slots
    src: public/front.js     # muss unter web_root liegen

config_schema:               # die Felder des Bereichs Konfiguration auf der Karte des Plugins
  api_key:
    type: password           # wird nie an einen Client zurückgeschickt
    label: API key
    required: true
  city:
    type: string
    default: Paris
    description: Resolved through geocoding
```

| Feld | Regel |
|---|---|
| `name`, `version`, `kind` | Pflicht. `version` muss `X.Y.Z` oder `X.Y.Z-label` entsprechen; ein Update muss eine höhere Version tragen als die installierte. |
| `language` | Pflicht für `kind: plugin`: `python`, `nodejs`, `php`, `ruby`, `lua`, `bash`, `sh` oder `go`. Es bestimmt die Prüfung der Laufzeit und die Stubs, die neben den Einstiegspunkt kopiert werden; es startet selbst nichts. |
| `entrypoint` | Pflicht für `kind: plugin`. Das erste Wort ist der Interpreter (oder die Binärdatei, mit `native-binary`), der Rest sind seine Argumente; der Prozess läuft im Verzeichnis des Plugins. |
| `permissions` | Jeder Wert muss im [Katalog der Berechtigungen](security-model.md#permissions-catalog) stehen. `network` verlangt ein nicht leeres `external_fetch`. |
| `external_fetch` | Reine Hostnamen. Ein Schema wird geduldet und entfernt; ein Platzhalter oder eine IP-Adresse lässt die Installation scheitern ([Regeln](security-model.md#external-fetch-rules)). |
| `capabilities` | `metadata.provider`, `media.read`, `media.write`, `webhook.receiver` oder `notification.sender`; alles andere wird abgelehnt. Ein Theme darf keine angeben. |
| `web_root` | Pflicht, wenn `frontend` nicht leer ist; relativ, kein `..`, kein Backslash. `.` liefert das ganze Plugin-Verzeichnis aus und schreibt nur eine Warnung ins Protokoll. |
| `frontend[]` | `slot` muss in der [Liste der Slots](#slots) stehen; `src` muss unter `web_root` liegen. |
| `config_schema` | Eine Zuordnung von Feldname zu `{type, label, description, required, default, min, max, options}`. Typen: `string`, `text`, `password`, `boolean`, `integer`, `number`, `select` (mit `options: [{value, label}]`). |

Ein Theme (`kind: theme`) darf `language`, `entrypoint`, `permissions` oder
`capabilities` nicht angeben. Sein Format steht unter [Themes](../themes.md).

**Geheimnisse.** Ein Feld mit `type: password` oder mit `secret: true` wird
wie üblich gespeichert und an das Backend übergeben, kommt aber aus jeder
Leseroute maskiert zurück, auch im Bereich **Konfiguration** der Verwaltung.
Wer den Bereich mit unveränderter Maske speichert, behält den gespeicherten
Wert. Das Ändern eines anderen Feldes löscht also nie ein Geheimnis.

## Backend: die zwei Dienste

Der Backend-Prozess und der Server kommunizieren per gRPC über zwei Unix-Sockets,
deren Pfade in der Umgebung ankommen:

```
EVEY_PLUGIN_ID=<plugin id>
EVEY_PLUGIN_SOCK=<path>   # Ihr Prozess lauscht hier und bedient PluginService
EVEY_HOST_SOCK=<path>     # Ihr Prozess verbindet sich hierhin, um HostService aufzurufen
```

Der Server erkennt den Prozess an seinem Socket über die PID, die der Kernel
meldet (`SO_PEERCRED`); der Prozess bekommt keinen Token. Die Aufrufe sind
auf 600 pro Minute und Plugin begrenzt. `proto/plugin/v1/plugin_api.proto`
ist die Schnittstellendefinition. Stubs für die mitgelieferten Sprachen werden beim Start des
Plugins neben den Einstiegspunkt kopiert
([Voraussetzungen](dev-setup.md#1-prerequisites)).

### PluginService: was der Server aufruft

| Methode | Wann |
|---|---|
| `Initialize(plugin_id, config, host_version)` | Sobald der Prozess seinen Socket gebunden hat. `config` enthält die Werte aus `config_schema`, die der Administrator gesetzt hat. Antworten Sie `success: false` mit einem `error`, um den Start abzulehnen. |
| `HealthCheck()` | Regelmäßig, um einen lebenden Prozess von einem hängenden zu unterscheiden. Antworten Sie `healthy: true`, solange Sie arbeiten können. |
| `Shutdown(reason)` | Wenn das Plugin deaktiviert, aktualisiert oder deinstalliert wird oder der Server stoppt. Beenden Sie sich kurz nach der Antwort. |
| `OnEvent(event_type, payload, source)` | In der Schnittstelle definiert, aber der Server liefert Plugins heute keine Ereignisse: Setzen Sie es so um, dass es `handled: true` antwortet, und rechnen Sie mit keinem Aufruf. |
| `GetCapabilities()` | In der Schnittstelle definiert; der Server ruft es heute nicht auf. |

### HostService: was das Plugin aufrufen kann

Ein Aufruf, dessen Berechtigung das Manifest nicht angibt, wird mit
`PermissionDenied` abgelehnt. Methoden mit dem Vermerk *nicht bedient*
antworten `Unimplemented`. Sie gehören zur Schnittstelle für eine spätere Version
und stehen hier, damit ein Stub Sie nicht in die Irre führt.

| Methode | Berechtigung | Hinweise |
|---|---|---|
| `Log(level, message, fields)` | keine | Ins Protokoll des Plugins geschrieben, sichtbar unter **Protokolle** auf seiner Karte. |
| `GetConfig(key)` / `SetConfig(key, value)` | keine | Die Konfiguration des Plugins: die Werte aus dem Bereich **Konfiguration** plus Schlüssel, die das Plugin selbst schreibt. Höchstens 256 Schlüssel, Schlüssel bis 256 Byte, Werte bis 16 KiB. |
| `GetData`, `SetData`, `DeleteData`, `ListDataKeys` | keine | Der [Datenspeicher](#data-store) unten. |
| `EmitEvent(event_type, payload)` | keine | Auf dem Ereignisbus des Servers als `plugin.event.<event_type>` veröffentlicht, wo Webhooks es abonnieren können. |
| `GetMediaItem(id)`, `ListMediaItems(library_id, type, parent_id, limit, offset)` | `api.library` oder `media.read` | Der vollständige Datensatz des Mediums, mit `file_path`, `provider_ids` und `genres`. |
| `Fetch(url, method, headers)` | `network` | Die einzige Verbindung aus der Sandbox nach außen: `http` oder `https`, `GET` oder `HEAD`, ein Host aus `external_fetch`, private Adressen nach der Auflösung und bei Weiterleitungen abgelehnt. Der Inhalt ist auf 8 MiB begrenzt (`truncated` zeigt es an), die ganze Anfrage auf 20 Sekunden. Der Server hängt keine eigenen Zugangsdaten an. |
| `SearchMedia`, `GetMediaStreams`, `GetChapters`, `GetSubtitles` | `api.library` oder `media.read` | *Nicht bedient.* |
| `UpdateMetadata` | `api.metadata` oder `media.write` | *Nicht bedient.* |
| `GetUserPreferences` | `api.user` | *Nicht bedient.* |
| `SendNotification` | `notification.sender` | *Nicht bedient.* |

### Datenspeicher

Jedes Plugin hat einen dauerhaften Schlüssel-Wert-Speicher, ohne dass es eine
Berechtigung angeben muss: Cursor, Caches, der Zeitpunkt der letzten
Synchronisierung, alles, was einen Neustart überdauern soll. Werte sind
opake Zeichenketten; JSON kodieren Sie selbst. Die ID des Plugins
kommt vom authentifizierten Socket. Ein Plugin liest und schreibt also nur
seinen eigenen Namensraum, und der ganze Speicher wird mit dem Plugin
gelöscht.

| Methode | Zweck |
|---|---|
| `SetData(key, value)` | Einen Wert anlegen oder ersetzen. |
| `GetData(key)` | Liefert `value` und `found`. |
| `DeleteData(key)` | Einen Schlüssel entfernen. |
| `ListDataKeys()` | Die Schlüssel des Plugins. |

Dieser Speicher gilt für das ganze Plugin. Daten **pro Benutzer** (eine
Einstellung der Person, die das Widget ansieht) liegen auf der Seite der
Oberfläche, über die Berechtigung `prefs` unten.

## Frontend-API

Ein Oberflächenmodul läuft in einem Frame mit opakem Ursprung (opaque origin),
unter der Richtlinie `default-src 'none'`: kein Zugriff auf die Hostseite,
ihre Cookies oder ihren Speicher, kein Formular, kein Popup, und ein
`connect-src`, das auf die Hosts aus `external_fetch` über `https` begrenzt
ist. Alles andere läuft über `window.evey`, das vor dem Modul eingefügt
wird. Dasselbe Modul läuft unverändert in der Web-App und in den Apps für
Handy und Fernseher.

### `evey.onInit(callback)`

Wird einmal mit dem Kontext aufgerufen, den der Host schickt. Ein Callback,
der nach dem Eintreffen des Kontexts registriert wird, wird sofort
aufgerufen.

```js
evey.onInit(function (data) {
  // data.context         - die Konfigurationswerte des Plugins (config_schema)
  // data.locale          - die aktive Sprache, z. B. "en-US"
  // data.theme           - die ID des aktiven Designs
  // data.surface         - "web", "mobile" oder "tv"
  // data.suggestedHeight - nur home.section: die Höhe (px), die der Host empfiehlt
  // data.viewportHeight  - nur tv: die Höhe des Bereichs in dp
});
```

### `evey.navigate(path)`

Führt die Host-Anwendung zu `path`. Nur die Präfixe `/home`, `/library`,
`/media`, `/player`, `/settings` und `/search` werden befolgt. Alles andere
(der Verwaltungsbereich, die Anmeldung, eine absolute URL) wird
stillschweigend verworfen.

### `evey.toast(level, message)`

Zeigt eine kurze Meldung (Toast) in der Oberfläche des Hosts. `level` ist
`"success"`, `"error"` oder `"info"`; `message` ist reiner Text, nach 200
Zeichen abgeschnitten; höchstens 3 Meldungen pro Sekunde werden angezeigt.

### `evey.resize(height)`

Verlangt, dass der Slot `height` Pixel hoch ist, begrenzt auf 50 bis 800.
`0` klappt den Slot zu: Ein Widget, das nichts zu zeigen hat, nimmt keinen
Platz ein. Ein Widget in `home.section` öffnet sich ohne Zutun mit
`data.suggestedHeight`: Der Host kennt die Oberfläche, in der Ihr Widget
angezeigt wird, und empfiehlt die Höhe, die ein Startseiten-Widget dort hat.
So erhalten alle Widgets dieselbe Größe. Ändern Sie die Größe nur, wenn
Sie einen guten Grund dafür haben.

### `evey.prefs.*` (Berechtigung `prefs`)

Die Einstellungen des aktuellen Benutzers im eigenen Namensraum des
Plugins, an (dieses Plugin, diesen Benutzer) gebunden durch den Token, den
der Host für den Frame verwaltet. Jeder Aufruf gibt ein Promise zurück; fehlt die
Berechtigung, wird es abgelehnt.

```js
await evey.prefs.set("units", "celsius");
const { value, found } = await evey.prefs.get("units");
const all = await evey.prefs.list();        // [{ key, value }]
await evey.prefs.delete("units");
```

### `evey.media.*` (Berechtigung `public`)

Eine sichere Projektion der Medien: ID, Titel, Typ, Jahr, Inhaltsangabe,
URL des Posters. Nie ein Dateipfad, eine Größe, eine Anbieter-ID oder ein
Stream.

```js
const item  = await evey.media.get(id);
const page  = await evey.media.list({ libraryId });
const data  = await evey.media.poster(id);  // das Poster als data:-URL
```

`poster` liefert eine `data:`-URL, weil die Richtlinie des Frames verbietet,
ein Bild direkt vom Server zu laden. Nutzen Sie sie als Quelle eines `<img>`
oder als Textur.

### `evey.assets.read(path)`

Liest eine der eigenen Dateien des Plugins aus `web_root`, über den
relativen Pfad, als `data:`-URL: Eine Datei, die das Backend zur Laufzeit
geschrieben hat, wird im Frame nutzbar, ohne Probleme mit fremden Ursprüngen.
Nur die eigenen Dateien des Plugins sind erreichbar.

### Auf einem Fernseher

Die TV-App führt das Modul in einer WebView aus, und einiges, das im Browser
gilt, gilt dort nicht mehr:

- **Richten Sie die Größe nicht nach Ihrem eigenen Viewport.** In der
  WebView ist `window.innerHeight` der Slot, den der Host Ihnen gerade
  zugeteilt hat. Richten Sie sich nach `data.viewportHeight` oder
  `data.suggestedHeight`.
- **Kein Fokus, kein Zeiger.** Das Widget bekommt nie den Fokus des
  Steuerkreuzes, und es gibt kein Hover. Alles Interaktive gehört auf die
  Web- und Handy-Oberflächen.
- **Halten Sie Text von den Rändern fern.** Fernseher schneiden den Rand ab
  (Overscan): Lassen Sie an jeder Seite etwa 5 % der Breite und Höhe frei.
- **Testen Sie WebGL auf einer echten Box.** Ein Shader läuft auf der GPU
  und dem Treiber der Box, nicht auf denen Ihres Rechners; halten Sie
  Rauschkoordinaten begrenzt. `tests/plugins/weather-v2` ist das
  Referenz-Widget für all das.

## Slots

| Slot | Wo er erscheint |
|---|---|
| `home.section` | Ein Inhaltsbereich auf der Startseite |
| `home.hero` | Der Bereich des großen Banners auf der Startseite |
| `library.sidebar` | Ein Seitenleistenbereich in den Bibliotheksansichten |
| `media.detail.sidebar` | Ein Seitenleistenbereich auf der Detailseite eines Mediums |
| `player.overlay` | Eine Ebene über dem Videoplayer |
| `settings.user.appearance` | Der Bereich Darstellung in den Benutzereinstellungen |
| `sidebar.bottom` | Der untere Teil der Hauptnavigation |
| `admin.nav` | Ein Eintrag im Verwaltungsmenü, der die eigene Seite des Plugins öffnet |

`admin.nav` ist der einzige Slot im Verwaltungsbereich. Er fügt nichts in
bestehende Verwaltungsbildschirme ein: Er ergänzt einen Menüeintrag, der die
Seite des Plugins im selben Sandbox-Frame öffnet, auf einer Route nur für
Administratoren. Wer ihn verlangt, erscheint damit in der Übersicht der
Änderungen, die bestätigt werden müssen. Jeder andere Slot `admin.*`,
`login.*`, `setup.*` und `profile.security.*` wird bei der Installation
abgelehnt.

## Capabilities

Eine Capability gibt die Rolle an, die das Plugin nach eigener Angabe erfüllt:
`metadata.provider`, `media.read`, `media.write`, `webhook.receiver` oder
`notification.sender`. Der Installer prüft den Namen, der Administrator
sieht die Liste in der Vorschau und jede Ergänzung beim Update, und das
Sicherheits-Dashboard hält sie fest. Der Server leitet heute nichts anhand
einer Capability weiter: Ein `metadata.provider` bekommt keinen Aufruf zur
Anreicherung, ein `webhook.receiver` keine eingehende Anfrage. Was ein
Plugin wirklich tun kann, entscheiden seine **Berechtigungen**.

## Berechtigungen der Oberfläche

`public` und `prefs` werden wie die anderen unter `permissions` angegeben.
Sie gewähren dem Backend-Prozess nichts; sie schalten `evey.media.*` und
`evey.prefs.*` im Frame frei. Der Frame erhält nie einen Token: Der Host stellt
einen Token für fünf Minuten aus, begrenzt auf (Plugin, Benutzer), und
leitet die Aufrufe weiter. Der Server lehnt standardmäßig jede Methode ab,
die die Bereiche des Tokens nicht nennen
([ADR-0012](../adr/0012-plugin-token-deny-by-default.md)).

!!! note "Was ein Plugin nie verlangen kann"
    `admin.*`, `auth.*`, `system.*`, `user.write` und `library.write` sind
    fest gesperrt: Ein Manifest, das sie nennt, wird abgelehnt, ein Token
    kann sie nie tragen, und Verwaltungsmethoden lehnen einen Plugin-Token
    ab, egal welche Bereiche er hat.

## Beispiele

`examples/plugins/` enthält funktionierende Plugins im aktuellen
Manifestformat: Vorlagen in Node.js und Python (`nodejs-example`,
`python-example`), einen Metadatenanbieter (`metadata-example`), einen
Webhook-Empfänger (`webhook-receiver`), ein Startseiten-Widget mit
Build-Schritt (`weather`), den Datenspeicher mit den Berechtigungen `public`
und `prefs` und dem Slot `admin.nav` (`data-demo`), einen Rundgang durch
alle Feldtypen von `config_schema` (`config-demo`) und zwei Themes
(`theme-dark-blue`, `open-workshop`). `make plugin-zips` packt sie alle.

<!-- sources
manifest fields, validation: internal/domain/plugin/model.go - PluginManifest, Validate, validateWebRoot, validateSrcUnderWebRoot, IsSecretConfigField ; internal/adapters/driven/pluginrunner/manifest.go - Parse, validateManifest, validateExternalFetch ; capabilities.go - KnownCapabilities, KnownPermissions
runtimes: internal/adapters/driven/pluginrunner/runtime_availability.go ; interpreters: runner_start.go - knownInterpreters
config field types: frontend/apps/web/src/features/admin/PluginCard.tsx ; examples/plugins/config-demo/plugin.yaml
secrets masked: internal/adapters/driving/grpcapi/admin_plugin.go (SecretMask)
env vars, Initialize call: internal/adapters/driven/pluginrunner/runner_start.go ; ipc_client.go - Initialize, Shutdown, HealthCheck, OnEvent ; runner_control.go - SendEvent (no caller)
peer auth, rate limit: internal/adapters/driving/pluginhost/host_server.go - makePluginAuthInterceptor, pluginRPCRatePerMin ; peer_creds.go
host RPCs and permissions: internal/adapters/driving/grpcapi/plugin_interceptor.go ; pluginhost/host_server.go - Log, GetConfig, SetConfig (maxConfigKeys, maxConfigKeyLen, maxConfigValueLen), EmitEvent (pluginEmittedEvent.EventName), GetMediaItem, ListMediaItems, mediaItemToProto, UpdateMetadata, GetUserPreferences (Unimplemented), UnimplementedHostServiceServer ; fetch.go - Fetch, maxFetchBytes, fetchTimeout
proto: proto/plugin/v1/plugin_api.proto - PluginService, HostService, InitRequest, FetchRequest, FetchResponse, MediaItem
frame policy: internal/adapters/driving/http/plugin_frame_handler.go - pluginFramePolicy ; frontend/packages/client/src/plugins/hostProtocol.ts - buildConnectSrc, ALLOWED_NAV_PREFIXES, SLOT_MAX_HEIGHT, clampSlotHeight, TOAST_MAX, TOAST_WINDOW_MS, TOAST_MAX_LENGTH ; web/PluginIframe.tsx - MIN_SLOT_HEIGHT, suggestedSlotHeight ; native/PluginWebView.tsx (surface)
bridge: frontend/packages/client/src/plugins/bridgeScript.ts - EveyBridge ; pluginRpc.ts - pluginRequestFor
slots: internal/domain/plugin/slots.go - AllowedSlots
token, deny by default: internal/app/plugin/token_service.go - PluginTokenTTL ; internal/pkg/auth/plugin_jwt.go - AllowedPluginScopes, forbiddenPrefix ; docs/adr/0003-plugin-scoped-tokens.md ; docs/adr/0012-plugin-token-deny-by-default.md
examples: examples/plugins/* ; Makefile - plugin-zips
-->
