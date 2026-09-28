---
source_sha: 8da868d1b798afe178e9ef7c621a9a0513b023c77f535745e4e5d2c44b5dae50
---
# Einrichtung für Erweiterungen

Ein vollständiger Durchgang für alle, die Evey gerade geklont haben und eine Erweiterung oder ein Theme schreiben, testen und ausliefern wollen. Wählen Sie die Laufzeit, die zur Sprache Ihrer Erweiterung passt; der Rest ist für alle gleich.

## 1. Voraussetzungen

Evey betreibt den Host-Prozess (Go) und liefert Erweiterungen als ZIP-Dateien aus. Welche Laufzeit die Erweiterung braucht, hängt davon ab, was Ihr Code aufruft.

### Immer nötig

| Werkzeug | Wozu | Installation |
|---|---|---|
| Go ≥ 1.25 | Den Evey-Host bauen | `https://go.dev/dl/` |
| Docker + Docker Compose | Standard-Entwicklungsumgebung (Postgres, Meilisearch, Server, Web) | https://docs.docker.com/engine/install/ |
| `bubblewrap` (`bwrap`) | Sandbox für die Prozesse der Erweiterungen - nötig für die Modi `bubblewrap` und `strict` | `apt install bubblewrap` (Debian/Ubuntu), `dnf install bubblewrap` (Fedora), `pacman -S bubblewrap` (Arch). macOS: Linux in einer VM installieren, bwrap gibt es nur für Linux. |

Der Sandbox-Modus `basic` läuft ohne bwrap, sollte aber nie außerhalb von lokalen Wegwerf-Experimenten benutzt werden - verlassen Sie ihn, sobald Sie können.

### Je nach Sprache

Nehmen Sie, was Ihre Erweiterung tatsächlich nutzt; Sie brauchen nicht alles.

| Sprache der Erweiterung | Werkzeuge | Hinweise |
|---|---|---|
| Python 3 | `python3`, `pip` | Nutzen Sie `pip install --user`, um den Host sauber zu halten. Die SDK-Stubs liegen in `sdk/python/`. |
| Node.js | Node 20+, `npm` oder `pnpm` | Das SDK lädt `plugin_api.proto` zur Laufzeit über `@grpc/proto-loader`. |
| Go | Go ≥ 1.21 | Keine mitgelieferten Stubs - erzeugen Sie sie mit `protoc-gen-go` + `protoc-gen-go-grpc`; ein kompiliertes Go-Binary braucht außerdem die Berechtigung `native-binary`. |
| Dart / Java / PHP / Ruby / Swift | Die Werkzeugkette der jeweiligen Sprache | Die Stubs liegen fertig erzeugt in `sdk/<lang>/`. |
| Theme nur mit CSS | Nichts weiter | Ein Theme ist eine ZIP-Datei mit `kind: theme` + einer `theme.css`. |
| Theme mit JS | Nichts weiter (der Host führt Ihr JS in einem abgeschotteten `iframe` mit `sandbox="allow-scripts"` und einer strengen CSP aus) | Der statische Scanner lehnt jeden Zugriff auf `document`, `window`, `fetch`, `localStorage` usw. ab - siehe `internal/pkg/themejs/scan.go`. |

### Stubs für eine andere Sprache erzeugen

Für die Sprachen oben liegen die Stubs fertig bei, aber der gRPC-Vertrag der
Erweiterungen ist nur eine Proto-Datei - Sie können **jede Sprache ansteuern, die
`protoc` unterstützt** (Rust, C#, Kotlin, Elixir, …). Die einzige Quelle der
Wahrheit ist:

```
proto/plugin/v1/plugin_api.proto
```

Erzeugen Sie sie mit `protoc` (oder `buf`) und richten Sie das gRPC-Plugin Ihrer
Sprache darauf aus:

```bash
# generic shape - exact --*_out flags depend on the language/plugin
protoc --proto_path=proto \
       --<lang>_out=OUT_DIR --grpc_<lang>_out=OUT_DIR \
       proto/plugin/v1/plugin_api.proto
```

Erprobte Rezepte für die mitgelieferten Sprachen (welches gRPC-Plugin jede
braucht: `grpc_php_plugin`, `protoc-gen-grpc-java`, `protoc-gen-grpc-swift`, …)
stehen im Ziel `plugin-sdk` des `Makefile` im Wurzelverzeichnis und in jeder
`sdk/<lang>/README.md` (z. B. `sdk/php/README.md`). `make plugin-sdk` erzeugt
die ausgelieferten Stubs neu.

Legen Sie die erzeugten Stubs in die ZIP-Datei Ihrer Erweiterung - der Host
kopiert Stubs nur für die Sprachen automatisch, die er mitbringt (`sdk/<lang>/`).

### Optional, aber empfohlen

| Werkzeug | Wozu |
|---|---|
| `cosign` (Sigstore) | Die ZIP-Datei Ihrer Erweiterung signieren. `EVEY_PLUGINS_COSIGN_POLICY` steht standardmäßig auf `warn` (nicht `off`) - mit `warn` und ohne konfigurierten öffentlichen Schlüssel tut die Prüfung nichts (eine Warnung wird geloggt, nichts wird blockiert); setzen Sie `EVEY_PLUGINS_COSIGN_PUBLIC_KEY`, um Signaturen wirklich zu prüfen, oder `enforce`, um unsignierte Uploads abzulehnen. https://docs.sigstore.dev/cosign/installation/ |
| `pre-commit` | Offensichtliche Probleme abfangen, bevor Sie eine Erweiterung einreichen. |
| `golangci-lint` | Nötig, wenn Ihre Erweiterung in Go geschrieben ist und Sie dieselbe Lint-Strenge wie der Host wollen. |

## 2. Evey lokal starten

```sh
git clone git@github.com:Evey-project/evey.git
cd evey
cp .env.dev.example .env.dev   # generates a sane local config
make dev                        # docker compose up: postgres, meilisearch, server (Air), web (Vite)
```

Ist der Server gesund, öffnen Sie https://evey.test (Caddy liefert es aus). Die Standard-Zugangsdaten stehen in `.env.dev` - ändern Sie sie, bevor Sie die Instanz erreichbar machen.

Wenn `make dev` an `bwrap` scheitert:
- Prüfen Sie, dass `which bwrap` auf dem Host einen Pfad liefert
- Im Docker-Container bringt der Entwicklungscontainer das bwrap-Binary mit; haben Sie den Compose-Stack angepasst, hängen Sie es ein oder setzen Sie `EVEY_PLUGINS_SANDBOX_MODE=basic` *nur für den Entwicklungscontainer*

## 3. Das Gerüst Ihrer ersten Erweiterung

Beispiele liegen in `examples/plugins/`: Startpunkte in Node.js und Python (`nodejs-example`, `python-example`), eine Metadatenquelle (`metadata-example`), ein Webhook-Empfänger (`webhook-receiver`), ein Widget für die Startseite (`weather`), die Konfiguration einer Erweiterung (`config-demo`) und zwei Themes (`theme-dark-blue`, `open-workshop`). Nehmen Sie das, was Ihrem Fall am nächsten kommt, und kopieren Sie es in ein Arbeitsverzeichnis. Für Datenspeicherung und Berechtigungen siehe **`examples/plugins/data-demo`** - es nutzt den Datenspeicher pro Erweiterung und verlangt die Berechtigungen `public` / `prefs` sowie den Slot `admin.nav`.

```sh
cp -r examples/plugins/nodejs-example ~/my-first-plugin
cd ~/my-first-plugin
```

Bearbeiten Sie `plugin.yaml`:

- `name`: kebab-case, eindeutig
- `version`: semver - bei jeder Veröffentlichung erhöhen
- `kind`: `plugin` (Backend nötig) oder `theme` (nur Oberfläche)
- `language`: `python` | `nodejs` | `go` | `php` | `ruby` | `lua` | `bash` | `sh` (Kennung im Manifest - nicht der Name des Interpreter-Binarys; `python3`/`node` sind keine gültigen `language`-Werte, siehe `entrypoint` unten).
- `entrypoint`: die Befehlszeile, die die Laufzeit ausführt
- `permissions`: Geben Sie jede Host-API an, die Sie aufrufen. Das Installationsprogramm lehnt Aufrufe ab, die nicht angegeben sind. Die vollständige Liste steht im [Berechtigungskatalog](security-model.md#permissions-catalog).
- `external_fetch`: wenn Sie mit einer API eines Drittanbieters sprechen müssen. Einträge sind nackte Hostnamen (`api.example.com`) - kein Schema nötig oder verwendet (ein Präfix `https://` wird gelesen und nur der Hostname behalten), keine Platzhalter (`*.example.com` wird bei der Installation abgelehnt: `external_fetch entry %q is a wildcard - use explicit hostnames`), keine literalen IP-Adressen (keine einzige, nicht nur RFC1918). Die Prüfung steckt in `internal/adapters/driven/pluginrunner/manifest.go` (`validateExternalFetch`); ihre Tests in `internal/adapters/driven/pluginrunner/manifest_test.go` - führen Sie `go test ./internal/adapters/driven/pluginrunner/...` aus, um zu prüfen, ob Ihr Wert passt.

Frontend-Modul (optional für `kind: plugin`, verboten für `kind: theme`):

```yaml
frontend:
  - slot: home.section
    src: dist/index.js
```

Der Slot muss in `internal/domain/plugin/slots.go` existieren (`home.section`, `home.hero`, `library.sidebar`, `media.detail.sidebar`, `player.overlay`, `sidebar.bottom`, `settings.user.appearance` und `admin.nav` - der eine kontrollierte Slot im Administrationsbereich, ein Menüeintrag, der die eigene abgeschottete Seite Ihrer Erweiterung öffnet). Alles andere (`admin.*`-Injektion, `login.*`, `setup.*`, `profile.security.*`) wird bei der Installation abgelehnt.

**Daten speichern.** Ihr Backend bekommt über den Host-Socket einen privaten
Schlüssel-Wert-Speicher (`SetData`/`GetData`/`DeleteData`/`ListDataKeys`) - ohne
Berechtigung, bei der Deinstallation automatisch gelöscht. Für Einstellungen pro
Benutzer aus Ihrem iframe verlangen Sie die Berechtigung `prefs`; für
Lesezugriff auf nicht sensible Medien (Titel/Poster) `public`. Siehe die
[API-Referenz](api-reference.md#permissions). Bereiche für Administration,
Anmeldung, System und allgemeines Schreiben lassen sich nie anfordern.

## 4. Die Installationsschleife

```sh
# zip your plugin
cd ~/my-first-plugin
zip -r my-first-plugin.zip . -x ".*" "node_modules/*" "*.pyc"

# upload through the admin UI
open https://evey.test/admin/plugins
# or via the API
curl -F "file=@my-first-plugin.zip" \
     -H "Authorization: Bearer $EVEY_TOKEN" \
     https://evey.test/api/v1/admin/plugins
```

Das Installationsprogramm läuft in dieser Reihenfolge - jeder Schritt kann die Installation ablehnen:

1. **Prüfung des Manifests** - `plugin.yaml` wird direkt aus der ZIP-Datei gelesen (vor allen Prüfungen auf ZIP-Ebene unten) und gegen das Schema geprüft. Positivliste der Slots. Positivliste der Berechtigungen/Fähigkeiten. Parser für `external_fetch` (lehnt Platzhalter und jede literale IP-Adresse ab; ein Schema-Präfix wie `http://` wird akzeptiert, aber ignoriert - nur der Hostname bleibt). Die angegebene `language`-Laufzeit muss im Image verfügbar sein.
2. **Rechteausweitung** (nur bei Updates) - das Installationsprogramm vergleicht das neue Manifest mit der Vorversion. Jede neu verlangte Berechtigung/Fähigkeit, jeder neue Slot oder `external_fetch`-Host blockiert die Installation mit einem Fehler, den der Aufrufer ausdrücklich übergehen muss (`accept_permission_changes: true` aus der Oberfläche). Neuinstallationen überspringen diesen Schritt - es gibt nichts zu vergleichen.
3. **Cosign-Signatur** - geprüft, wenn eine Signaturprüfung konfiguriert ist; ein Fehlschlag blockiert die Installation nur bei `EVEY_PLUGINS_COSIGN_POLICY=enforce`. Siehe Abschnitt 6.
4. **Entpacken der ZIP-Datei** - Endung, Gesamtgröße ≤ 500 MB, Größe pro Eintrag ≤ 100 MB, Kompressionsverhältnis ≤ 100:1, keine symbolischen Links, keine Einträge mit Pfadüberschreitung; die Dateien landen in einem Staging-Verzeichnis (Upgrades rühren die laufende Version erst an, wenn alle späteren Schritte bestanden sind).
5. **CSS-Bereinigung** (nur Themes) - entfernt `@import`, blockiert `url()` außer `data:`, blockiert Vollbild-Überlagerungen, blockiert Selektoren auf `[type=password]` und `.admin-*`, begrenzt `z-index` auf ≤ 999.
6. **Scanner für Theme-JS** (nur Themes) - lehnt jeden Verweis auf `document.cookie`, `localStorage`, `fetch`, `eval`, `globalThis`, Eigenschaftszugriffe mit eckigen Klammern auf globale Host-Objekte, `constructor.constructor` usw. ab. Themes dürfen nur die API `evey.theme.*` nutzen.
7. **Statische Analyse** - durchsucht jede Textdatei nach Shell-Ausführung, Reverse Shell, eval, Base64-Dekodieren-dann-Ausführen, Verschleierungsmustern, Installationsbefehlen zur Laufzeit, Rechteausweitung. Kritische Funde = Installation abgelehnt. Warnungen (Deserialisierung, Verschleierung, nativer Code) werden zur Ablehnung, sofern die passende Berechtigung nicht angegeben ist.
8. **Übernahme** - tauscht das Staging-Verzeichnis ein, schreibt den Eintrag in Postgres, startet optional die Laufzeit.

Schlägt ein Schritt fehl, zeigt die Oberfläche den genauen Grund. Die Serverlogs (`docker compose logs evey`) enthalten den vollständigen Verlauf.

## 5. Weiterentwickeln

Laden Sie Ihre Erweiterung während der Entwicklung im laufenden Betrieb neu:

```sh
# edit code → re-zip → re-upload via UI (Update button)
# or, faster: drop the file directly into the plugin directory inside the container
docker compose exec evey ls /var/lib/evey/plugins/<plugin-id>/
```

Wenn Sie `version` erhöhen, nimmt das Installationsprogramm den Upload als Update an. Fügen Sie eine neue Berechtigung hinzu, landen Sie im Ablauf der Rechteausweitung - das ist Absicht, kein Fehler.

Die Logs des Erweiterungsprozesses landen:
- unter `/admin/plugins/<id>` - die letzten N Zeilen in der Oberfläche
- in `docker compose logs evey | grep <plugin-id>` - der vollständige Strom

## 6. Ihre Erweiterung signieren (in der Entwicklung optional, zur Verbreitung empfohlen)

Wollen Sie die Erweiterung an andere Evey-Instanzen verteilen, signieren Sie sie. Der empfangende Administrator kann dann `EVEY_PLUGINS_COSIGN_POLICY=warn` (oder `enforce`) einsetzen, um unsignierte Uploads sichtbar zu machen.

```sh
# one-time: generate a keypair (keep cosign.key safe - never commit it)
cosign generate-key-pair

# sign the ZIP
cosign sign-blob \
  --key cosign.key \
  --output-signature my-first-plugin.zip.sig \
  my-first-plugin.zip

# distribute: my-first-plugin.zip + my-first-plugin.zip.sig + cosign.pub
```

Auf der Evey-Instanz, die die Erweiterung installiert:

```sh
# put the public key somewhere readable by the server
sudo cp cosign.pub /etc/evey/cosign.pub

# point Evey at it and switch policy
export EVEY_PLUGINS_COSIGN_PUBLIC_KEY=/etc/evey/cosign.pub
export EVEY_PLUGINS_COSIGN_POLICY=warn   # or "enforce" to refuse unsigned
docker compose restart evey
```

Evey prüft den Schlüssel beim Start (PEM-Parsing) - eine fehlende oder fehlerhafte Schlüsseldatei lässt den Start scheitern statt erst die erste Installation. KMS-Verweise (`gcpkms://...`, `awskms://...`, `k8s://...`) werden unbesehen akzeptiert.

## 7. Ressourcengrenzen und Sandbox

Voreinstellungen für jeden Erweiterungsprozess:

| Umgebungsvariable | Standard | Wirkung |
|---|---|---|
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto` = wählt das Beste Verfügbare (normalerweise `landlock`). `landlock` = Positivliste über Landlock LSM + bedingungslose Sperre ausgehender TCP-Verbindungen (`-restrict-net` + seccomp `-deny-inet`, keine Ausnahme für die Berechtigung `network`) + LD_PRELOAD-seccomp. `basic` = nur LD_PRELOAD-seccomp + UID-Wechsel (Debug). `bubblewrap` = vollständige Namespaces + Root schreibgeschützt eingebunden + `--unshare-net` (ebenfalls bedingungslos, für jede Erweiterung, unabhängig von der Berechtigung `network`). `strict` = bubblewrap + seccomp-Positivliste. |
| `EVEY_PLUGINS_MEMORY_MB` | 256 | Maximaler RSS. Der Ressourcenmonitor fragt `/proc` ab und stoppt die Erweiterung darüber. `0` schaltet ab. |
| `EVEY_PLUGINS_CPU_NICE` | 10 | `nice`-Wert (0-19), beim Start gesetzt, damit eine Erweiterung dem Server nie die CPU entzieht. |
| `EVEY_PLUGINS_PIDS_MAX` | 64 | Maximale Zahl von Threads/Prozessen. Verhindert Fork-Bomben. `0` schaltet ab. |
| `EVEY_PLUGINS_DISK_MAX_MB` | 100 | Maximale Größe des Installationsverzeichnisses; der Monitor misst sie etwa alle 30 s und stoppt eine Erweiterung darüber. `0` schaltet ab. |

Eine Erweiterung, die ins Netz spricht, braucht sowohl `permissions: [network]` als auch `external_fetch: [<host>...]`. Direkter Netzwerkzugang nach außen ist in der Sandbox für jede Erweiterung bedingungslos gekappt, unabhängig von ihren Berechtigungen (siehe die Sandbox-Modi oben) - der Erweiterungsprozess selbst kann nie einen Socket ins Internet öffnen. Der einzige Weg hinaus ist der `Fetch`-RPC-Vermittler auf Host-Seite: Die Erweiterung ruft ihn über den Unix-Socket `EVEY_PLUGIN_SOCK` auf, der Host prüft den Zielhost gegen `external_fetch` und wählt sich dann über einen SSRF-sicheren Client ein (private/reservierte IPs werden beim Verbinden abgelehnt, Weiterleitungen gegen die aufgelöste IP und den erlaubten Host erneut geprüft). Siehe `internal/adapters/driving/pluginhost/fetch.go`.

## 8. Eine Erweiterung verbreiten

Einen offiziellen Katalog gibt es noch nicht: Ein Administrator installiert eine
Erweiterung, indem er ihre ZIP-Datei im Administrationsbereich hochlädt, von
wo auch immer Sie sie veröffentlichen (ein GitHub-Release, Ihre eigene Seite).
Was „Verbreiten“ heute bedeutet:

- Liefern Sie die ZIP-Datei, ihre `.sig` und Ihren `cosign.pub` zusammen aus
  (Abschnitt 6), damit der empfangende Administrator den Upload mit
  `EVEY_PLUGINS_COSIGN_POLICY=warn` oder `enforce` prüfen kann.
- Nennen Sie die Berechtigungen und die `external_fetch`-Hosts in Ihrem README -
  der Administrator sieht sie bei der Installation und muss sie genehmigen; eine
  Überraschung dort ist der häufigste Grund, warum eine Installation abgelehnt
  wird.
- Damit Ihre Erweiterung bei den mitgelieferten Beispielen aufgeführt wird,
  öffnen Sie einen PR gegen `examples/plugins/` mit dem Quellcode und einem
  README. Beispiele werden geprüft und aus dem Quellcode gebaut; ein PR ist keine
  Empfehlung und trägt keine Signatur des Projekts.

## 9. Fehlersuche

| Symptom | Ursache | Abhilfe |
|---|---|---|
| `bwrap: ... permission denied` | Das Standard-seccomp von Docker blockiert `unshare`. | Das Repository liefert ein eigenes seccomp-Profil (`docker/seccomp.json`). Achten Sie darauf, dass Ihre Compose-Datei es anwendet. |
| `forbidden pattern "fetch(" found in theme JS` | Der Scanner für Theme-JS hat eine verbotene API gefunden. | Themes dürfen nur `evey.theme.*` aufrufen. Was blockiert ist, steht in `internal/pkg/themejs/scan.go`. |
| `manifest: external_fetch entry "*.tmdb.org" is a wildcard - use explicit hostnames` | Alle Platzhalter werden bei der Installation abgelehnt, ohne Ausnahme. | Nutzen Sie `api.tmdb.org` (und führen Sie jede nötige Subdomain einzeln auf), nicht `*.tmdb.org` oder `cdn.*.tmdb.org`. |
| `installer: slot "admin.section" not allowed` | Frontends von Erweiterungen sind in privilegierten Bereichen verboten. | Wählen Sie einen unkritischen Slot aus der Positivliste oder überdenken Sie die Erweiterung. |
| `cosign verify-blob: failed signature` | Der öffentliche Schlüssel auf dem Server passt nicht zum privaten Schlüssel, mit dem signiert wurde. | Mit dem richtigen Schlüssel neu signieren oder `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` anpassen. |
| Erweiterung startet und stirbt mit Exit 137 | Von der cgroup wegen Speichermangels beendet. | Speicherverbrauch senken oder `EVEY_PLUGINS_MEMORY_MB` erhöhen. |
| Frontend-Modul nicht sichtbar | Erweiterung nicht aktiviert, Slot passt nicht, oder falsches `web_root`-Präfix in `frontend.src`. | Prüfen Sie die Plugins-Seite der Administration (Statusabzeichen), vergewissern Sie sich, dass der Slot-Name zu einem echten `<PluginSlot name=...>` passt, und sehen Sie im Netzwerk-Tab nach Aufrufen von `/api/v1/plugins/{id}/assets/...`. |

## 10. Weiterführendes

- Das Sicherheitsmodell von Anfang bis Ende: [security-model.md](security-model.md)
- Die API des Hosts: [api-reference.md](api-reference.md)
- Verfügbare Slots: [api-reference.md](api-reference.md#slots)
- SDK-Code je Sprache: `sdk/`
- Referenz-Erweiterungen und -Themes: `examples/plugins/`
