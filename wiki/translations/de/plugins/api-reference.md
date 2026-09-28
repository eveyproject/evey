---
source_sha: 2d10a959b8d809ae197a485f1214dbb6d86d37c8658c1c4d4c12d0222f6593f8
---
# API-Referenz für Erweiterungen

Dieses Dokument richtet sich an externe Entwickler, die eine Evey-Erweiterung schreiben. Die Sicherheitsvorgaben, die für Erweiterungen durchgesetzt werden, stehen in [`security-model.md`](security-model.md).

## Manifest-Schema

Jede ZIP-Datei einer Erweiterung muss im Wurzelverzeichnis eine `plugin.yaml` enthalten.

```yaml
name: my-plugin              # required; human-readable name
version: 1.2.3               # required; semver
kind: plugin                 # required; must be "plugin" (use "theme" for CSS/JS-only themes)
language: nodejs             # required for plugins; nodejs | python | go | php | ruby | lua | bash | sh
entrypoint: node index.js    # required for plugins; the full command line the interpreter runs -
                              # the first token is the interpreter, looked up on PATH; the
                              # remaining tokens are the script/args, path-confined to the ZIP.
                              # "language" is only used to check the runtime is available and to
                              # copy SDK stubs - it does NOT launch the entrypoint.

description: "..."           # optional
author: "..."                # optional
min_evey_version: "1.0.0"   # optional; semver minimum host version

permissions:
  - network                  # see Permissions section
  - media.read

external_fetch:              # required when "network" permission is declared
  - api.themoviedb.org       # see External fetch rules in security-model.md
  - api.example.com          # bare hostname only - no scheme, no wildcard, no literal IP

capabilities:
  - metadata.provider        # see Capabilities section

# Optional: declare a sub-directory whose contents are served as static assets.
# Required when frontend modules are declared.
web_root: public/

# Optional: frontend modules injected into the host UI.
frontend:
  - slot: home.section       # see Slot whitelist in security-model.md
    src: public/front.js     # path relative to ZIP root; must be under web_root

config_schema:               # optional; JSON Schema object describing plugin config keys
  api_key:
    type: string
    secret: true             # or `type: password`: never sent back to any client, admin included
    description: "TMDB API key"
```

Ein mit `secret: true` markiertes Feld (oder eines vom `type: password`) wird wie
üblich gespeichert und an die Erweiterung übergeben, aber jede lesende Route - die
öffentliche wie der Konfigurationsbereich der Administration - gibt es maskiert
zurück. Der Administrationsbereich behält den gespeicherten Wert, wenn die Maske
unverändert zurückgeschickt wird, sodass das Speichern der anderen Felder nie
ein Geheimnis löscht.

**Bei der Installation durchgesetzte Prüfregeln:**

- `kind: plugin` verlangt `language` und `entrypoint`.
- `kind: theme` darf `language`, `entrypoint` und `permissions` NICHT angeben.
- `web_root` ist Pflicht, wenn `frontend` nicht leer ist.
- `frontend[].src` muss unter `web_root` liegen.
- `frontend[].slot` muss in der serverseitigen Positivliste der Slots stehen.
- Die Berechtigung `network` verlangt eine nicht leere `external_fetch`-Liste.
- Alle `external_fetch`-Einträge müssen die Prüfung der Host-Positivliste bestehen.
- Alle Fähigkeiten und Berechtigungen müssen dem Server bekannt sein.

## Backend-IPC

Der Erweiterungsprozess spricht mit dem Evey-Host über gRPC auf einem Unix-Domain-Socket. Der Pfad des Sockets wird beim Start in der Umgebungsvariable `EVEY_PLUGIN_SOCK` übergeben.

Der Host authentifiziert den Erweiterungsprozess, indem er direkt nach `cmd.Start` dessen PID über `SO_PEERCRED` prüft - dem Prozess selbst wird kein Token ausgestellt.

```
EVEY_PLUGIN_SOCK=/tmp/evey/plugins/<plugin_id>/host.sock
EVEY_PLUGIN_ID=<plugin_id>
EVEY_HOST_SOCK=<path>          # socket the plugin can dial back to reach the host
```

Ein separates, kurzlebiges JWT (5 Minuten, begrenzt auf `plugin_id` + `user_id`) wird bei Bedarf über die RPC `IssuePluginToken` ausgestellt - das *Frontend-iframe* der Erweiterung holt sich eines, um seine eigenen Aufrufe im Bereich `public`/`prefs` zu authentifizieren. Es hat nichts mit dem Backend-Socket oben zu tun und wird dem Erweiterungsprozess beim Start nie übergeben.

Die gRPC-Dienstdefinitionen stehen in `proto/plugin/v1/plugin_api.proto`. Sprachspezifische SDK-Stubs werden aus diesem Proto erzeugt und können in die ZIP-Datei der Erweiterung aufgenommen oder daneben mitgeliefert werden. Für mehrere Sprachen liegen die Stubs fertig bei (`sdk/<lang>/`); für eine nicht mitgelieferte siehe [dev-setup.md → Stubs für eine andere Sprache erzeugen](dev-setup.md#generating-stubs-for-another-language).

Der Host setzt Berechtigungen auf Ebene der RPCs durch - der Aufruf einer RPC, die Ihr Manifest nicht angegeben hat, endet mit `PermissionDenied`.

### Datenspeicher

Jede Erweiterung bekommt ihren eigenen dauerhaften Schlüssel-Wert-Speicher, im
Namensraum der Erweiterung - **keine Berechtigung nötig, eine Erweiterung besitzt
ihre eigenen Daten**. Nutzen Sie ihn für Cursor, Caches, Zeitpunkte der letzten
Synchronisation, alles, was die Erweiterung über Neustarts hinweg wissen muss.
Werte sind undurchsichtige Zeichenketten (kodieren Sie JSON selbst, wenn Sie
Struktur brauchen).

| `HostService`-RPC | Zweck |
|---|---|
| `SetData(key, value)` | Einen Wert anlegen oder ersetzen. |
| `GetData(key) → {value, found}` | Einen Wert lesen. |
| `DeleteData(key)` | Einen Schlüssel entfernen. |
| `ListDataKeys() → {keys}` | Die Schlüssel der Erweiterung auflisten. |

Die ID der Erweiterung wird dem authentifizierten Socket entnommen - eine
Erweiterung kann nur ihren **eigenen** Namensraum lesen und beschreiben, nie den
einer anderen. Wird die Erweiterung deinstalliert, wird ihr ganzer Speicher
automatisch gelöscht (`ON DELETE CASCADE`), sodass keine veralteten Daten die
Entfernung überleben.

Dieser Speicher gilt für die ganze Erweiterung. Um Daten **pro Benutzer** zu
speichern (etwa eine Vorliebe des gerade schauenden Benutzers), siehe die
Berechtigung `prefs` unten - diese Schnittstelle erreicht man aus dem iframe der
Erweiterung, wo ein Benutzerkontext existiert.

## Frontend-API

Deklariert eine Erweiterung ein `frontend`-Modul, enthält das HTML für den `srcdoc` des iframes ein Brücken-Skript, das dem JavaScript des Moduls `window.evey` bereitstellt.

Das Frontend-Modul hat keinen direkten Zugriff auf das DOM der Host-Seite, auf `window`, `fetch`, `localStorage` oder irgendein anderes globales Browser-Objekt über das hinaus, was ausdrücklich über `window.evey` bereitgestellt wird.

### `evey.navigate(path)`

Lässt die Host-Anwendung zu `path` navigieren. Der Pfad muss eine erlaubte Route sein; Anfragen an `/admin/*`, `/auth/*`, `/setup/*` und `/profile/security/*` werden stillschweigend verworfen.

```js
evey.navigate("/library/movies");
evey.navigate("/media/abc123");
```

### `evey.toast(level, message)`

Zeigt eine Toast-Benachrichtigung in der Host-Oberfläche.

- `level`: `"success"` | `"error"` | `"info"`
- `message`: reiner Text, höchstens 200 Zeichen, HTML wird entfernt
- Begrenzt auf 3 Toasts pro Sekunde

```js
evey.toast("success", "Metadata enriched.");
evey.toast("error", "API key missing.");
```

### `evey.resize(height)`

Bittet den Host, den iframe-Slot auf `height` Pixel zu bringen. Begrenzt auf 50-800. `0` klappt den Slot ein: Eine Erweiterung, die nichts zu zeigen hat, nimmt auf der Seite keinen Platz ein.

```js
evey.resize(300);
evey.resize(0); // nothing to say: the slot disappears
```

### `evey.onInit(callback)`

Registriert einen Callback, der einmal ausgelöst wird, sobald der Host der Erweiterung ihren Initialisierungskontext schickt. Hat der Host `init` schon gesendet, bevor `onInit` aufgerufen wird, wird der Callback sofort mit den zwischengespeicherten Daten aufgerufen.

```js
evey.onInit(function(data) {
  // data.context         - plugin config values (from config_schema)
  // data.locale          - active locale string, e.g. "en-US"
  // data.theme           - active theme id
  // data.surface         - "web", "mobile" or "tv": where the widget landed
  // data.suggestedHeight - home.section only: the height (px) the host recommends there
  // data.viewportHeight  - tv only: the panel height in dp
});
```

Der Host wartet bis zu 5 Sekunden darauf, dass die Erweiterung `ready` meldet (das erledigt das Brücken-Skript automatisch). Ist die Frist abgelaufen, wird das iframe ausgehängt und ein Fehler gemeldet.

#### Höhe

Ein `home.section`-Widget öffnet sich mit `data.suggestedHeight`, ohne dass Sie etwas tun müssen: Der Host kennt die Oberfläche, auf die er Sie gesetzt hat - eine Desktop-Seite, ein Handy, ein Fernseh-Panel - und empfiehlt die Höhe, die ein Startseiten-Widget dort hat, sodass jedes Widget auf einer Startseite gleich groß landet. Rufen Sie `evey.resize()` nur auf, wenn Sie es besser wissen; der Host ändert oder skaliert Ihren Inhalt nie hinter Ihrem Rücken.

#### Auf einem Fernseher

Die TV-App führt Ihr Widget in einer WebView aus, und einiges, was im Browser gilt, gilt dort nicht mehr:

- **Richten Sie die Größe nicht nach Ihrem eigenen Viewport.** In der WebView ist `window.innerHeight` der Slot, den der Host Ihnen gerade gewährt hat - ein Zirkelschluss. Wenn Sie sich selbst vergrößern, richten Sie sich nach `data.viewportHeight` (dem Panel) oder `data.suggestedHeight`, nie nach `innerHeight`.
- **Kein Fokus, kein Zeiger.** Das Widget bekommt nie den Fokus des Steuerkreuzes, und es gibt kein Hover. Alles Interaktive gehört auf die Web- und Mobiloberflächen.
- **Halten Sie Text von den Rändern fern.** Fernseher beschneiden den Rand (Overscan): Lassen Sie auf jeder Seite etwa 5 % der Breite und Höhe frei.
- **Testen Sie WebGL auf einem echten Gerät, vergrößert.** Ein Shader läuft auf der GPU des Geräts und ihrem Treiber, nicht auf der Ihres Desktops. Ein Gitter-Hash, der seine Eingabe vor `fract()` mit ein paar Hundert multiplizierte, zeichnete auf einer Mali-GPU flache Polygone statt Wolken, während er im Browser gut aussah; halten Sie Rauschkoordinaten begrenzt (`mod()`) und hashen Sie begrenzte Werte. `tests/plugins/weather-v2` ist das Referenz-Widget für all das.

## Slots

| Slot | Wo er erscheint |
|---|---|
| `home.section` | Inhaltsbereich auf der Startseite |
| `home.hero` | Hero-Banner auf der Startseite |
| `library.sidebar` | Seitenleiste in den Bibliotheksansichten |
| `media.detail.sidebar` | Seitenleiste auf den Detailseiten der Medien |
| `player.overlay` | Überlagerungsebene über dem Videoplayer |
| `settings.user.appearance` | Unterabschnitt Darstellung in den Benutzereinstellungen |
| `sidebar.bottom` | Unterer Bereich der Hauptnavigation |
| `admin.nav` | Ein Menüeintrag im Administrationsbereich, der die eigene abgeschottete Seite der Erweiterung öffnet |

`admin.nav` ist der **einzige** Slot im Administrationsbereich. Er fügt keine
Oberfläche in bestehende Verwaltungsbildschirme ein - er ergänzt einen einzigen
Menüeintrag, der die eigene Seite Ihrer Erweiterung in einem abgeschotteten
iframe auf einer eigenen Verwaltungsroute öffnet. Wer ihn verlangt, taucht in der
Übersicht zur Zustimmung bei der Installation auf.

Alle anderen Slots unter `admin.*`, `login.*`, `setup.*`, `profile.security.*`
werden bei der Installation abgelehnt.

## Fähigkeiten

Fähigkeiten geben an, welche serverseitige Rolle die Erweiterung erfüllt. Sie beeinflussen, welche RPCs der Host an die Erweiterung weiterleitet und wie er sie in seiner Dienstregistrierung einträgt.

### `metadata.provider`

Die Erweiterung kann angereicherte Metadaten für Medieneinträge liefern. Eine eigene RPC zur Anreicherung gibt es nicht - der `PluginService`, den der Host aufruft, bietet nur `Initialize` / `Shutdown` / `OnEvent` / `HealthCheck` / `GetCapabilities` (`proto/plugin/v1/plugin_api.proto`). Eine `metadata.provider`-Erweiterung erhält die passenden Ereignisse über `OnEvent` und meldet ihre Fähigkeit über `GetCapabilities`; sie implementiert keine eigene Anreicherungsmethode.

Nötige Berechtigung: `api.metadata`.

### `media.read`

Die Erweiterung darf Medieneinträge und Streams lesen. Entspricht der Berechtigung `media.read` im Manifest.

### `media.write`

Die Erweiterung darf Medieneinträge anlegen oder ändern. Entspricht der Berechtigung `media.write`.

### `notification.sender`

Die Erweiterung darf Benachrichtigungen veröffentlichen. Der Host leitet ausgehende Benachrichtigungsereignisse über die Erweiterung. Entspricht der Berechtigung `notification.sender`.

### `webhook.receiver`

Die Erweiterung registriert sich als Webhook-Endpunkt. Der Host leitet passende eingehende HTTP-POST-Anfragen über gRPC an die Erweiterung weiter. Über die Angabe der Fähigkeit hinaus ist keine Berechtigung nötig.

## Berechtigungen

`public` und `prefs` sind **Berechtigungen**, keine Fähigkeiten - sie ändern
nicht, welche serverseitige Rolle die Erweiterung erfüllt, sondern gewähren dem
iframe der Erweiterung zusätzliche Lese-/Schreibbereiche. Geben Sie sie unter
`permissions` im Manifest an.

### `public`

Gewährt dem iframe der Erweiterung Lesezugriff auf nicht sensible Serverdaten:
`GetPublicMediaItem` / `ListPublicMediaItems`, die eine **sichere Projektion**
liefern (ID, Titel, Typ, Jahr, Zusammenfassung, Poster-URL) - nie Dateipfade,
Größen, Anbieter-IDs oder Streams. Das Branding des Servers (Name) ist schon
öffentlich und braucht keine Berechtigung. Geben Sie `public` unter
`permissions` an, um sie zu verlangen.

### `prefs`

Gewährt dem iframe der Erweiterung Lese-/Schreibzugriff auf die Einstellungen des
**aktuellen Benutzers** im eigenen Namensraum der Erweiterung, über
`PluginUserDataService` (`GetPref` / `SetPref` / `DeletePref` / `ListPrefs`).
Erweiterung und Benutzer sind durch das iframe-Token gebunden, sodass eine
Erweiterung nur Daten (ihrer-eigenen-Erweiterung, des-aktuellen-Benutzers)
anfassen kann - nie die einer anderen Erweiterung, nie die eines anderen
Benutzers, nie Identitäts- oder Sicherheitsfelder. Die Zeilen werden mit der
Erweiterung oder dem Benutzer kaskadierend gelöscht. Geben Sie `prefs` unter
`permissions` an.

> **Was Sie nie verlangen können.** `admin.*`, `auth.*` (Anmeldung/Abmeldung/
> Sitzung), `system.*` und das allgemeine `user.write` / `library.write` sind
> hart gesperrt: Sie werden aus jedem Token entfernt und an der API-Grenze
> abgelehnt, selbst wenn ein Manifest versucht, sie anzugeben. Ein begrenztes
> Erweiterungs-Token ist auf der Server-API **standardmäßig verweigert** - es
> erreicht nur die Methoden, die die gewährten Berechtigungen oben ausdrücklich
> freischalten.

## Beispiele

`examples/plugins/` enthält funktionierende Erweiterungen im aktuellen Manifest-Format: Startpunkte in Node.js und Python (`nodejs-example`, `python-example`), eine Metadatenquelle (`metadata-example`), einen Webhook-Empfänger (`webhook-receiver`), ein Startseiten-Widget (`weather`), Konfiguration und Geheimnisse einer Erweiterung (`config-demo`), den Datenspeicher mit den Berechtigungen `public` / `prefs` und dem Slot `admin.nav` (`data-demo`) sowie zwei Themes. [dev-setup.md](dev-setup.md#3-scaffold-your-first-plugin) zeigt, wie man eines davon kopiert.
