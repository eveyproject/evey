---
source_sha: c9ccdddebbef17e96ff8f3da3475c803b0d09dc237cd4c140c897bdfdaf289c5
reviewed: false
---
# Einrichtung für Plugin-Entwickler

Durchgehende Anleitung für alle, die Evey gerade geklont haben und ein Plugin oder Theme schreiben, testen und ausliefern wollen. Wählen Sie die Laufzeit, die zur Sprache Ihres Plugins passt; der Rest ist gemeinsam.

## 1. Voraussetzungen

Evey führt den Host-Prozess aus (Go) und liefert Plugins als ZIP-Dateien aus. Die Laufzeit des Plugins hängt davon ab, was Ihr Code aufruft.

### Immer erforderlich

| Werkzeug | Wozu | Installation |
|---|---|---|
| Go ≥ 1.27 | Den Evey-Host bauen | `https://go.dev/dl/` |
| Docker + Docker Compose | Standard-Entwicklungsstack (Postgres, Meilisearch, Server, Web) | https://docs.docker.com/engine/install/ |
| `bubblewrap` (`bwrap`) | Sandbox für Plugin-Prozesse - erforderlich für die Modi `bubblewrap` und `strict` | `apt install bubblewrap` (Debian/Ubuntu), `dnf install bubblewrap` (Fedora), `pacman -S bubblewrap` (Arch). macOS: Linux in einer VM installieren, bwrap gibt es nur unter Linux. |

Der Sandbox-Modus `basic` läuft ohne bwrap, sollte aber nie außerhalb von lokalen Wegwerfexperimenten genutzt werden - verlassen Sie ihn, sobald Sie können.

### Pro Sprache

Wählen Sie, was Ihr Plugin tatsächlich nutzt; Sie brauchen nicht alles.

| Plugin-Sprache | Werkzeuge | Hinweise |
|---|---|---|
| Python 3 | `python3`, `pip` | `pip install --user` nutzen, um den Host sauber zu halten. Die SDK-Stubs liegen in `sdk/python/`. |
| Node.js | Node 20+, `npm` oder `pnpm` | Das SDK lädt `plugin_api.proto` zur Laufzeit über `@grpc/proto-loader`. |
| Go | Go ≥ 1.21 | Keine mitgelieferten Stubs - mit `protoc-gen-go` + `protoc-gen-go-grpc` erzeugen; ein kompiliertes Go-Programm braucht außerdem die Berechtigung `native-binary`. |
| Dart / Java / PHP / Ruby / Swift | Werkzeugkette der Sprache | Die Stubs liegen vorgeneriert in `sdk/<lang>/`. |
| Nur-CSS-Theme | Nichts weiter | Ein Theme ist ein ZIP mit `kind: theme` + einer `theme.css`. |
| Theme-JS | Nichts weiter (der Host führt Ihr JS in einem abgeschotteten `iframe` mit `sandbox="allow-scripts"` und einer verriegelten CSP aus) | Der statische Scanner verweigert Cookies, Speicher, Netzwerkaufrufe und den Zugriff auf die Host-Seite (`document.cookie`, `localStorage`, `fetch(`, `window.parent`...) - siehe [Themes](../themes.md#what-the-scanner-refuses). |

### Stubs für eine andere Sprache erzeugen

Für die Sprachen oben werden Stubs vorgeneriert mitgeliefert, aber der gRPC-Vertrag
der Plugins ist nur eine Proto-Datei - Sie können **jede Sprache, die `protoc`
unterstützt**, ansteuern (Rust, C#, Kotlin, Elixir, …). Die einzige Quelle der
Wahrheit ist:

```
proto/plugin/v1/plugin_api.proto
```

Erzeugen Sie mit `protoc` (oder `buf`) und richten Sie das gRPC-Plugin Ihrer Sprache darauf aus:

```bash
# allgemeine Form - die genauen --*_out-Optionen hängen von Sprache/Plugin ab
protoc --proto_path=proto \
       --<lang>_out=OUT_DIR --grpc_<lang>_out=OUT_DIR \
       proto/plugin/v1/plugin_api.proto
```

Ausgearbeitete Rezepte für die mitgelieferten Sprachen (welches gRPC-Plugin jede
braucht: `grpc_php_plugin`, `protoc-gen-grpc-java`, `protoc-gen-grpc-swift`, …)
stehen im Ziel `plugin-sdk` des `Makefile` im Wurzelverzeichnis und in jeder
`sdk/<lang>/README.md` (z. B. `sdk/php/README.md`). `make plugin-sdk` erzeugt die
mitgelieferten Stubs neu.

Legen Sie die erzeugten Stubs in Ihr Plugin-ZIP - der Host kopiert Stubs nur für
die Sprachen, die er mitliefert, automatisch (`sdk/<lang>/`).

### Optional, aber empfohlen

| Werkzeug | Wozu |
|---|---|
| `cosign` (Sigstore) | Ihr Plugin-ZIP signieren. `EVEY_PLUGINS_COSIGN_POLICY` ist standardmäßig `warn` (nicht `off`) - mit `warn` und ohne konfigurierten öffentlichen Schlüssel tut die Prüfung nichts (eine Warnung wird protokolliert, nichts wird blockiert); setzen Sie `EVEY_PLUGINS_COSIGN_PUBLIC_KEY`, um Signaturen wirklich zu prüfen, und die Richtlinie auf `enforce`, um unsignierte Uploads abzulehnen (`enforce` ohne öffentlichen Schlüssel hält den Server beim Start an, statt jedes Plugin ungeprüft anzunehmen). https://docs.sigstore.dev/cosign/installation/ |
| `pre-commit` | Offensichtliche Probleme abfangen, bevor Sie ein Plugin einreichen. |
| `golangci-lint` | Erforderlich, wenn Ihr Plugin in Go ist und Sie dieselbe Lint-Prüfung wie der Host wollen. |

## 2. Evey lokal starten

```sh
git clone git@github.com:Evey-project/evey.git
cd evey
cp .env.dev.example .env.dev   # erzeugt eine vernünftige lokale Konfiguration
make dev                        # docker compose up: postgres, meilisearch, Server (Air), Web (Vite)
```

Ist der Server gesund, öffnen Sie https://evey.test (von Caddy ausgeliefert). Der Einrichtungsassistent legt beim ersten Start das Administratorkonto an, wie unter [Ersteinrichtung](../getting-started/first-setup.md) beschrieben.

Wenn `make dev` an `bwrap` scheitert:
- Prüfen Sie, dass `which bwrap` auf dem Host einen Pfad liefert
- In Docker bringt der Entwicklungscontainer das Programm bwrap mit; haben Sie den Compose-Stack angepasst, binden Sie es ein oder setzen Sie `EVEY_PLUGINS_SANDBOX_MODE=basic` *nur für den Entwicklungscontainer*

## 3. Ihr erstes Plugin anlegen

Beispiele liegen in `examples/plugins/`: Vorlagen in Node.js und Python (`nodejs-example`, `python-example`), ein Metadatenanbieter (`metadata-example`), ein Webhook-Empfänger (`webhook-receiver`), ein Widget für die Startseite (`weather`), Plugin-Konfiguration (`config-demo`) und zwei Themes (`theme-dark-blue`, `open-workshop`). Wählen Sie das Ihrem Anwendungsfall nächste und kopieren Sie es in ein Arbeitsverzeichnis. Für Persistenz und Berechtigungen siehe **`examples/plugins/data-demo`** - es nutzt den Datenspeicher pro Plugin und verlangt die Berechtigungen `public` / `prefs` sowie den Slot `admin.nav`.

```sh
cp -r examples/plugins/nodejs-example ~/my-first-plugin
cd ~/my-first-plugin
```

Bearbeiten Sie `plugin.yaml`:

- `name`: kebab-case, eindeutig
- `version`: semver - bei jeder Veröffentlichung erhöhen
- `kind`: `plugin` (Backend erforderlich) oder `theme` (nur Frontend)
- `language`: `python` | `nodejs` | `go` | `php` | `ruby` | `lua` | `bash` | `sh` (Kennung im Manifest - nicht der Name des Interpreters; `python3`/`node` sind keine gültigen Werte für `language`, siehe `entrypoint` unten).
- `entrypoint`: die Befehlszeile, die die Laufzeit ausführt
- `permissions`: jede aufgerufene Host-API angeben. Der Installer lehnt nicht angegebene Aufrufe ab. Die vollständige Liste steht im [Berechtigungskatalog](security-model.md#permissions-catalog).
- `external_fetch`: wenn Sie mit der API eines Dritten sprechen müssen. Einträge sind reine Hostnamen (`api.example.com`) - kein Schema nötig oder genutzt (ein Präfix `https://` wird geparst und nur der Hostname behalten), keine Platzhalter (`*.example.com` wird bei der Installation abgelehnt: `external_fetch entry %q is a wildcard - use explicit hostnames`), keine IP-Literale (alle, nicht nur RFC1918). Der Validator liegt in `internal/adapters/driven/pluginrunner/manifest.go` (`validateExternalFetch`); seine Tests stehen in `internal/adapters/driven/pluginrunner/manifest_test.go` - führen Sie `go test ./internal/adapters/driven/pluginrunner/...` aus, um Ihren Wert zu prüfen.

Frontend-Modul (optional bei `kind: plugin`, verboten bei `kind: theme`):

```yaml title="plugin.yaml"
frontend:
  - slot: home.section
    src: dist/index.js
```

Der Slot muss in `internal/domain/plugin/slots.go` existieren (`home.section`, `home.hero`, `library.sidebar`, `media.detail.sidebar`, `player.overlay`, `sidebar.bottom`, `settings.user.appearance` und `admin.nav` - der eine kontrollierte Slot im Admin-Bereich, ein Menüeintrag, der die eigene abgeschottete Seite Ihres Plugins öffnet). Alles andere (Einschleusen in `admin.*`, `login.*`, `setup.*`, `profile.security.*`) wird bei der Installation abgelehnt.

**Daten speichern.** Ihr Backend erhält über den Host-Socket einen privaten
Schlüssel-Wert-Speicher (`SetData`/`GetData`/`DeleteData`/`ListDataKeys`) - ohne
Berechtigung, bei der Deinstallation automatisch gelöscht. Für Vorlieben pro
Benutzer aus Ihrem iframe verlangen Sie die Berechtigung `prefs`; für
Lesezugriff auf nicht sensible Medien (Titel/Poster) verlangen Sie `public`.
Siehe die [API-Referenz](api-reference.md#permissions). Admin-, Auth-, System-
und allgemeine Schreibbereiche können nie verlangt werden.

## 4. Die Installationsschleife

```sh
# Ihr Plugin zippen
cd ~/my-first-plugin
zip -r my-first-plugin.zip . -x ".*" "node_modules/*" "*.pyc"

# über den Admin-Bereich hochladen
open https://evey.test/admin/plugins
# oder über die API
curl -F "plugin=@my-first-plugin.zip" \
     -H "Authorization: Bearer $EVEY_TOKEN" \
     https://evey.test/api/v1/admin/plugins/upload
# eine Signatur kommt in ein zweites Feld: -F "sig=@my-first-plugin.zip.sig"
# ein Update, das neue Berechtigungen verlangt: -F "accept_permission_changes=true"
```

Der Installer läuft in dieser Reihenfolge - jeder Schritt kann die Installation ablehnen:

1. **Manifest-Prüfung** - `plugin.yaml` direkt aus dem ZIP geparst (vor allen ZIP-Prüfungen unten), gegen das Schema geprüft. Liste der erlaubten Slots. Liste der erlaubten Berechtigungen/Capabilities. Parser für `external_fetch` (lehnt Platzhalter und jedes IP-Literal ab; ein Schemapräfix wie `http://` wird akzeptiert, aber ignoriert - nur der Hostname bleibt). Die angegebene Laufzeit `language` muss im Image verfügbar sein.
2. **Erweiterung der Berechtigungen** (nur bei Updates) - der Installer vergleicht das neue Manifest mit der vorherigen Version. Jede neu verlangte Berechtigung/Capability/jeder neue Slot/Host in `external_fetch` blockiert die Installation mit einem Fehler, den der Aufrufer ausdrücklich übergehen muss (`accept_permission_changes: true` aus dem Frontend). Neuinstallationen überspringen diesen Schritt - es gibt nichts zu vergleichen.
3. **Cosign-Signatur** - geprüft, wenn ein Signaturprüfer konfiguriert ist; ein Fehlschlag blockiert die Installation nur bei `EVEY_PLUGINS_COSIGN_POLICY=enforce`. Siehe Abschnitt 6.
4. **ZIP-Entpacken** - Erweiterung, Gesamtgröße ≤ 500 MB, Größe pro Eintrag ≤ 100 MB, Kompressionsverhältnis ≤ 100:1, keine symbolischen Links, keine Einträge mit Pfad-Traversal; die Dateien landen in einem Staging-Verzeichnis (Updates berühren die laufende Version nie, bis jeder spätere Schritt bestanden ist).
5. **CSS-Bereiniger** (nur Themes) - entfernt `@import`, blockiert `url()` außer `data:`, blockiert Vollbild-Überlagerungen, blockiert Selektoren auf `[type=password]` und `.admin-*`, begrenzt `z-index` auf ≤ 999.
6. **Theme-JS-Scanner** (nur Themes) - verweigert jede Nennung von `document.cookie`, `localStorage`, `fetch`, `eval`, `globalThis`, Eigenschaftszugriffe in Klammernotation auf Host-Globale, `constructor.constructor` usw. Themes dürfen nur die API `evey.theme.*` nutzen.
7. **Statische Analyse** - durchsucht jede Textdatei nach Shell-Ausführung, Reverse-Shell, eval, base64-Dekodieren-dann-Ausführen, Verschleierungsmustern, Installationsbefehlen zur Laufzeit, Rechteausweitung. Kritische Funde = Installation abgelehnt. Warnungsfunde (Deserialisierung, Verschleierung, native_code) werden zu einer Ablehnung, außer die passende Berechtigung ist angegeben.
8. **Speichern** - tauscht das Staging-Verzeichnis ein, schreibt die Zeile in Postgres, startet optional die Laufzeit.

Scheitert ein Schritt, zeigt der Admin-Bereich den genauen Grund. Die Server-Logs (`docker compose logs evey`) enthalten den vollständigen Stack.

## 5. Iterieren

Ihr Plugin während der Entwicklung neu laden:

```sh
# Code bearbeiten → neu zippen → über die Oberfläche neu hochladen (Schaltfläche Update)
# oder, schneller: die Datei direkt in das Plugin-Verzeichnis im Container legen
docker compose exec evey ls /data/plugins/<plugin-id>/
```

Ein Upload wird nur dann als Update angenommen, wenn seine `version` höher als die installierte ist: Erhöhen Sie sie bei jeder Iteration (eine gleiche oder niedrigere Version wird abgelehnt), oder deinstallieren Sie zuerst. Fügen Sie eine neue Berechtigung hinzu, landen Sie im Ablauf zur Erweiterung der Berechtigungen - das ist gewollt, kein Fehler.

Die Logs des Plugin-Prozesses landen in:
- `/admin/plugins/<id>` - die letzten N Zeilen über die Oberfläche
- `docker compose logs evey | grep <plugin-id>` - der vollständige Strom

## 6. Ihr Plugin signieren (optional in der Entwicklung, empfohlen für die Verteilung)

Wenn Sie das Plugin an andere Evey-Instanzen verteilen wollen, signieren Sie es. Der empfangende Administrator kann dann `EVEY_PLUGINS_COSIGN_POLICY=warn` (oder `enforce`) setzen, um unsignierte Uploads sichtbar zu machen.

```sh
# einmalig: ein Schlüsselpaar erzeugen (cosign.key sicher aufbewahren - nie committen)
cosign generate-key-pair

# das ZIP signieren
cosign sign-blob \
  --key cosign.key \
  --output-signature my-first-plugin.zip.sig \
  my-first-plugin.zip

# verteilen: my-first-plugin.zip + my-first-plugin.zip.sig + cosign.pub
```

Auf der Evey-Instanz, die das Plugin installiert:

```sh
# den öffentlichen Schlüssel an einen für den Server lesbaren Ort legen
sudo cp cosign.pub /etc/evey/cosign.pub

# Evey darauf verweisen und die Richtlinie umstellen
export EVEY_PLUGINS_COSIGN_PUBLIC_KEY=/etc/evey/cosign.pub
export EVEY_PLUGINS_COSIGN_POLICY=warn   # oder "enforce", um Unsigniertes abzulehnen
docker compose restart evey
```

Evey prüft den Schlüssel beim Start (PEM-Parse) - eine fehlende oder fehlerhafte Schlüsseldatei lässt den Start scheitern statt der ersten Installation. KMS-Verweise (`gcpkms://...`, `awskms://...`, `k8s://...`) werden unverändert akzeptiert.

## 7. Ressourcengrenzen und Sandbox

Standards für jeden Plugin-Prozess:

| Umgebungsvariable | Standard | Wirkung |
|---|---|---|
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto` = wählt das beste verfügbare (normalerweise `landlock`). `landlock` = Landlock-LSM-Erlaubnisliste + bedingungslose Sperre von ausgehendem TCP (`-restrict-net` + seccomp `-deny-inet`, keine Ausnahme für die Berechtigung `network`) + LD_PRELOAD-seccomp. `basic` = LD_PRELOAD-seccomp + nur UID-Herabstufung (Debug). `bubblewrap` = vollständige Namespaces + Wurzel schreibgeschützt eingebunden + `--unshare-net` (ebenfalls bedingungslos, für jedes Plugin, unabhängig von der Berechtigung `network`). `strict` = bubblewrap + der LD_PRELOAD-seccomp-Shim. |
| `EVEY_PLUGINS_MEMORY_MB` | 256 | Maximaler RSS. Der Ressourcenmonitor fragt `/proc` ab und stoppt das Plugin darüber. `0` schaltet es ab. |
| `EVEY_PLUGINS_CPU_NICE` | 10 | `nice`-Wert (0-19) beim Start, damit ein Plugin den Server nie um CPU bringen kann. |
| `EVEY_PLUGINS_PIDS_MAX` | 64 | Maximale Zahl von Threads/Prozessen. Verhindert Fork-Bomben. `0` schaltet es ab. |
| `EVEY_PLUGINS_DISK_MAX_MB` | 100 | Maximale Größe des Installationsverzeichnisses; der Monitor misst sie etwa alle 30 s und stoppt ein Plugin darüber. `0` schaltet es ab. |

Ein Plugin, das mit dem Netzwerk spricht, braucht sowohl `permissions: [network]` als auch `external_fetch: [<host>...]`. Der direkte ausgehende Netzwerkverkehr wird in der Sandbox für jedes Plugin bedingungslos gekappt, unabhängig von seinen Berechtigungen (siehe die Sandbox-Modi oben) - der Plugin-Prozess selbst kann nie einen Socket ins Internet öffnen. Der einzige Weg hinaus ist der Broker `Fetch` des Hosts: Das Plugin ruft ihn über den Unix-Socket `EVEY_PLUGIN_SOCK` auf, der Host prüft den Zielhost gegen `external_fetch` und verbindet dann über einen SSRF-sicheren Client (private/reservierte IPs beim Verbinden abgelehnt, Weiterleitungen gegen die aufgelöste IP und den erlaubten Host erneut geprüft). Siehe `internal/adapters/driving/pluginhost/fetch.go`.

## 8. Ein Plugin verteilen

Es gibt noch keinen offiziellen Katalog: Ein Administrator installiert ein Plugin,
indem er sein ZIP im Admin-Bereich hochlädt, von wo auch immer Sie es
veröffentlichen (ein GitHub-Release, Ihre eigene Website). Was „Verteilen" heute
bedeutet:

- Liefern Sie das ZIP, seine `.sig` und Ihren `cosign.pub` zusammen aus
  (Abschnitt 6), damit der empfangende Administrator den Upload mit
  `EVEY_PLUGINS_COSIGN_POLICY=warn` oder `enforce` prüfen kann.
- Nennen Sie die Berechtigungen und die Hosts in `external_fetch` in Ihrer
  README - der Administrator sieht sie bei der Installation und muss sie
  genehmigen; eine Überraschung dort ist der erste Grund, warum eine
  Installation abgelehnt wird.
- Damit Ihr Plugin bei den mitgelieferten Beispielen aufgeführt wird, öffnen Sie
  einen PR gegen `examples/plugins/` mit dem Quellcode und einer README.
  Beispiele werden geprüft und aus dem Quellcode gebaut; ein PR ist keine
  Empfehlung und trägt keine Signatur des Projekts.

## 9. Fehlerbehebung

| Symptom | Ursache | Behebung |
|---|---|---|
| `bwrap: ... permission denied` | Modus `bubblewrap` in Docker: Die Standardprofile von seccomp und AppArmor blockieren die Aufrufe `unshare` und `mount`, die er braucht. | Behalten Sie den Standardmodus `auto`, der im Container Landlock nutzt. Sonst wenden Sie das mitgelieferte Profil `docker/seccomp-evey.json` an (auch im Image unter `/etc/evey/seccomp.json`), eine bewusste Absenkung der Sicherheit: siehe [Plugin-Sandbox einrichten](sandbox-deployment.md). |
| `forbidden pattern "fetch(" found in theme JS` | Der Theme-JS-Scanner hat eine verbotene API gefunden. | Themes dürfen nur `evey.theme.*` aufrufen. Lesen Sie `internal/pkg/themejs/scan.go`, um zu sehen, was blockiert wird. |
| `manifest: external_fetch entry "*.tmdb.org" is a wildcard - use explicit hostnames` | Alle Platzhalter werden bei der Installation abgelehnt, ohne Ausnahme. | Nutzen Sie `api.tmdb.org` (und listen Sie jede benötigte Subdomain ausdrücklich), nicht `*.tmdb.org` oder `cdn.*.tmdb.org`. |
| `manifest: frontend slot "admin.section" not allowed` | Plugin-Frontends sind in privilegierten Bereichen verboten. | Wählen Sie einen nicht kritischen Slot aus der Liste oder überdenken Sie das Plugin. |
| `cosign verify-blob: failed signature` | Der öffentliche Schlüssel auf dem Server passt nicht zum privaten Schlüssel, mit dem signiert wurde. | Mit dem richtigen Schlüssel neu signieren oder `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` aktualisieren. |
| `plugin already installed at version 1.0.0, new version must be higher` | Das ZIP trägt die bereits installierte Version. | `version` in `plugin.yaml` erhöhen oder das Plugin zuerst deinstallieren. |
| Das Plugin stoppt, das Log sagt `memory quota exceeded (… MB > … MB)` | Der Ressourcenmonitor hat es über seiner Speichergrenze beendet (Exit 137). | Den Speicherverbrauch senken oder die Speichergrenze des Plugins auf seiner Karte unter **Admin → Plugins** erhöhen (`EVEY_PLUGINS_MEMORY_MB` setzt den Standard). |
| Frontend-Modul nicht sichtbar | Plugin nicht aktiviert, Slot passt nicht oder falsches Präfix `web_root` in `frontend.src`. | Prüfen Sie die Plugins-Seite des Admin-Bereichs (Statusabzeichen), dass die Slot-Zeichenkette zu einer echten Einbindung `<PluginSlot name=...>` passt, und sehen Sie im Netzwerkbereich nach Aufrufen von `/api/v1/plugins/{id}/assets/...`. |

## 10. Weiterführend

- Sicherheitsmodell von A bis Z: [security-model.md](security-model.md)
- Die API des Hosts: [api-reference.md](api-reference.md)
- Verfügbare Slots: [api-reference.md](api-reference.md#slots)
- SDK-Code pro Sprache: `sdk/`
- Referenz-Plugins und -Themes: `examples/plugins/`
