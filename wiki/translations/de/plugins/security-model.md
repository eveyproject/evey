---
source_sha: 80123a8d7d69b50b13baa0a7c638767ebf54582aa06f7b85e176a6fee202d4ca
---
# Sicherheitsmodell für Erweiterungen: was ein Autor angeben darf

> **Welche Seite suchen Sie?** Diese hier ist für Autoren von Erweiterungen: der
> Katalog der Berechtigungen, die Regeln, die ein externer Abruf erfüllen muss,
> die Slots, in die Sie sich einhängen dürfen, und was bei einem Update
> passiert. Wenn Sie einen Server betreiben und wissen wollen, wie eine
> Erweiterung eingesperrt wird und was die Sandbox noch nicht verhindern kann,
> lesen Sie stattdessen [Sandbox und Bedrohungsmodell für Erweiterungen](../SECURITY.md).

Evey-Erweiterungen sind Code von Dritten, der als eigene Betriebssystemprozesse neben dem Server läuft. Dieses Dokument erklärt, was eine bösartige Erweiterung versuchen könnte, wie Evey sich gegen jeden Angriffsweg wehrt, und wo die ehrlichen Grenzen der heutigen Umsetzung liegen.

## Bedrohungsmodell: was einem Manifest anvertraut wird

| Bedrohung | Beschreibung |
|---|---|
| RCE auf dem Server | Die Erweiterung bricht aus ihrer Sandbox aus und führt beliebige Befehle als Serverprozess aus |
| Abfluss von Geheimnissen | Die Erweiterung liest JWT-Schlüssel, Datenbank-Zugangsdaten oder Benutzer-Tokens aus dem Dateisystem |
| Horizontale Ausweitung | Die Erweiterung gibt sich über IPC als eine andere aus, um RPCs aufzurufen, für die sie keine Berechtigung hat |
| Abfluss über das Netzwerk | Die Erweiterung öffnet ausgehende Verbindungen, um erbeutete Daten an einen Host des Angreifers zu schicken |
| DoS / Ressourcenmissbrauch | Die Erweiterung erschöpft CPU, Speicher, PIDs oder Plattenplatz (Fork-Bomben, Krypto-Mining, volle Platte) |
| Phishing über das Frontend | Die Erweiterung blendet im Browser des Benutzers eine Vollbild-Überlagerung ein, um Zugangsdaten zu stehlen |
| Lieferkette | Der Autor liefert verschleierten oder kompilierten Code, den die statische Analyse nicht einsehen kann |

## Verteidigungsschichten

### 1. Prüfung des Manifests (bei der Installation)

Jede `plugin.yaml` wird vor dem Entpacken geprüft. Die Installation wird abgelehnt, wenn:

- `kind` fehlt oder nicht `plugin`/`theme` ist
- `kind: plugin` kein `language` oder `entrypoint` hat
- `kind: theme` `language`, `entrypoint` oder `permissions` angibt
- `frontend[].slot` nicht in der serverseitigen Positivliste der Slots steht (siehe [Positivliste der Slots](#slot-whitelist))
- `external_fetch`-Einträge gegen die Regeln der Host-Positivliste verstoßen (siehe [Regeln für externe Abrufe](#external-fetch-rules))
- eine angegebene Fähigkeit oder Berechtigung unbekannt ist

Maßgebliche Quelle: `internal/domain/plugin/model.go`, `internal/adapters/driven/pluginrunner/capabilities.go`.

### 2. Sicheres Entpacken der ZIP-Datei

Bevor irgendeine Datei auf die Platte geschrieben wird:

- Obergrenze der Einträge: höchstens 5.000 Dateien
- Obergrenze pro Datei: 100 MB
- Obergrenze der entpackten Gesamtgröße: 500 MB
- Obergrenze des Kompressionsverhältnisses: 100:1 (Erkennung von ZIP-Bomben)
- Prüfung auf Pfadüberschreitung: Einträge mit `..` im aufgelösten Pfad werden abgelehnt
- Symbolische Links: bedingungslos abgelehnt (die Unix-Typbits in `ExternalAttrs` werden geprüft)
- setuid-/setgid-/Sticky-Bits: bei allen entpackten Dateien entfernt

Quelle: `internal/adapters/driven/pluginrunner/zip.go`.

### 3. Statische Analyse (bei der Installation)

Jede Textdatei der ZIP-Datei wird von einem mehrsprachigen, regex-basierten Analysator geprüft, bevor die Erweiterung in die Datenbank aufgenommen wird. Ein Normalisierungsdurchlauf vorab entschärft verbreitete Verschleierungen (Auflösung von Aliasen, Zusammenfalten von String-Verkettungen, Auflösen von f-Strings).

| Kategorie | Schwere | Beispiele |
|---|---|---|
| `shell_exec` | CRITICAL | `os.system`, `subprocess.run`, `child_process.exec`, PHP `shell_exec`/`proc_open`/`preg_replace /e`, Ruby Backtick/`Kernel.exec`/`open('|cmd')` |
| `network` | WARNING/CRITICAL | `requests.get`, `fetch(`, `net.Dial`, Muster für Reverse-/Bind-Shells |
| `filesystem` | WARNING/CRITICAL | `/etc/passwd`, `.ssh/`, `rm -rf`, Operationen auf Symlinks, `/proc/<pid>/environ` |
| `obfuscation` | CRITICAL | `eval(`, `base64.b64decode`, Hex-Folgen, Ketten von `String.fromCharCode`, PHP-`chr()`-Ketten, `str_rot13`, `assert(base64_decode(...))` |
| `privilege` | CRITICAL | `setuid`, `chmod 4777`, `sudo`, `os.kill(1, ...)` |
| `cryptomining` | CRITICAL | `stratum+tcp`, `xmrig`, `cryptonight` |
| `runtime_install` | CRITICAL | `pip install`, `npm install`, `curl ... \| sh` |
| `dynamic_import` | CRITICAL | `__import__()`, `importlib.import_module`, `getattr(__builtins__, ...)` |
| `sandbox_escape` | CRITICAL | `vm.runInNewContext`, `process.binding`, `new Function(`, Python `__subclasses__()`, `__mro__` |
| `deserialization` | CRITICAL/WARNING | `pickle.loads`, `marshal.loads`, `__reduce__`; WARNING: `yaml.load()` ohne Loader |
| `native_code` | CRITICAL/WARNING | `ctypes.CDLL`, `ffi.Library`; WARNING: Go `unsafe.Pointer` |
| `compiled_binary` | WARNING → BLOCK | `.so`, `.dll`, `.exe`, `.wasm`, `.jar` - für den Scanner undurchsichtig |
| `embedded_archive` | WARNING → BLOCK | `.zip`, `.tar.gz` - kann nicht analysierten Code enthalten |
| `mime_mismatch` | WARNING (wie kritisch behandelt) | Die Dateiendung behauptet Bild/Schrift, der Inhalt ist ausführbar |
| `suspicious_url` | WARNING | Nicht-HTTP-Schema in einem URL-Literal (`ftp://`, `wss://`, `ssh://`) |
| `unvendored_deps` | WARNING | `requirements.txt`/`package.json` mit angegebenen Abhängigkeiten, aber ohne mitgelieferte Module |

CRITICAL-Funde oder `mime_mismatch` setzen `Safe = false` und blockieren die Installation.

`compiled_binary` und `embedded_archive` blockieren die Installation, sofern das Manifest nicht die Berechtigung `native-binary` angibt. Der Administrator sieht die Warnung und die Berechtigung im Vorschau-Schritt, bevor er zustimmt.

Quelle: `internal/adapters/driven/pluginrunner/analyzer.go`, `analyzer_normalize.go`.

#### Nach bestem Bemühen, keine Durchsetzung

Der Analysator arbeitet mit Regex und Mustern. Er entschärft bekannte Klassen der Verschleierung, **kann aber nicht garantieren, jede gezielte Umgehung zu erkennen**. Beispiele für die Abdeckung je Klasse:

| Klasse | Erkannt | Hinweise |
|---|---|---|
| PHP-Variable als Funktion aus String-Verkettung | `$f = "sys" . "tem"; $f($cmd)` (einzeilig) und `$x="sys"; $y="tem"; $f=$x.$y; $f($cmd)` (mehrstufig, Auflösung bis zum Fixpunkt) | Bis zu 5 Fixpunkt-Iterationen, Zyklen `$a=$b; $b=$a` enden sauber |
| Python-Builtin über den Namen | `getattr(__builtins__, "exec")` und `__builtins__.__dict__["exec"]` (als Literal oder über eine aufgelöste Variable) | Der Zugriff mit eckigen Klammern auf das Dict löst Critical aus, wenn das Literal gefährlich ist |
| Ausbruch über JS-`Function.constructor` | `(fn).constructor("...")()` als sofortiger Aufruf und `const F = (fn).constructor; F("...")` (gespeicherte Referenz, verfolgt durch Umschreiben der Bindung) | Aufrufe mit variablem Argument `F(code)` werden zu `.constructor(code)` umgeschrieben und vom Muster für variable Argumente erfasst |
| Mehrstufige, zerstückelte Verkettung | `$a="s"; $b="y"; $c="s"; $d="t"; $e="e"; $f="m"; $g=$a.$b.$c.$d.$e.$f; $g(...)` | Harmlose Bruchstücke werden getrennt von „gefährlichen“ Werten verfolgt; die Gefahrenprüfung greift nur auf den fertig verketteten String |

Jede Umgehungsklasse hat einen dauerhaften Regressionstest in `tests/fixtures/plugins/malicious/`. Eine neue Klasse aufzunehmen verlangt: (1) Fixture eingecheckt, (2) Analyseregel oder Normalisierungsdurchlauf erweitert, (3) `installer_security_test.go` stellt `workerANeeded:true → false` um, sobald die Erkennung greift.

**Außerhalb des Umfangs (die Sandbox setzt durch):** dynamische Umformungen auf AST-Ebene - `strrev("metsys")`, `base64.b64decode(...)`, `eval(open("/encoded").read())`, regex.replace zur Laufzeit. Der Regex-Analysator kann sie nicht auflösen; die Sandbox zur Laufzeit (die Landlock-Positivliste oder die bwrap-Mount-Positivliste, dazu die seccomp-Sperrliste des Kernels) + der Ressourcenmonitor im Userspace + IPC-Peer-Credentials garantieren, dass die Erweiterung nicht ausbrechen, `/data` nicht abfließen lassen und nicht über die angegebenen Berechtigungen hinaus aufsteigen kann.

**Bekannte Fehlalarme:** Der Analysator schlägt auch in Quelltext-Kommentaren und Docstrings an. Eine Zeile `# pip install foo` in einem Kommentar löst die Regel `runtime_install` aus. Hinnehmbar: Betreiber anzuweisen, solche Kommentare vor der Veröffentlichung zu entfernen, ist billiger als ein sprachspezifisches Entfernen von Kommentaren im Regex-Analysator; die bwrap-Mount-Positivliste + der Ressourcenmonitor im Userspace machen die Sperre zur Laufzeit davon unabhängig.

#### Empirische Bestätigung zur Laufzeit

Die Mount-Positivliste wurde durchgängig an einem laufenden Stack über eine Kanarienvogel-Erweiterung (`tests/fixtures/plugins/canary-sandbox-runtime/`) geprüft, die beim Start ihre Sicht auf die Sandbox abtastet:

```
ROOT_LIST count=9 dirs=bin,data,dev,etc,lib,proc,sbin,tmp,usr
STAT ok /data type=dir size=60 kids=1 sample=plugins
STAT_BLOCKED /var, /home, /root, /mnt, /media, /opt, /srv, /run (all ENOENT)
STAT_BLOCKED /etc/passwd, /etc/shadow, /data/jwt.key (all ENOENT)
```

`/data` erscheint nicht, weil das `/data` des Hosts eingehängt wäre (das ist es nie), sondern weil bubblewrap die leeren Elternverzeichnisse des eigenen Schreibziels der Erweiterung (`/data/plugins/<id>`) erzeugt; es enthält daher nur dieses eine Unterverzeichnis. Kein `jwt.key`, keine `backups`, keine `artworks` sind sichtbar. `/proc/1/environ` IST lesbar, aber PID 1 im Namespace ist die Erweiterung selbst (`--unshare-pid`), das Lesen ihrer eigenen Umgebung ist also kein Leck.

Lassen Sie den Kanarienvogel nach jeder Änderung an `runner_bwrap.go` erneut laufen, um zu bestätigen, dass das Mount-Layout weiterhin sperrt.

**Die Grenze der Durchsetzung ist die Sandbox zur Laufzeit (Landlock oder bubblewrap) + seccomp im Kernel + Ressourcenmonitor im Userspace + IPC-Peer-Credentials, nicht der Analysator.** Eine Erweiterung, die ein CRITICAL-Muster am Analysator vorbeischmuggelt, kann trotzdem `/data` nicht lesen (Landlock verweigert es / bwrap hängt es nie ein), keinen verbotenen Systemaufruf machen, keine Fork-Bombe über die PID-Grenze hinaus zünden und keine gRPC-Methoden aufrufen, für die sie keine Berechtigung hat. Der Analysator ist eine Schicht der gestaffelten Verteidigung, nützlich, um offensichtlich bösartigen Code in der Vorschau sichtbar zu machen und die Angriffsfläche zur Laufzeit zu verkleinern - nicht der Perimeter.

### 4. Prüfung der Cosign-Signatur

Einstellbar über den Schlüssel `cosign_policy` in `evey.yaml`:

| Richtlinie | Verhalten |
|---|---|
| `off` | Signatur wird nicht geprüft; `Plugin.Signature` ist nil |
| `warn` (Standard) | Ein Prüffehler wird festgehalten und dem Administrator gezeigt, blockiert die Installation aber nicht; ohne konfigurierten öffentlichen Schlüssel tut die Prüfung nichts |
| `enforce` | Ein Prüffehler blockiert die Installation mit einem Fehler |

Quelle: `internal/adapters/driven/pluginrunner/cosign.go`.

### 5. CSS-Bereinigung

Wird bei der Installation auf `theme.css` angewandt. Das vollständige Regelwerk steht in [`docs/themes/security-model.md`](../themes/security-model.md). Die Bereinigung entfernt oder schreibt gefährliche Konstrukte um, statt die ganze Datei abzulehnen (Warnungen werden festgehalten).

### 6. Scanner für Theme-JS

Wird bei der Installation auf `theme.js` angewandt. Ein zutreffendes Muster führt zur Ablehnung der Installation. Die Liste der gesperrten Muster steht in [`docs/themes/security-model.md`](../themes/security-model.md).

### 7. Sandbox zur Laufzeit (Landlock / bubblewrap + seccomp + Ressourcenmonitor im Userspace)

> **Standard ist `landlock`** (`sandbox_mode=auto` löst im offiziellen
> Docker-Image dazu auf): eine Positivliste für das Dateisystem über Landlock
> LSM + `-restrict-net` (kein direkter IP-Ausgang für irgendeine Erweiterung) +
> die seccomp-Sperrliste von Kernel und `LD_PRELOAD` + `-deny-inet`, alles unter
> dem Standard-seccomp+AppArmor von Docker, ohne jede Änderung am Host. Die
> Übersicht steht in [`docs/plugins.md`](../plugins.md), die vollständige
> Tabelle je Modus in [`docs/SECURITY.md`](../SECURITY.md#sandbox-modes).

Die folgenden Einschlussschichten gelten je nach Fähigkeiten des Hosts. Der
Launcher (und das seccomp im Kernel, das er installiert) wird nur in den Modi
**`landlock`** und **`basic`** verwendet: `applySandbox` leitet
`bubblewrap`/`strict` auf einen eigenen bwrap-Codepfad, der den Launcher nie
aufruft, sodass diese beiden Modi sich stattdessen auf die eigene
Namespace-Isolation von bwrap stützen (plus den LD_PRELOAD-Shim in `strict`).

- **UID-Trennung** (`EVEY_PLUGINS_UID_SEPARATION`, Standard `auto`) - wenn
  aktiv, wechselt jede Erweiterung auf die UID 1001 (`evey-plugin`), getrennt
  von der UID 1000 des Servers, über `cap_setuid` auf dem **Launcher** (der
  Server bleibt ohne Privilegien). `auto` schaltet sie nur im Modus `basic` ein
  (ohne Landlock/bwrap), wo DAC die einzige Wand um die Geheimnisse ist; `on`
  erzwingt sie überall; `off` schaltet sie ab. Ist sie aktiv, sind `jwt.key`
  (`0600`) und `/data/{keys,logs,backups}` (nur für den Besitzer) für eine
  Erweiterung mit UID 1001 per DAC unlesbar, auch ohne Landlock.
- **seccomp-Sperrliste im Kernel (nur `landlock`/`basic`)** - von
  `evey-plugin-launcher` je Build-Architektur vor `execve` installiert, sodass
  sie **auch statische/kompilierte Erweiterungs-Binaries** einschränkt (die
  `libevey-sandbox.so` nie laden). Sperrt `ptrace`, `mount`, `unshare`, `setns`,
  `bpf`, `kexec`, das Laden von Modulen, die setuid-Familie, `capset`, den
  Keyring, Signale an PID 1 usw. Wird über `execve` an die Erweiterung und ihre
  Kinder vererbt. **In `bubblewrap`/`strict` gilt dieser Filter des Launchers
  nicht** - `strict` ergänzt nur den `LD_PRELOAD`-Shim `libevey-sandbox.so`
  (dieselbe Sperrliste, plus `execve`/`execveat`, damit eine interpretierte
  Erweiterung ohne `subprocess` keine Kinder starten kann), der nur für
  dynamische Erweiterungen gilt und statische Binaries nicht abdeckt.
- **`-deny-inet`** - für **jede** Erweiterung, ob mit Netzwerkberechtigung oder
  nicht, beantwortet der Launcher `socket(AF_INET/AF_INET6)` im Kernel mit
  EPERM: kein IP-Ausgang, auch auf Landlock-ABI <4. `AF_UNIX` bleibt für den
  gRPC-Socket von HostService erlaubt.
- **`-deny-fork`** - für jede Erweiterung **ohne** die Berechtigung
  `subprocess` wird das Erzeugen von Prozessen im Kernel mit EPERM beantwortet
  (`fork`, `vfork`, `clone` ohne `CLONE_THREAD`); das Erzeugen von Threads
  bleibt unberührt, sodass die Laufzeiten von node, Python und Go nicht
  betroffen sind. `clone3` liefert `ENOSYS` statt `EPERM`, damit glibc auf das
  prüfbare `clone(2)` zurückfällt, statt beim Erzeugen von Threads ganz zu
  scheitern.
- **`PR_SET_DUMPABLE=0` des Servers** - der Server markiert sich beim Start als
  nicht dumpbar, sodass eine Erweiterung mit derselben UID
  `/proc/<server>/{mem,environ,maps}` nicht lesen kann (den JWT-Schlüssel im
  Speicher, `DB_PASSWORD`, `MEILI_MASTER_KEY`).
- **Ressourcenmonitor im Userspace** - siehe unten; cgroups werden nicht
  benutzt.

Was das **Dateisystem-/Netzwerk-Backend** hinzufügt, hängt vom Modus ab:

| Modus | Einschluss im Dateisystem | Netzwerk | Gewählt, wenn |
|---|---|---|---|
| `landlock` | Positivliste über Landlock LSM (Systemverzeichnisse nur lesbar; Verzeichnis der Erweiterung + Socket-Verzeichnis beschreibbar; `/data` & Geschwister verweigert) | `-restrict-net` (jeder IP-Ausgang gekappt, kein Filter pro Port/Host) + `-deny-inet` | `sandbox_mode=landlock` (scheitert geschlossen, wenn nicht vorhanden); `auto` auf einem Kernel mit Landlock - der Normalfall des offiziellen Images |
| `bubblewrap` | Mount-Positivliste in abgetrennten Namespaces (siehe unten) | `--unshare-net` immer (jede Erweiterung, unabhängig von der Berechtigung `network`) | `sandbox_mode=bubblewrap`; `auto`, wo Landlock fehlt, die bwrap-Probe aber besteht (Bare Metal / privilegiert) |
| `strict` | wie `bubblewrap` + LD_PRELOAD-seccomp | wie `bubblewrap` | `sandbox_mode=strict` |
| `basic` | keiner (nur DAC; letzter Ausweg von `auto`) | nur `-deny-inet` | `sandbox_mode=basic`; `auto`, wenn weder Landlock noch bubblewrap nutzbar ist |

`auto` löst einmal beim ersten Start einer Erweiterung zu **landlock → bubblewrap
→ basic** auf und loggt den gewählten Modus. `basic` ist **nicht** „keine
Isolation“: Es wendet weiterhin UID-Trennung, das seccomp von Kernel +
LD_PRELOAD, `-deny-inet`, Landlock nach bestem Bemühen und den Ressourcenmonitor
an - es verliert nur den *Dateisystem-Einschluss pro Pfad*, wenn der Kernel kein
Landlock hat (eine Erweiterung kann dann nicht sensible, für alle lesbare
Dateien lesen, und die *Auflistung* der Prozesse in `/proc` ist sichtbar;
`/proc/<server>/{mem,environ}` bleiben geschützt).

Ist `sandbox_mode` auf `bubblewrap` oder `strict` gesetzt (per Opt-in über `evey.yaml`), läuft jeder Erweiterungsprozess in einem bubblewrap-Namespace mit:

**Namespace-Isolation (5 abgetrennt):**
- `--unshare-user` - eigener User-Namespace, UID-Zuordnung
- `--unshare-pid` - isolierter PID-Namespace
- `--unshare-uts` - isolierter Hostname
- `--unshare-ipc` - isoliertes IPC
- `--unshare-cgroup` - isolierter cgroup-Namespace
- `--unshare-net` - Netzwerk-Namespace **immer** abgetrennt, für jede Erweiterung unabhängig von der Berechtigung `network`; direkter Ausgang läuft über die `Fetch`-RPC des Hosts

**Mount-Positivliste:**

Die Sandbox bindet eine **ausgewählte Positivliste** nur lesbarer Systempfade ein, die zur Laufzeit gebraucht werden (Interpreter, gemeinsame Bibliotheken, SSL, DNS), und legt nie Datenpfade offen. Es gibt **kein** vollständiges Einbinden von `/`: Nur die Verzeichnisse unten werden eingehängt, sodass `/data`, `/var`, `/home`, `/root` und die Verzeichnisse benachbarter Erweiterungen im Namespace schlicht nicht existieren.

| Pfad | Zugriff | Begründung |
|---|---|---|
| `/usr`, `/bin`, `/sbin` | Nur lesbar | Interpreter und System-Binaries |
| `/lib`, `/lib64` | Nur lesbar (`--ro-bind-try`) | Gemeinsame Bibliotheken (fehlende Pfade je Distribution übersprungen) |
| `/etc/ssl`, `/etc/ca-certificates`, `/etc/resolv.conf`, `/etc/nsswitch.conf`, `/etc/{alpine,os}-release` | Nur lesbar (`--ro-bind-try`) | TLS-Vertrauensspeicher, DNS-Auflösung, Erkennung der Laufzeit. **Nicht** `/etc` als Ganzes: `/etc/passwd`, `/etc/shadow`, `/etc/ssh` werden nie eingehängt. |
| `<plugin_dir>` | Lesen/Schreiben | Das eigene Installationsverzeichnis der Erweiterung |
| `<plugin_sock_dir>` | Lesen/Schreiben | IPC-Socket-Verzeichnis pro Erweiterung (nach dem tmpfs von `/tmp` erneut eingebunden) |
| `/proc` | Proc-Mount | Von den Laufzeiten benötigt |
| `/dev` | Synthetisch (`--dev`) | Minimaler Gerätesatz (null, zero, full, random, urandom, tty, pts) - keine physischen Geräte |
| `/tmp` | tmpfs | Flüchtiger Arbeitsbereich; das `/tmp` des Hosts ist nicht sichtbar |

**Nicht** eingehängte Pfade: `/data`, `/var`, `/home`, `/root`, `/run`, `/etc/passwd`, `/etc/shadow`, `/etc/ssh`, die Verzeichnisse anderer Erweiterungen.

Quelle: `buildBwrapCmd` in `internal/adapters/driven/pluginrunner/runner_bwrap.go`.

**Ressourcenmonitor im Userspace** (pro Erweiterung):

Ressourcengrenzen werden von einem Aufräumer im Userspace durchgesetzt (nicht von cgroups, die in einem nicht privilegierten Docker-Container unbrauchbar sind). Der Monitor fragt etwa alle 2 s `/proc/<pid>/{statm,status,stat}` ab und beendet die Erweiterung, sobald eine Grenze überschritten ist. `nice` wird beim Start gesetzt, damit eine Erweiterung dem Server nie die CPU entzieht.

- Speichergrenze (RSS, Standard 256 MiB) - über `/proc/<pid>/statm`
- Grenze für PIDs/Threads (Standard 64) - über `/proc/<pid>/status`
- CPU-Priorität - über `nice` (Standard 10)
- Plattengrenze (Installationsverzeichnis, Standard 100 MiB) - über ein rekursives `filepath.Walk` etwa alle 30 s

Die Begründung für den Verzicht auf cgroups steht in [ADR-0013](../adr/0013-userspace-plugin-resource-monitor.md).

Hinweis: `--unshare-cgroup` in der Liste der bwrap-Namespaces unten erzeugt einen isolierten cgroup-_Namespace_ (die Erweiterung sieht eine frische cgroup-Hierarchie), setzt aber **keine** Ressourcengrenzen; diese kommen vom Monitor im Userspace oben.

Quelle: `internal/adapters/driven/pluginrunner/runner_landlock.go`, `runner_bwrap.go`, `runner_resmon.go`; `cmd/evey-plugin-launcher/{main.go,seccomp_linux.go,landlock_linux.go,droppriv_linux.go}`; `cmd/evey/hardening_linux.go`; `sandbox/sandbox.c`.

### 8. (abgeschafft) DNS-Positivliste

Die DNS-Positivliste über `/etc/hosts` aus V1, die in früheren Fassungen dieses
Dokuments beschrieben war, wurde **entfernt**. `runner_dns.go` existiert nicht
mehr; `buildBwrapCmd` bindet `/etc/hosts` nicht ein (siehe den Kommentar in
`internal/adapters/driven/pluginrunner/runner_bwrap.go`: „no /etc/hosts
whitelist is bind-mounted here anymore“). Ersetzt wurde sie durch einen
strengeren Mechanismus: Direkter IP-Ausgang ist für jede Erweiterung vollständig
gekappt (`-restrict-net` + `-deny-inet` in `landlock`/`basic`, `--unshare-net`
in `bubblewrap`/`strict`), und Erweiterungen mit Netzwerkberechtigung erreichen
das Netz nur über den `Fetch`-Vermittler des Hosts, der die Positivliste
`external_fetch` und einen SSRF-sicheren Verbindungsaufbau auf Host-Seite
durchsetzt (siehe [Netzwerkausgang der Erweiterungen](../SECURITY.md#plugin-network-egress-brokered-since-2026-07-11) in `docs/SECURITY.md`).

### 9. Authentifizierung über IPC-Peer-Credentials

Der Host stellt pro Erweiterung einen Unix-Socket unter `/tmp/evey/plugins/<plugin_id>/host.sock` bereit. Jeder eingehende gRPC-Aufruf wird authentifiziert, indem die PID des Gegenübers über `getsockopt(SO_PEERCRED)` gelesen wird - ein vom Kernel gelieferter Wert, der im Userspace nicht gefälscht werden kann. Die PID wird mit dem Wert verglichen, der direkt nach `cmd.Start` registriert wurde.

Folgen:
- Eine Erweiterung kann sich nicht als eine andere ausgeben: `x-plugin-id`-Header werden ignoriert; die Identität kommt vom lauschenden Socket.
- Eine Erweiterung, die per fork/exec ein Kind startet, kann Host-Aufrufe nicht über das Kind leiten (andere PID → `Unauthenticated`).

Quelle: `internal/adapters/driving/pluginhost/`.

### 10. Isolation des Frontends

Frontend-Module von Erweiterungen laufen in einem `<iframe>` mit:
- `sandbox="allow-scripts"` - kein Zugriff auf denselben Ursprung, kein Absenden von Formularen, keine Pop-ups
- CSP-`<meta>`-Tag im `srcdoc`: `default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src <external_fetch_hosts>; img-src data: blob:; font-src data:`
- auf Host-Seite erzwungener Ursprung für postMessage
- Ratenbegrenzung der Brücke: `toast` höchstens 3 Nachrichten/Sekunde, 200 Zeichen, kein HTML
- Positivliste der Pfade für `navigate`: `/admin/*`, `/auth/*`, `/setup/*`, `/profile/security/*` werden abgelehnt
- `resize` begrenzt: 50-800 px
- Positivliste der Slots sowohl serverseitig (Installation) als auch clientseitig (Darstellung) durchgesetzt

## Katalog der Berechtigungen

| Berechtigung | Was sie gewährt |
|---|---|
| `network` | Die Erweiterung darf ausgehende Netzwerkverbindungen aufbauen. **Muss** mit einer nicht leeren `external_fetch`-Liste kombiniert werden - ohne sie wird die Installation abgelehnt. |
| `subprocess` | Die Erweiterung darf Kindprozesse starten. Ohne sie greifen zwei Schichten: Der LD_PRELOAD-seccomp-Shim sperrt `execve`/`execveat` für interpretierte Erweiterungen, und das `-deny-fork` des Launchers sperrt das Erzeugen von Prozessen selbst (`fork`/`vfork`/`clone` ohne `CLONE_THREAD`) im Kernel. Threads funktionieren so oder so weiter. (Das seccomp des Launchers im Kernel verweigert `execve` nicht - er muss die Erweiterung ja starten -, aber jedes Kind erbt den Kernel-Filter + die Landlock-Domäne.) |
| `native-binary` | Die Erweiterung darf `.so`/`.dll`/`.exe`/`.wasm`/`.jar` oder eingebettete Archive mitliefern. Der Administrator sieht den undurchsichtigen Code in der Installationsvorschau und muss ihn annehmen. |
| `filesystem` | Angegebene Absicht, auf das Dateisystem über das eigene Verzeichnis der Erweiterung hinaus zuzugreifen. Bei der Prüfung durchgesetzt; noch keine harte Regel der Sandbox (siehe Bekannte Grenzen). |
| `media.read` | Die Erweiterung darf `MediaRead`-RPCs aufrufen: Metadaten von Mediendateien und Zugriff auf Streams lesen. |
| `media.write` | Die Erweiterung darf `MediaWrite`-RPCs aufrufen: Medieneinträge anlegen/ändern. |
| `api.library` | Die Erweiterung darf RPCs zur Bibliotheksverwaltung aufrufen: Bibliotheken auflisten/durchsuchen, Scans auslösen. |
| `api.user` | Die Erweiterung darf benutzerbezogene RPCs aufrufen (eigenes Profil, Einstellungen lesen). |
| `api.metadata` | Die Erweiterung darf Metadaten-RPCs aufrufen: suchen und Anreicherungsdaten schreiben. |
| `notification.sender` | Die Erweiterung darf Benachrichtigungsereignisse veröffentlichen (Web-Push, E-Mail, Webhook). |
| `webhook.receiver` | Die Erweiterung darf sich als Webhook-Ziel registrieren (empfängt HTTP-POST von externen Diensten). |
| `hardware.gpu` | **Berechtigung mit dem höchsten Risiko - nur für Administratoren, Stufe für Erweiterungen aus erster Hand.** Gewährt den DRM-Render-Knoten des Hosts (`-render-dev`, das echte `renderD1xx`, nie ein card-Knoten, nie `/dev/dxg`/`/dev/nvidia*`/`/dev/kfd`): eine Angriffsfläche auf einen Kernel-Treiber und eine GPU-Speichergrenze, die mit dem Transcoder GETEILT ist (Speicherleck zwischen Prozessen auf GPUs von AMD/Apple/Qualcomm möglich; eine hängende GPU kann die Transkodierung blockieren). Ohne diese Berechtigung sind GPU-Geräteknoten vollständig aus der Landlock-Positivliste der Geräte ausgeschlossen (verweigert unabhängig von DAC). Ist sie gewährt, fügt der Launcher eine ausdrückliche Landlock-Regel `IOCTL_DEV` genau für diese eine Datei hinzu (ABI v5+); auf älteren Landlock-ABIs werden ioctls von Landlock gar nicht geregelt, sodass Lese- und Schreibzugriff allein schon das Einreichen von GPU-Befehlen erlaubt. Das Installationsprogramm zeigt in der Vorschau eine laute CRITICAL-Warnung; gewähren Sie sie nur vertrauenswürdigen Erweiterungen aus erster Hand. |

## Regeln für externe Abrufe

Angegeben in `external_fetch: [...]` im Manifest und bei der Installation von
`validateExternalFetch` (`internal/adapters/driven/pluginrunner/manifest.go`)
geprüft. Alles oder nichts: Ein ungültiger Eintrag lässt die ganze Installation
scheitern.

Ein Eintrag ist ein **nackter Hostname**. Ein Schema-Präfix wird geduldet und
verworfen - nur der Hostname bleibt.

| Eintrag | Gültig | Grund |
|---|---|---|
| `api.tmdb.org` | Ja | Exakter Hostname, die normale Form |
| `https://api.tmdb.org` | Ja | Schema gelesen und verworfen, Hostname behalten |
| `*.tmdb.org` | Nein | Platzhalter werden abgelehnt - führen Sie jede nötige Subdomain auf |
| `*` | Nein | Dieselbe Regel |
| `192.168.1.1`, `::1` | Nein | IP-Literale werden abgelehnt, beide Familien |

Zur Laufzeit fügt der Vermittler des Hosts eigene Grenzen hinzu, die das
Manifest nicht erweitern kann: nur `http` und `https`, nur `GET` und `HEAD`,
der Zielhost wird ohne Beachtung der Groß-/Kleinschreibung mit Ihrer Liste
abgeglichen, und private oder reservierte Adressen werden nach der
DNS-Auflösung abgelehnt - Weiterleitungen werden gegen beide Regeln erneut
geprüft.

> Der Abgleich des Vermittlers versteht auch eine Form `*.example.com`, aber das
> Installationsprogramm lehnt einen solchen Eintrag ab, sodass er ihn nie
> erreicht. Schreiben Sie ausdrückliche Hostnamen.

## Positivliste der Slots

Erweiterungen dürfen Oberfläche nur in ausdrücklich erlaubte Slots einfügen. Kritische Pfade fehlen absichtlich.

| Slot | Anmeldung nötig | Hinweise |
|---|---|---|
| `home.section` | Ja | Inhaltsbereich der Hauptseite |
| `home.hero` | Ja | Bereich des Hero-Banners |
| `library.sidebar` | Ja | Seitenleiste der Bibliotheksansicht |
| `media.detail.sidebar` | Ja | Seitenleiste der Mediendetails |
| `player.overlay` | Ja | Überlagerung im Player |
| `settings.user.appearance` | Ja | Abschnitt Darstellung in den Benutzereinstellungen |
| `sidebar.bottom` | Ja | Unterer Bereich der Navigationsleiste |
| `admin.nav` | Ja | Ein Menüeintrag im Administrationsbereich, der die eigene abgeschottete Seite der Erweiterung öffnet - fügt nichts in bestehende Verwaltungsbildschirme ein |

**Nie erlaubt:** `admin.*` (außer `admin.nav`), `login.*`, `setup.*`, `profile.security.*` - diese Pfade sind für Anmeldung und Privilegien kritisch, und eine Einfügung durch Erweiterungen dort wäre ein Weg zur Rechteausweitung oder zum Phishing.

Auch der Inhalt von `admin.nav` ist abgeschottet wie jeder andere Slot: Er wird in einem `<iframe sandbox="allow-scripts">` (ohne `allow-same-origin`) mit CSP `default-src 'none'` dargestellt, ist also Inhalt von Dritten, der im Administrationsbereich angezeigt wird, keine Verwaltungsfähigkeit, die der Erweiterung gewährt wird.

Quelle: `internal/domain/plugin/slots.go`.

## Ablauf eines Updates

1. Der Administrator lädt eine neue ZIP-Datei hoch → `PreviewPluginUpdate` führt die statische Analyse + die Cosign-Prüfung aus und liefert einen Vergleich der geänderten Berechtigungen/Slots/`external_fetch` gegenüber der installierten Version.
2. Ist der Vergleich nicht leer (neue Berechtigungen, neue Slots, geänderte externe Hosts), muss der Administrator ausdrücklich bestätigen, bevor das Update angewandt wird (`requires_ack = true`).
3. Der Hash der Quelle wird bei der Vorschau erfasst und beim Anwenden geprüft (Schutz gegen TOCTOU: Die ZIP-Datei, die analysiert wurde, ist die, die installiert wird).
4. Steht die Cosign-Richtlinie auf `enforce`, blockiert eine nicht passende Signatur beim Anwenden das Update, auch wenn die Vorschau bestanden wurde.

## Bekannte Grenzen des Berechtigungsmodells

| Grenze | Auswirkung | Heutige Abschwächung |
|---|---|---|
| `auto` kann auf einem Host ohne Landlock und ohne nutzbares bubblewrap zu `basic` auflösen | Kein Dateisystem-Einschluss **pro Pfad**: Eine Erweiterung kann nicht sensible, für alle lesbare Dateien lesen, und die Prozessliste in `/proc` ist sichtbar | Die Geheimnisse des Servers bleiben geschützt (UID-Trennung + `PR_SET_DUMPABLE=0`); seccomp im Kernel (auch für statische Binaries) + `-deny-inet` gelten weiter; der aufgelöste Modus wird beim ersten Start einer Erweiterung geloggt |
| Alle Erweiterungen teilen sich die UID 1001 | Eine bösartige Erweiterung könnte ihren Geschwistern Signale schicken (Signale an PID 1 / den Server sperrt seccomp) | `PR_SET_DUMPABLE=0` sperrt prozessübergreifende Lesezugriffe auf `/proc/<pid>/{environ,mem,maps,fd}`; Landlock / bubblewrap isolieren die Verzeichnisse der Erweiterungen vollständig voneinander |
| Der `Fetch`-Vermittler kontrolliert, *wohin* ein ausgehender Aufruf gehen kann, nicht *was* darin hinausgeht | Steht ein Host einmal auf der Positivliste `external_fetch`, kann der Vermittler den Inhalt nicht prüfen, ein Abfluss an einen erlaubten Host bleibt also möglich | Administratoren sollten nur Hosts erlauben, denen sie Daten der Erweiterung anvertrauen; der Vermittler sperrt weiterhin nicht erlaubte Ziele und private/reservierte IPs |
| Der LD_PRELOAD-seccomp-Shim schränkt nur dynamische (interpretierte) Erweiterungen ein | Ein statisch gelinktes Erweiterungs-Binary im Modus `basic` ohne vorhandenen Launcher umgeht die Sperrliste des Shims vollständig | Das offizielle Image liefert den Launcher immer mit (seccomp im Kernel deckt statische Binaries in `landlock`/`basic` ab); Entwicklungs-Images ohne Launcher-Binary sollten keine unvertrauten Erweiterungen ausführen |
| Regex-Analysator durch fortgeschrittene Verschleierung umgehbar | Ein entschlossener Angreifer kann Muster verstecken | Vorschau durch den Administrator + Prüfung der Berechtigungen; die Grenze der Durchsetzung sind Sandbox + Ressourcenmonitor + IPC-Peer-Credentials, nicht der Analysator |

## Was V2 hinzufügen wird

- Eine nftables-Firewall für den Ausgang auf Container-Ebene als unabhängige Schicht hinter dem `Fetch`-Vermittler (Härtung für Betreiber, keine Lücke der Erweiterungs-Sandbox - direkter IP-Ausgang ist schon für jede Erweiterung vollständig gekappt)
- Ein Egress-Proxy für eine vollständige Durchsetzung auf Ebene der Anfragen
- Ein seccomp-Positivlisten-Modus (strenge Sandbox: nur ein minimaler Satz von Systemaufrufen erlaubt)
- Abdeckung von Platzhalter-Einträgen in `external_fetch` durch Durchsetzung pro Anfrage
