---
source_sha: bb7b744c4f03cee0ce9a6cdfdd4b96cd6e99f6d71eee21c50f601fc4595ae619
reviewed: false
---
# Plugin-API-Referenz

Dieses Dokument richtet sich an externe Entwickler, die ein Plugin für Evey schreiben. Für die Sicherheitsvorgaben, die für Plugins gelten, siehe [`security-model.md`](security-model.md).

## Manifest-Schema

Jedes Plugin-ZIP muss in seinem Wurzelverzeichnis eine `plugin.yaml` enthalten.

```yaml
name: my-plugin              # erforderlich; lesbarer Name
version: 1.2.3               # erforderlich; semver
kind: plugin                 # erforderlich; muss "plugin" sein ("theme" für reine CSS/JS-Themes)
language: nodejs             # erforderlich für Plugins; nodejs | python | go | php | ruby | lua | bash | sh
entrypoint: node index.js    # erforderlich für Plugins; die vollständige Befehlszeile, die der Interpreter ausführt -
                              # das erste Token ist der Interpreter, im PATH gesucht; die
                              # übrigen Tokens sind Skript/Argumente, auf das ZIP beschränkt.
                              # "language" dient nur dazu, die Verfügbarkeit der Laufzeit zu prüfen und
                              # SDK-Stubs zu kopieren - es startet den entrypoint NICHT.

description: "..."           # optional
author: "..."                # optional
min_evey_version: "1.0.0"   # optional; semver-Mindestversion des Hosts

permissions:
  - network                  # siehe Abschnitt Berechtigungen
  - media.read

external_fetch:              # erforderlich, wenn die Berechtigung "network" angegeben ist
  - api.themoviedb.org       # siehe Regeln für externe Abrufe in security-model.md
  - api.example.com          # nur reiner Hostname - kein Schema, kein Platzhalter, keine IP

capabilities:
  - metadata.provider        # siehe Abschnitt Capabilities

# Optional: ein Unterverzeichnis, dessen Inhalt als statische Dateien ausgeliefert wird.
# Erforderlich, wenn Frontend-Module angegeben sind.
web_root: public/

# Optional: Frontend-Module, die in die Host-Oberfläche eingefügt werden.
frontend:
  - slot: home.section       # siehe Slot-Liste in security-model.md
    src: public/front.js     # Pfad relativ zur ZIP-Wurzel; muss unter web_root liegen

config_schema:               # optional; JSON-Schema-Objekt, das die Konfigurationsschlüssel beschreibt
  api_key:
    type: string
    secret: true             # oder `type: password`: nie an einen Client zurückgesendet, Admin eingeschlossen
    description: "TMDB API key"
```

Ein mit `secret: true` markiertes Feld (oder eines vom `type: password`) wird
gespeichert und dem Plugin wie üblich übergeben, aber jede Leseroute - die
öffentliche und das Konfigurationspanel des Admin-Bereichs - liefert es maskiert.
Der Admin-Bereich behält den gespeicherten Wert, wenn die Maske unverändert
zurückgeschickt wird, sodass das Speichern der anderen Felder nie ein Geheimnis
löscht.

**Bei der Installation geprüfte Regeln:**

- `kind: plugin` verlangt `language` und `entrypoint`.
- `kind: theme` darf `language`, `entrypoint` oder `permissions` NICHT angeben.
- `web_root` ist erforderlich, wenn `frontend` nicht leer ist.
- `frontend[].src` muss unter `web_root` liegen.
- `frontend[].slot` muss in der serverseitigen Slot-Liste stehen.
- Die Berechtigung `network` verlangt eine nicht leere Liste `external_fetch`.
- Alle Einträge in `external_fetch` müssen den Host-Validator bestehen.
- Alle Capabilities und Berechtigungen müssen dem Server bekannt sein.

## Kommunikation mit dem Server

Der Plugin-Prozess und der Evey-Host sprechen gRPC über zwei Unix-Domain-Sockets,
deren Pfade beim Start in der Umgebung übergeben werden:

```
EVEY_PLUGIN_ID=<plugin_id>
EVEY_PLUGIN_SOCK=/tmp/evey/plugins/<plugin_id>/plugin.sock   # Ihr Prozess lauscht hier und stellt PluginService bereit
EVEY_HOST_SOCK=/tmp/evey/plugins/<plugin_id>/host.sock       # Ihr Prozess verbindet sich hier, um HostService aufzurufen
```

Der Host authentifiziert den Plugin-Prozess an seinem Socket über die PID, die
der Kernel meldet (`SO_PEERCRED`) - dem Prozess selbst wird kein Token ausgestellt.

Ein separates, kurzlebiges JWT (5 Minuten, auf `plugin_id` + `user_id` beschränkt) wird bei Bedarf über den RPC `IssuePluginToken` ausgestellt - der *Frontend-iframe* des Plugins holt eines, um seine eigenen Aufrufe mit den Bereichen `public`/`prefs` zu authentifizieren. Es hat nichts mit dem Backend-Socket oben zu tun und wird dem Plugin-Prozess nie beim Start übergeben.

Die gRPC-Dienstdefinitionen stehen in `proto/plugin/v1/plugin_api.proto`. Sprachspezifische SDK-Stubs werden aus dieser Proto-Datei erzeugt und können ins Plugin-ZIP aufgenommen oder daneben mitgeliefert werden. Für mehrere Sprachen werden Stubs vorgeneriert mitgeliefert (`sdk/<lang>/`); für eine nicht mitgelieferte siehe [dev-setup.md → Stubs für eine andere Sprache erzeugen](dev-setup.md#generating-stubs-for-another-language).

Der Host setzt Berechtigungen auf RPC-Ebene durch - ein RPC aufzurufen, den Ihr Manifest nicht angegeben hat, ergibt `PermissionDenied`.

### Datenspeicher

Jedes Plugin erhält einen eigenen dauerhaften Schlüssel-Wert-Speicher im Namensraum
des Plugins - **keine Berechtigung nötig, ein Plugin besitzt seine eigenen
Daten**. Nutzen Sie ihn für Cursor, Caches, Zeitpunkte der letzten
Synchronisation, alles, was sich das Plugin über Neustarts hinweg merken muss.
Werte sind undurchsichtige Zeichenketten (kodieren Sie JSON selbst, wenn Sie
Struktur brauchen).

| RPC `HostService` | Zweck |
|---|---|
| `SetData(key, value)` | Einen Wert anlegen oder ersetzen. |
| `GetData(key) → {value, found}` | Einen Wert lesen. |
| `DeleteData(key)` | Einen Schlüssel entfernen. |
| `ListDataKeys() → {keys}` | Die Schlüssel des Plugins auflisten. |

Die Plugin-ID stammt aus dem authentifizierten Socket - ein Plugin kann nur
seinen **eigenen** Namensraum lesen und schreiben, nie den eines anderen Plugins.
Bei der Deinstallation wird sein ganzer Speicher automatisch gelöscht
(`ON DELETE CASCADE`), sodass keine veralteten Daten die Entfernung überdauern.

Dieser Speicher gilt für das ganze Plugin. Um Daten **pro Benutzer** zu speichern
(z. B. eine Vorliebe des gerade schauenden Benutzers), siehe die Berechtigung
`prefs` unten - diese Schnittstelle wird aus dem iframe des Plugins erreicht, wo
ein Benutzerkontext existiert.

## Frontend-API

Gibt ein Plugin ein Modul `frontend` an, enthält das HTML des iframe-`srcdoc` ein Brückenskript, das dem JavaScript des Moduls `window.evey` bereitstellt.

Das Frontend-Modul läuft in einem abgeschotteten Rahmen mit opakem Ursprung: Es hat keinen Zugriff auf DOM, Cookies oder Speicher der Host-Seite und erreicht Evey nur über `window.evey`. Sein eigener Netzwerkzugang ist durch das `connect-src` des Rahmens auf die Hosts seiner Liste `external_fetch` beschränkt, über `https://`; ein Plugin ohne diese Liste kann gar keine Verbindung öffnen.

### `evey.navigate(path)`

Lässt die Host-Anwendung zu `path` navigieren. Der Pfad muss mit `/home`, `/library`, `/media`, `/player`, `/settings` oder `/search` beginnen; alles andere (der Admin-Bereich, die Anmeldung, die Einrichtung, eine absolute oder protokollrelative URL) wird stillschweigend verworfen.

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

Bittet den Host, den iframe-Slot auf `height` Pixel zu verändern. Begrenzt auf 50-800. `0` klappt den Slot zusammen: Ein Plugin, das nichts zu zeigen hat, nimmt auf der Seite keinen Platz ein.

```js
evey.resize(300);
evey.resize(0); // nichts zu sagen: der Slot verschwindet
```

### `evey.onInit(callback)`

Registriert einen Callback, der ausgelöst wird, sobald der Host dem Plugin seinen Initialisierungskontext sendet. Hat der Host `init` schon vor dem Aufruf von `onInit` gesendet, wird der Callback sofort mit den zwischengespeicherten Daten aufgerufen.

```js
evey.onInit(function(data) {
  // data.context         - Konfigurationswerte des Plugins (aus config_schema)
  // data.locale          - aktive Sprache, z. B. "en-US"
  // data.theme           - ID des aktiven Themes
  // data.surface         - "web", "mobile" oder "tv": wo das Widget gelandet ist
  // data.suggestedHeight - nur home.section: die Höhe (px), die der Host dort empfiehlt
  // data.viewportHeight  - nur tv: die Höhe des Panels in dp
});
```

Der Host wartet bis zu 5 Sekunden darauf, dass das Plugin `ready` signalisiert (erledigt das Brückenskript automatisch). Wird die Zeit überschritten, wird der iframe entfernt und ein Fehler gemeldet.

#### Höhe

Ein Widget `home.section` öffnet sich ohne Zutun mit `data.suggestedHeight`: Der Host kennt die Fläche, auf die er Sie gesetzt hat - eine Desktop-Seite, ein Telefon, ein Fernsehpanel - und empfiehlt die Höhe, die ein Widget der Startseite dort hat, sodass jedes Widget einer Startseite gleich groß ankommt. Rufen Sie `evey.resize()` nur auf, wenn Sie es besser wissen; der Host verändert oder skaliert Ihren Inhalt nie hinter Ihrem Rücken.

#### Auf einem Fernseher

Die Fernseh-App führt Ihr Widget in einer WebView aus, und einige Dinge, die im Browser stimmen, stimmen dort nicht mehr:

- **Nicht aus dem eigenen Viewport bemessen.** In der WebView ist `window.innerHeight` der Slot, den der Host Ihnen gerade gewährt hat - ein Zirkelschluss. Wenn Sie Ihre Größe selbst ändern, bemessen Sie sie aus `data.viewportHeight` (das Panel) oder `data.suggestedHeight`, nie aus `innerHeight`.
- **Kein Fokus, kein Zeiger.** Das Widget erhält nie den D-Pad-Fokus, und es gibt kein Hover. Alles Interaktive gehört auf die Flächen Web und Mobil.
- **Text von den Rändern fernhalten.** Fernseher haben Overscan: Lassen Sie auf jeder Seite etwa 5 % von Breite und Höhe frei.
- **WebGL auf einer echten Box testen, vergrößert.** Ein Shader läuft auf der GPU der Box und ihrem Treiber, nicht auf der Ihres Desktops. Ein Gitter-Hash, der seine Eingabe vor `fract()` mit einigen Hundert multiplizierte, zeichnete auf einer Mali-GPU flache Polygone statt Wolken, während er im Browser gut aussah; halten Sie Rauschkoordinaten begrenzt (`mod()`) und hashen Sie begrenzte Werte. `tests/plugins/weather-v2` ist das Referenz-Widget für all das.

## Slots

| Slot | Wo er erscheint |
|---|---|
| `home.section` | Inhaltsabschnitt auf der Startseite |
| `home.hero` | Bannerbereich auf der Startseite |
| `library.sidebar` | Seitenpanel in Bibliotheksansichten |
| `media.detail.sidebar` | Seitenpanel auf Detailseiten von Medien |
| `player.overlay` | Überlagerungsebene im Videoplayer |
| `settings.user.appearance` | Unterabschnitt Darstellung in den Benutzereinstellungen |
| `sidebar.bottom` | Unterer Bereich der Hauptnavigation |
| `admin.nav` | Ein Navigationseintrag im Admin-Bereich, der die eigene abgeschottete Seite des Plugins öffnet |

`admin.nav` ist der **einzige** Slot im Admin-Bereich. Er schleust keine Oberfläche
in bestehende Admin-Bildschirme ein - er fügt einen einzigen Menüeintrag hinzu, der
die eigene Seite Ihres Plugins in einem abgeschotteten iframe auf einer eigenen
Admin-Route öffnet. Ihn zu verlangen, erscheint in der Zustimmungsübersicht der
Installation.

Alle anderen Slots unter `admin.*`, `login.*`, `setup.*`, `profile.security.*`
werden bei der Installation abgelehnt.

## Capabilities

Capabilities geben an, welche serverseitige Rolle das Plugin erfüllt. Sie bestimmen, welche RPCs der Host an das Plugin weiterleitet und wie er das Plugin in seinem Dienstverzeichnis registriert.

### `metadata.provider`

Das Plugin kann angereicherte Metadaten für Medien liefern. Es gibt keinen eigenen RPC für die Anreicherung - der `PluginService`, den der Host aufruft, bietet nur `Initialize` / `Shutdown` / `OnEvent` / `HealthCheck` / `GetCapabilities` (`proto/plugin/v1/plugin_api.proto`). Ein Plugin `metadata.provider` erhält die passenden Ereignisse über `OnEvent` und meldet seine Capability über `GetCapabilities`; es implementiert keine eigene Anreicherungsmethode.

Erforderliche Berechtigung: `api.metadata`.

### `media.read`

Das Plugin darf Medieneinträge und Streams lesen. Entspricht der Berechtigung `media.read` im Manifest.

### `media.write`

Das Plugin darf Medieneinträge anlegen oder ändern. Entspricht der Berechtigung `media.write`.

### `notification.sender`

Das Plugin darf Benachrichtigungen veröffentlichen. Der Host leitet ausgehende Benachrichtigungsereignisse durch das Plugin. Entspricht der Berechtigung `notification.sender`.

### `webhook.receiver`

Das Plugin registriert sich als Webhook-Endpunkt. Der Host leitet passende eingehende HTTP-POST-Anfragen per gRPC an das Plugin weiter. Außer der Angabe der Capability ist keine weitere Berechtigung nötig.

## Berechtigungen

`public` und `prefs` sind **Berechtigungen**, keine Capabilities - sie ändern
nicht, welche serverseitige Rolle das Plugin erfüllt, sie gewähren dem iframe
des Plugins zusätzliche Lese- und Schreibbereiche. Geben Sie sie unter
`permissions` im Manifest an.

### `public`

Gewährt dem iframe des Plugins Lesezugriff auf nicht sensible Serverdaten:
`GetPublicMediaItem` / `ListPublicMediaItems`, die eine **sichere Projektion**
liefern (ID, Titel, Typ, Jahr, Zusammenfassung, Poster-URL) - nie Dateipfade,
Größen, Anbieter-IDs oder Streams. Das Branding des Servers (Name) ist bereits
öffentlich und braucht keine Berechtigung. Geben Sie `public` in `permissions`
an, um es zu verlangen.

### `prefs`

Gewährt dem iframe des Plugins Lese- und Schreibzugriff auf die Vorlieben des
**aktuellen Benutzers** im eigenen Namensraum des Plugins, über
`PluginUserDataService` (`GetPref` / `SetPref` / `DeletePref` / `ListPrefs`).
Plugin und Benutzer sind durch das iframe-Token gebunden, sodass ein Plugin nur
Daten (eigenes Plugin, aktueller Benutzer) anfassen kann - nie die eines anderen
Plugins, nie die eines anderen Benutzers, nie Identitäts- oder Sicherheitsfelder.
Die Zeilen werden mit dem Plugin oder dem Benutzer kaskadierend gelöscht. Geben
Sie `prefs` in `permissions` an.

> **Was Sie nie verlangen können.** `admin.*`, `auth.*` (Anmeldung/Abmeldung/Sitzung),
> `system.*` und allgemeines `user.write` / `library.write` sind fest gesperrt: Sie
> werden aus jedem Token entfernt und an der API-Grenze abgelehnt, selbst wenn ein
> Manifest sie anzugeben versucht. Ein Plugin-Token mit Bereichen ist auf der
> Server-API **standardmäßig verweigert** - es erreicht nur die Methoden, die die
> gewährten Berechtigungen oben ausdrücklich freischalten.

## Beispiele

`examples/plugins/` enthält funktionierende Plugins im aktuellen Manifest-Format: Vorlagen in Node.js und Python (`nodejs-example`, `python-example`), ein Metadatenanbieter (`metadata-example`), ein Webhook-Empfänger (`webhook-receiver`), ein Widget für die Startseite (`weather`), Plugin-Konfiguration und Geheimnisse (`config-demo`), der Datenspeicher mit den Berechtigungen `public` / `prefs` und dem Slot `admin.nav` (`data-demo`) sowie zwei Themes. [dev-setup.md](dev-setup.md#3-scaffold-your-first-plugin) führt durch das Kopieren eines davon.
