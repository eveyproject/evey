---
source_sha: 0ac15ab64c61bb43ae41b6ec652dda6261a8eed8431ec891b4f9ad3b33dd7d21
---
# Sandbox und Bedrohungsmodell für Erweiterungen

> **Welche Seite suchen Sie?** Diese hier beschreibt, was der *Server* tut, um
> eine Erweiterung einzusperren: die Schichten, die Sandbox-Modi, was jeder
> davon noch nicht verhindern kann, und wie man sie einstellt. Wenn Sie eine
> Erweiterung schreiben und wissen wollen, was Sie angeben dürfen und was
> abgelehnt wird, lesen Sie stattdessen
> [das Sicherheitsmodell für Erweiterungen](plugins/security-model.md). Beide
> Seiten behandeln dasselbe Thema von entgegengesetzten Enden und wiederholen
> sich bewusst nicht.

Dieses Dokument beschreibt die Verteidigungsschichten, die Evey auf Erweiterungen von Dritten anwendet, die Grenzen dieser Verteidigung und den Fahrplan zu stärkerer Isolation.

## Bedrohungsmodell: was eine feindliche Erweiterung versuchen würde

Evey-Erweiterungen laufen als eigene Betriebssystemprozesse, die über Unix-Sockets gRPC mit dem Host austauschen. Der Host vertraut standardmäßig keinem Erweiterungscode. Die Risiken, gegen die wir uns wehren, nach Schwere geordnet:

1. **Identitätsvortäuschung bei Host-RPCs** - eine bösartige Erweiterung gibt sich als eine andere aus, um deren Berechtigungen zu bekommen.
2. **Rechteausweitung** - eine Erweiterung bricht aus ihrer Sandbox aus und führt beliebige Befehle als Serverbenutzer aus.
3. **Lesen / Schreiben im Dateisystem** - eine Erweiterung liest oder schreibt Pfade außerhalb ihres Installationsverzeichnisses.
4. **Abfluss über das Netzwerk** - eine Erweiterung öffnet ausgehende Verbindungen, um Daten nach Hause zu schicken.
5. **Ressourcenmissbrauch** - Krypto-Mining, Fork-Bombe, Erschöpfung der Datei-Deskriptoren.
6. **Lieferkette** - der Autor einer Erweiterung liefert verschleierte oder kompilierte Schadsoftware, die der statische Analysator nicht einsehen kann.

## Verteidigungsschichten (aktuell)

### 1. Statische Analyse bei der Installation (VORSCHAU + INSTALLATION)

Jede ZIP-Datei einer Erweiterung wird in ein temporäres Verzeichnis entpackt und von [`pluginrunner.AnalyzePlugin`](../internal/adapters/driven/pluginrunner/analyzer.go) durchlaufen. Geprüfte Kategorien mit ihrer Schwere:

| Kategorie | Schwere | Beispiele |
|---|---|---|
| `shell_exec` | CRITICAL | `os.system`, `subprocess.run`, `exec()`, `child_process.exec`, PHP `system()`/`passthru()`/`shell_exec()`/`proc_open()`, `preg_replace /e`, Ruby Backtick/`Kernel.exec`/`open('|cmd')` |
| `network` | WARNING / CRITICAL | `requests.get`, `fetch`, Reverse-/Bind-Shells |
| `filesystem` | WARNING / CRITICAL | `/etc/passwd`, `.ssh/`, `rm -rf`, Operationen auf Symlinks |
| `obfuscation` | CRITICAL | `eval`, `exec`, Base64-Dekodierung, Hex-Folgen, `__builtins__`, PHP `assert(base64_decode(...))`, Shell `base64 -d \| bash`, `echo <blob> \| openssl` |
| `privilege` | CRITICAL | `setuid`, `chmod 4777`, `sudo`, Signal an PID 1 |
| `cryptomining` | CRITICAL | `stratum+tcp`, `xmrig`, `cryptonight` |
| `runtime_install` | CRITICAL | `pip install`, `npm install`, `curl ... \| sh` |
| `dynamic_import` | CRITICAL | `__import__()`, `importlib.import_module` |
| `sandbox_escape` | CRITICAL | `vm.runIn*`, `process.binding`, `new Function`, Python `__subclasses__()`, `__class__.__bases__`, Durchlaufen der Klassenhierarchie über `__mro__` |
| `deserialization` | CRITICAL / WARNING | `pickle.loads`/`pickle.load`, `marshal.loads`, `__reduce__`/`__reduce_ex__`; WARNING: `yaml.load()` ohne ausdrücklichen Loader |
| `native_code` | CRITICAL / WARNING | `ctypes.CDLL`, `ffi.Library`; WARNING: Go `unsafe.Pointer`, `unsafe.Slice`, `unsafe.SliceData`, `unsafe.StringData` |
| `compiled_binary` | WARNING → BLOCK | `.so`, `.dll`, `.exe`, `.wasm`, `.jar` (für den Scanner undurchsichtig) |
| `embedded_archive` | WARNING → BLOCK | `.zip`, `.tar.gz` (entpackbarer Code) |
| `mime_mismatch` | WARNING (wie kritisch behandelt) | die Dateiendung behauptet Bild, der Inhalt ist etwas anderes |
| `suspicious_url` | WARNING | Nicht-HTTP-Schema in einem URL-Literal |
| `unvendored_deps` | WARNING | `requirements.txt`, `package.json` vorhanden |

CRITICAL-Funde oder `mime_mismatch` blockieren die Installation (`Safe = false`).

`compiled_binary` und `embedded_archive` blockieren die Installation **standardmäßig**, sofern das Manifest nicht die Berechtigung `native-binary` angibt. Der Administrator sieht im Vorschau-Schritt sowohl die Warnung als auch die verlangte Berechtigung, bevor er zustimmt.

### 2. Sperre über die Berechtigungen im Manifest

Die `plugin.yaml` einer Erweiterung gibt die Berechtigungen an, die sie verlangt. Der Runner setzt sie auf zwei Ebenen durch:

- **`subprocess`** - darf Kindprozesse starten. Ohne sie verweigert der LD_PRELOAD-seccomp-Shim `execve`/`execveat` für interpretierte Erweiterungen. (Das seccomp des Launchers im Kernel verweigert `execve` *nicht* - er muss die Erweiterung ja starten -, aber Kinder erben den Filter + die Landlock-Domäne trotzdem.)
- **`native-binary`** - nötig, um `.so` / `.dll` / `.exe` / Archive mitzuliefern. Der Administrator muss den undurchsichtigen Code bei der Installation annehmen.
- **API-Berechtigungen** (`api.library`, `api.user`, `api.metadata`, `media.read`, `media.write`, `notification.sender`) - bei jeder Host-RPC von [`PluginPermissionInterceptor`](../internal/adapters/driving/grpcapi/plugin_interceptor.go) geprüft.

### 3. UID-Trennung (DAC)

Ein Erweiterungsprozess kann auf die UID 1001 (`evey-plugin`) herabgestuft werden, getrennt von der UID des Servers (1000, `evey`), sodass die Geheimnisse des Servers für ihn schon per einfachem DAC unlesbar sind. Gesteuert wird das über `EVEY_PLUGINS_UID_SEPARATION` (`auto` | `on` | `off`, Standard `auto`):

- `auto` - Herabstufung **nur, wenn die Sandbox zu `basic` auflöst** (kein Dateisystem-Einschluss über Landlock/bubblewrap) - der einzige Fall, in dem DAC das Einzige ist, was die Geheimnisse schützt. Unter landlock/bubblewrap übersprungen, wo sie redundant ist.
- `on` - immer (eine zusätzliche, unabhängige Schicht über Landlock/bubblewrap).
- `off` - nie.

**Die Herabstufung führt der Launcher aus, nicht der Server.** Der Server bleibt ohne Root und ohne Capabilities; `evey-plugin-launcher` trägt `cap_setuid` (eine Datei-Capability, `setcap cap_setuid+ep` im `Dockerfile`) und führt direkt vor dem exec `setgid`/`setuid` auf die gemeinsame Gruppe + UID 1001 aus. Er lehnt jede Ziel-UID unter 1000 ab. Kann er nicht herabstufen (keine Capability, gesetztes `no_new_privs` oder fehlendes Launcher-Binary), hängt das Ergebnis vom Modus ab: in `basic`, wo die UID die einzige Wand um die Geheimnisse ist, wird die Erweiterung **abgelehnt** (scheitert geschlossen); unter `landlock`, wo das Dateisystem schon abgeriegelt ist, warnt er und startet die Erweiterung unter der UID des Servers. Ein als root laufender Server stuft Erweiterungen direkt über `SysProcAttr` herab und braucht keine Capability. Anleitung in einfachen Worten: [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md).

Was das bringt, **auch auf einem Kernel ohne Landlock** (wenn aktiv):

- Die Geheimnisse des Servers sind per DAC vor einer Erweiterung mit UID 1001 geschützt: `jwt.key` (`0600`) und die Sicherungen (Verzeichnis `0700` + Dateien `0600`) werden **zur Laufzeit** nur für den Besitzer lesbar gemacht, sodass der Schutz auch auf einem eingebundenen `/data` gilt; eine Erweiterung mit UID 1001 kann sie nicht lesen. (Das Image setzt außerdem `chmod 700` auf `/data/{keys,logs,backups}`, aber ein vom Host eingebundenes `/data` behält seine eigenen Rechte - garantiert sind die Laufzeit-Modi oben.)
- Zusammen mit `PR_SET_DUMPABLE=0` auf dem Server (siehe [§8](#8-server-process-hardening-pr_set_dumpable0)) sind der JWT-Schlüssel im Speicher und die Geheimnisse der Umgebung (`DB_PASSWORD`, `MEILI_MASTER_KEY`) über `/proc/<server>/{mem,environ,maps}` unlesbar.

Alle Erweiterungen teilen sich die UID 1001, sind also durch DAC allein nicht *voneinander* isoliert - genau das fügt die Schicht des Dateisystem-Einschlusses (Landlock / bubblewrap) hinzu. Unter `basic` + UID-Trennung könnte eine bösartige Erweiterung die Dateien einer Geschwister-Erweiterung lesen oder stören; Erweiterungen ohne Landlock voneinander zu isolieren, bräuchte eine UID *pro Erweiterung*. Siehe [§4](#4-syscall-confinement-kernel-seccomp--ld_preload), die Tabelle je Modus unten und [Bekannte Grenzen](#known-limits-of-the-sandbox).

### 4. Einschluss der Systemaufrufe (seccomp im Kernel + LD_PRELOAD)

Zwei seccomp-Filter wenden fast dieselbe Sperrliste an (sie unterscheiden sich nur bei den Signal-Systemaufrufen - siehe Tabelle); beide rufen zuerst `prctl(PR_SET_NO_NEW_PRIVS)` auf und installieren dann ein BPF-Programm, das die gesperrten Systemaufrufe mit EPERM beantwortet und alles andere erlaubt.

1. **Im Kernel angewandt (die Hauptschicht).** Der Shim [`evey-plugin-launcher`](../cmd/evey-plugin-launcher) wird dem argv der Erweiterung vorangestellt und installiert den BPF-Filter ([`seccomp_linux.go`](../cmd/evey-plugin-launcher/seccomp_linux.go)) **direkt vor `execve`**, je Build-Architektur. Der Filter wird über `execve` an die Erweiterung und an jedes Kind vererbt, das sie später startet. Weil er vom Launcher und nicht über `LD_PRELOAD` installiert wird, **schränkt er auch statische/kompilierte Erweiterungs-Binaries ein** - die `libevey-sandbox.so` nie laden und den Shim früher vollständig umgingen. Eine abweichende Architektur im ABI-Feld von `seccomp_data` führt zu `SECCOMP_RET_KILL_PROCESS` (vereitelt die Verwechslung von Systemaufrufnummern zwischen i386/x32).
2. **LD_PRELOAD (doppelte Absicherung für dynamische Erweiterungen).** [`sandbox/sandbox.c`](../sandbox/sandbox.c) ist ein kleiner Konstruktor, der interpretierten Erweiterungen vorgeladen wird; dieselbe Sperrliste, und zusätzlich setzt er `PR_SET_DUMPABLE=0` für die Erweiterung selbst.

**Welcher der beiden läuft, hängt vom Sandbox-Modus ab** (siehe Tabelle je Modus unten): Der Kernel-Filter des Launchers wird in `landlock` und `basic` angewandt, der LD_PRELOAD-Shim in `strict` und `basic`. `bubblewrap`/`strict` starten den Launcher **nicht**, sodass ein **statisches** Erweiterungs-Binary unter reinem `bubblewrap` überhaupt keine Sperrliste für Systemaufrufe bekommt - sein Einschluss dort sind die bwrap-Namespaces + die Dateisystem-Isolation, nicht seccomp.

Gesperrte Systemaufrufe, nach Kategorie gruppiert:

| Kategorie | Gesperrte Systemaufrufe |
|---|---|
| Prozesssteuerung / Debug | `ptrace`, `process_vm_readv`, `process_vm_writev` |
| Prozess-Signale | `kill` (beide Filter); der Launcher verweigert außerdem `tgkill` ganz, während der LD_PRELOAD-Shim `tkill`/`tgkill` nur verweigert, wenn sie auf PID 1 (den Server) zielen |
| Einhängen von Dateisystemen | `mount`, `umount2`, `pivot_root`, `chroot` |
| Namespaces | `unshare`, `setns` |
| Kernelmodule | `init_module`, `finit_module`, `delete_module` |
| BPF / perf | `bpf`, `perf_event_open` |
| Hardware-E/A | `ioperm`, `iopl` (nur LD_PRELOAD-Shim) |
| Swap / Speicherverwaltung | `swapon`, `swapoff` |
| Systemsteuerung | `syslog`, `reboot`, `settimeofday`, `clock_settime`, `clock_adjtime`, `adjtimex` |
| Kexec | `kexec_load` (beide Filter); `kexec_file_load` (nur LD_PRELOAD-Shim) |
| setuid-Familie | `setuid`, `setgid`, `setreuid`, `setregid`, `setresuid`, `setresgid` |
| Capabilities | `capset` |
| Keyring | `add_key`, `request_key`, `keyctl` |

`execve`/`execveat` wird **im LD_PRELOAD-Shim verweigert** (damit eine interpretierte Erweiterung ohne `subprocess` keine Kinder starten kann), **aber NICHT im Kernel-Filter des Launchers** - der Launcher muss die Erweiterung ja selbst per `execve` starten. Gestartete Kinder erben den Filter des Launchers + die Landlock-Domäne und bleiben so oder so eingeschlossen.

**Kein IP-Ausgang für ALLE Erweiterungen (seit 2026-07-11).** Jede Erweiterung - mit oder ohne Netzwerkberechtigung - bekommt `-deny-inet` (landlock/basic) oder `--unshare-net` (bubblewrap): Der Erweiterungsprozess kann überhaupt kein `socket(AF_INET)` / `socket(AF_INET6)` öffnen. Eine Erweiterung, die das Netz braucht, verbindet sich NICHT direkt; sie ruft den `Fetch`-Vermittler des Hosts über ihren `AF_UNIX`-Socket von HostService auf (der erlaubt bleibt), und dieser setzt auf Host-Seite die Positivliste `external_fetch` und einen SSRF-sicheren Verbindungsaufbau durch. Das schließt das Loch des Ausgangs über IP-Literale (z. B. `169.254.169.254`), das eine DNS-Positivliste pro Host und ein Kernel-Filter pro Port nicht schließen konnten. Siehe [ADR-0014](adr/0014-landlock-plugin-sandbox.md).

Grenzen - bitte genau lesen:

- **Sperrliste, keine Positivliste.** `open`, `read`, `write`, `connect`, `socket`, `unlink`, `mmap`, `mprotect`, `clone` usw. sind alle erlaubt. (Der Modus `strict` legt die LD_PRELOAD-Schicht über bubblewrap; eine echte Positivliste der Systemaufrufe steht noch auf dem Fahrplan.)
- **Die Dateisystem-Isolation kommt aus einer anderen Schicht.** seccomp schränkt keine Pfade ein; das tut Landlock (oder bubblewrap). Auf einem Kernel ohne Landlock und ohne bubblewrap kann die Erweiterung jede für alle lesbare Datei lesen (aber nicht die Geheimnisse des Servers - die sind per DAC geschützt, siehe §3).
- **Netzwerk-Einschluss.** Direkter IP-Ausgang ist für jede Erweiterung gekappt, mit oder ohne Netzwerkberechtigung: `-restrict-net` (keine `-net-connect`-Freigaben) plus `-deny-inet` sperren alle `AF_INET`/`AF_INET6`-Sockets im Kernel. Einen Kernel-Filter pro Port oder Host gibt es nicht. Eine Erweiterung mit Netzwerkberechtigung erreicht das Netz nur über den `Fetch`-Vermittler des Hosts über ihren `AF_UNIX`-Socket, der die Positivliste `external_fetch` auf Host-Seite durchsetzt.
- **Scheitert geschlossen für interpretierte Erweiterungen.** Findet der Runner `libevey-sandbox.so` nicht und fehlt der Erweiterung `subprocess`, weigert sich [`Runner.Start`](../internal/adapters/driven/pluginrunner/runner.go), die Erweiterung zu starten, und sendet ein Ereignis `plugin.error`. Das seccomp des Launchers im Kernel ist nach bestem Bemühen (ein Kernel ohne seccomp fällt auf die anderen Schichten zurück).

### 5. gRPC-Socket des Hosts pro Erweiterung + Authentifizierung über SO_PEERCRED

Der Host stellt pro Erweiterung einen Unix-Socket unter `/tmp/evey/plugins/<plugin_id>/host.sock` bereit. Jeder Socket wird von einem eigenen gRPC-Server mit einem Interceptor bedient, der:

1. die PID des Gegenübers über `getsockopt(SO_PEERCRED)` liest - **diesen Wert liefert der Kernel, er lässt sich im Userspace nicht fälschen**.
2. sie mit der PID vergleicht, die der Runner direkt nach `cmd.Start` mit `SetExpectedPID` registriert hat.
3. die ID der Erweiterung über eine Closure in den gRPC-Kontext stempelt (die ID stammt aus der Identität des lauschenden Sockets, nie aus dem Datenstrom).

Wirkung:

- Eine Erweiterung kann sich nicht als eine andere ausgeben, indem sie einen Header `x-plugin-id` schickt (der Header wird ignoriert).
- Eine Erweiterung kann den Host-Socket einer anderen nicht erreichen und deren RPCs aufrufen - selbst wenn sie `connect()` aufruft, lehnt die PID-Prüfung mit `Unauthenticated: PID mismatch` ab.
- Eine Erweiterung, die per fork/exec ein Kind startet, kann Host-Aufrufe nicht über das Kind leiten (andere PID → abgelehnt). Das ist Absicht und passt zur Sperre über die Berechtigung `subprocess`.

### 6. Sperre über die Vorschau durch den Administrator

`POST /api/v1/admin/plugins/preview` führt den statischen Analysator aus und liefert Manifest + Funde, ohne zu installieren. Der Administrator sieht:

- Die verlangten Berechtigungen (einschließlich `native-binary`, `subprocess`, `network`, `api.*`).
- Alle Funde mit Schwere, Datei, Zeile und Kategorie.
- Die entnommenen URLs, nach Art gruppiert (Literal, Vorlage, Verkettung).
- Den SHA256 der Quelle (entspricht der kommenden Installation).

Der Administrator ist die Vertrauensgrenze für erhöhte Berechtigungen - Evey zeigt die Angriffsfläche, der Administrator nimmt an oder lehnt ab.

### 7. Benachrichtigung bei einem Absturz der Erweiterung

Der Runner veröffentlicht ein Domänenereignis `plugin.error`, wenn:

- die Sandbox-Bibliothek fehlt und der Erweiterung `subprocess` fehlt.
- der Erweiterungsprozess mit einem Code ungleich null endet.
- der Auth-Interceptor einen Versuch mit falscher PID loggt (nur slog auf Warnstufe - noch nicht an den Bus angebunden).

Der Benachrichtigungsdienst abonniert das Ereignis und alarmiert den Administrator über den eingestellten Kanal (Web-Push / E-Mail / Webhook).

### 8. Härtung des Serverprozesses (`PR_SET_DUMPABLE=0`)

Beim Start ruft der Server `prctl(PR_SET_DUMPABLE, 0)` auf ([`cmd/evey/hardening_linux.go`](../cmd/evey/hardening_linux.go)). Das stellt den Besitzer von `/proc/<server>/{mem,environ,maps,fd,…}` auf root um, sodass ein Prozess mit der UID des Servers sie nicht lesen kann. Am wichtigsten ist das im Rückfall ohne Landlock (z. B. ein alter Synology-Kernel), wo eine Erweiterung sonst den JWT-Schlüssel im Speicher des Servers und seine Geheimnisse der Umgebung (`DB_PASSWORD`, `MEILI_MASTER_KEY`) über `/proc` abgreifen könnte. Auf Kerneln mit Landlock wird der Erweiterung `/proc` des Servers schon verweigert; dies ist die doppelte Absicherung über alle Kernel hinweg. Es beeinträchtigt nicht, dass der Server seine eigenen Dateien liest; ein Fehlschlag wird geloggt, ist aber nicht fatal.

## Sandbox-Modi

`sandbox_mode` wird über `evey.yaml` oder `EVEY_PLUGINS_SANDBOX_MODE` eingestellt (Standard `auto`). Es wählt das **Backend** des Einschlusses, und der Mechanismus für Systemaufrufe/Ausgang hängt vom Backend ab: `landlock` und `basic` starten die Erweiterung über den Launcher, der die seccomp-Sperrliste im Kernel + `-deny-inet` installiert; `bubblewrap`/`strict` isolieren das Netz stattdessen mit einem Namespace (`--unshare-net`) und haben kein Launcher-seccomp (`strict` fügt nur den LD_PRELOAD-Shim hinzu, der nur für dynamische Erweiterungen gilt). Was in JEDEM Modus gilt: UID-Trennung, der Ressourcenmonitor im Userspace und die Tatsache, dass **direkter IP-Ausgang gekappt ist** (durch `-deny-inet` oder `--unshare-net`), sodass eine Netzwerk-Erweiterung nur über den `Fetch`-Vermittler des Hosts hinausreicht. Der Runner wählt das Backend in [`applySandbox`](../internal/adapters/driven/pluginrunner/runner.go); `auto` löst einmal beim ersten Start einer Erweiterung auf ([`resolveAutoMode`](../internal/adapters/driven/pluginrunner/runner_landlock.go)).

### `auto` (Standard)

Löst das beste Backend auf, das der Host tatsächlich unterstützt, in dieser Reihenfolge: **landlock → bubblewrap → basic**.

- Bevorzugt **landlock**: Es braucht keine Privilegien, läuft unter dem Standard-seccomp + AppArmor von Docker ohne jede Änderung am Host und lässt sich nach dem Anwenden nicht mehr lockern - die richtige Voreinstellung für die hauptsächliche Bereitstellung im Container.
- Fällt nur dann auf **bubblewrap** zurück, wenn Landlock fehlt *und* eine Wegwerf-bwrap-Sandbox sich tatsächlich aufbauen lässt ([`ProbeSandbox`](../internal/adapters/driven/pluginrunner/sandbox_probe.go) - Bare Metal / privilegierter Container mit nutzbaren User-Namespaces + mount).
- Sonst **basic**. Der aufgelöste Modus wird beim ersten Start einer Erweiterung geloggt.

### `landlock`

- **Schließt ein:** das Dateisystem über eine Positivliste mit Landlock LSM, die der Launcher anwendet - Systemverzeichnisse (`/usr`, `/lib`, `/lib64`, `/bin`, `/sbin`, `/etc`, `/app`, `/dev`) nur lesen/ausführen; das eigene Installationsverzeichnis der Erweiterung + ihr Socket-Verzeichnis lesen/schreiben; alles andere (insbesondere `/data`, die Verzeichnisse benachbarter Erweiterungen) verweigert. Netzwerk: **kein direkter IP-Ausgang für irgendeine Erweiterung** - `-restrict-net` (keine `-net-connect`-Freigabe) plus `-deny-inet` sperren alle `AF_INET`/`AF_INET6`-Sockets; Erweiterungen mit Netzwerkberechtigung erreichen das Netz nur über den `Fetch`-Vermittler des Hosts über ihren `AF_UNIX`-Socket. Obendrein: die seccomp-Sperrliste von Kernel + LD_PRELOAD.
- **Verlangt:** Kernel ≥ 5.13 mit Landlock in der aktiven LSM-Liste und das Binary `evey-plugin-launcher`. Kein mount, kein unshare, keine Privilegien - läuft sauber unter dem Standard-Sicherheitsprofil von Docker.
- **Gewählt, wenn:** `sandbox_mode=landlock` (die Erweiterung scheitert geschlossen, wenn Landlock nicht durchgesetzt werden kann) oder `auto` dazu auflöst (der Normalfall des offiziellen Images).
- **Grenzen:** Direkter IP-Ausgang ist für jede Erweiterung gekappt (`-deny-inet` gilt auch auf Landlock-ABI <4); die Positivliste pro Host lebt jetzt auf Host-Seite im `Fetch`-Vermittler, nicht im Kernel. Der Dateisystem-Einschluss gilt so oder so.

### `bubblewrap`

- **Schließt ein:** Dateisystem + Netzwerk + Prozesssicht über 6 abgetrennte Namespaces (`--unshare-user/-pid/-uts/-ipc/-cgroup/-net` - `--unshare-net` wird bedingungslos auf **jede** Erweiterung angewandt; direkter Ausgang läuft über den Vermittler) und eine Mount-Positivliste - Systemverzeichnisse nur lesbar, ein beschreibbares Erweiterungs- + Socket-Verzeichnis, ein synthetisches `/proc` und ein minimales `/dev`, ein tmpfs-`/tmp`; `/data`, `/var`, `/home`, `/root` und die Nachbarverzeichnisse werden schlicht nicht eingehängt. `applySandbox` leitet diesen Modus an `applyBwrapSandbox`, der den Launcher nie aufruft: Das seccomp des Launchers im Kernel gilt hier **nicht**, und der LD_PRELOAD-Shim auch nicht (der ist nur in `strict`). Der Einschluss der Systemaufrufe kommt allein aus den abgetrennten Namespaces.
- **Verlangt:** das Binary `bwrap` UND die Systemaufrufe `mount`/`unshare`; in Docker braucht das gelockerte Sicherheit, ist also praktisch nur auf Bare Metal (oder in einem privilegierten Container) möglich.
- **Gewählt, wenn:** `sandbox_mode=bubblewrap` oder `auto` auf einem Host ohne Landlock, auf dem die bwrap-Probe besteht.
- **Grenzen:** `--unshare-cgroup` ist ein cgroup-*Namespace*, keine Ressourcengrenze (die kommen vom Monitor im Userspace). In diesem Modus gilt keine seccomp-Sperrliste im Kernel (siehe oben); `-deny-inet` wird auch nicht benutzt, da `--unshare-net` bereits alle IP-Sockets sperrt.

### `strict`

- Wie `bubblewrap` **plus** die LD_PRELOAD-seccomp-Bibliothek darüber. Dieselben Voraussetzungen und Grenzen wie bubblewrap. (Eine echte *Positivliste* der Systemaufrufe steht noch auf dem Fahrplan.)

### `basic`

- **Schließt ein:** UID-Wechsel auf 1001 + die seccomp-Sperrliste im Kernel + `-deny-inet` + Landlock nach bestem Bemühen - alles geliefert, indem die Erweiterung mit dem Launcher umhüllt wird, sofern das Launcher-Binary vorhanden ist (im offiziellen Image ist es das). Fehlt der Launcher (manche Entwicklungs-Images), bleibt nur LD_PRELOAD + UID-Wechsel.
- **Gewählt, wenn:** `sandbox_mode=basic` oder `auto` darauf zurückfällt (kein Landlock und bubblewrap unbrauchbar - z. B. ein abgeriegelter alter Kernel).
- **Grenzen:** Das ist der schwächste Modus für den *Dateisystem*-Einschluss: Hat der Kernel kein Landlock, gibt es **keinen Dateisystem-Einschluss pro Pfad**, sodass eine Erweiterung nicht sensible, für alle lesbare Dateien lesen kann, und die *Auflistung* der Prozesse in `/proc` ist sichtbar. Es ist **nicht** „keine Isolation“: Die Geheimnisse des Servers bleiben per DAC geschützt (UID-Trennung + `PR_SET_DUMPABLE=0`), die Erweiterung bekommt keinen IP-Ausgang (`-deny-inet`), gefährliche Systemaufrufe werden im Kernel gesperrt (auch für statische Binaries), und die Ressourcen deckelt der Monitor.

### Übersicht der Lage

| Bereitstellung | Dateisystem-Einschluss | Netzwerk | Gefährliche Systemaufrufe | Geheimnisse | Urteil |
|---|---|---|---|---|---|
| Docker + Landlock (`auto`→landlock) | Positivliste pro Pfad | kein direkter Ausgang (`-deny-inet`); über Vermittler | gesperrt (auch statisch) | geschützt | **stark** |
| Docker ohne Landlock (z. B. altes NAS, `auto`→basic) | keiner (für alle Lesbares sichtbar) | kein IP-Ausgang (`-deny-inet`) | gesperrt (auch statisch) | geschützt (DAC + DUMPABLE + seccomp) | ordentlich, nicht gleichwertig mit Landlock |
| Bare Metal (`auto`→bubblewrap) | Namespace + Mount-Positivliste | Netzwerk-Namespace | Namespaces (PID/mount/net) isolieren; LD_PRELOAD-seccomp in `strict` - der Kernel-Filter des Launchers gilt im bwrap-Modus *nicht* | geschützt | **am stärksten** |

## Bekannte Grenzen der Sandbox

| Grenze | Auswirkung | Heutige Abschwächung | Fahrplan |
|---|---|---|---|
| Kein Dateisystem-Einschluss pro Pfad im Modus `basic` (Host ohne Landlock) | eine Erweiterung kann nicht sensible, für alle lesbare Dateien lesen; auch die Verzeichnisse benachbarter Erweiterungen sind sichtbar | die Geheimnisse des Servers bleiben per DAC geschützt (UID-Trennung + `PR_SET_DUMPABLE=0`); der Analysator meldet Muster wie `/etc/passwd`; `auto` bevorzugt Landlock/bubblewrap, wo verfügbar | - (Landlock oder bubblewrap sind die Antwort; beide werden schon ausgeliefert) |
| Alle Erweiterungen teilen sich die UID 1001 | eine bösartige Erweiterung kann ihren Geschwistern Signale schicken (Signale an PID 1 / den Server sperrt seccomp) | `PR_SET_DUMPABLE=0` schließt Lesezugriffe auf `/proc/<pid>/{environ,mem,maps,fd}` zwischen Prozessen derselben UID; Landlock/bubblewrap isolieren die Verzeichnisse der Erweiterungen vollständig | User-Namespace pro Erweiterung über bubblewrap |
| seccomp ist eine Sperrliste, keine Positivliste | die Erweiterung kann frei `open`/`read`/`write`/(AF_UNIX) `socket`/`connect` aufrufen; ptrace, mount, Namespaces, bpf, kexec, Laden von Modulen, die setuid-Familie, Keyring und IP-Sockets (alle Erweiterungen) werden im Kernel gesperrt, auch für statische Binaries - siehe [§4](#4-syscall-confinement-kernel-seccomp--ld_preload) | seccomp im Kernel über den Launcher (vor exec, deckt statische Binaries ab) + `-deny-inet`; Sperre über die Berechtigung `subprocess` | echter Positivlisten-Modus für Systemaufrufe bei `strict`-Erweiterungen |
| Der Vermittler verlagert die SSRF-Angriffsfläche auf den Host | ein Fehler in der URL-/IP-Prüfung des Vermittlers könnte eine Erweiterung über den Host eine interne Adresse erreichen lassen | der Vermittler setzt die Positivliste `external_fetch` + einen SSRF-sicheren Verbindungsaufbau (private/reservierte IPs beim Verbinden abgelehnt) in Go durch; Betreiber SOLLTEN eine Firewall für den Ausgang des Containers ergänzen (`169.254.0.0/16` verwerfen, DB+Meili erlauben) als unabhängige Schicht | nftables pro Namespace / eigener Ausgang für den Vermittler (V2) |
| Der Regex-Analysator lässt sich durch Verschleierung umgehen | ein entschlossener Angreifer kann Muster verstecken | Vorschau durch den Administrator + Prüfung der Berechtigungen; die Grenze der Durchsetzung ist die Sandbox, nicht der Analysator | serverseitige Risikobewertung + Prüfung des Rufs des Autors |

## Konfiguration von `sandbox_mode`

Einstellbar über `evey.yaml` (oder die Umgebungsvariable `EVEY_PLUGINS_SANDBOX_MODE`); die ausführliche Aufschlüsselung je Modus steht oben im Abschnitt [Sandbox-Modi](#sandbox-modes):

```yaml
sandbox_mode: auto   # default - resolves landlock → bubblewrap → basic for this host
# sandbox_mode: landlock    # Landlock LSM allowlist + no-IP-egress (-deny-inet) + seccomp (fails closed if Landlock absent)
# sandbox_mode: basic       # UID drop + kernel/LD_PRELOAD seccomp + -deny-inet + best-effort Landlock
# sandbox_mode: bubblewrap  # bwrap namespace isolation (bare-metal / privileged only)
# sandbox_mode: strict      # bubblewrap + LD_PRELOAD seccomp lib
```

`EVEY_PLUGINS_UID_SEPARATION` (`auto` | `on` | `off`, Standard `auto`) ist ein eigener Regler, der steuert, ob Erweiterungen unter einer eigenen UID laufen (siehe [§3](#3-uid-separation-dac)): `auto` schaltet sie nur ein, wenn der Modus zu `basic` auflöst, `on` erzwingt sie überall (zusätzliche unabhängige Schicht), `off` schaltet sie ab. Vom Launcher getragen, scheitert in `basic` geschlossen (nur unter `landlock` nach bestem Bemühen); unverträglich mit `no_new_privs`. Durchgang je Szenario: [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md).

Zuordnung je Modus (alle Werte hängen von den Fähigkeiten des Hosts ab - z. B. verlangt Landlock einen Kernel ≥ 5.13 mit Landlock in der aktiven LSM-Liste):

| Schicht | auto | landlock | basic | bubblewrap | strict |
|---|---|---|---|---|---|
| UID-Trennung (1001) - `EVEY_PLUGINS_UID_SEPARATION`, Standard `auto` | nur in basic an | aus (mit `on` erzwingen) | an | über userns¹ | über userns¹ |
| seccomp im Kernel (Launcher, vor exec, deckt statische Binaries ab) | ja, wenn zu landlock/basic aufgelöst; nein, wenn zu bubblewrap | ja | ja | nein (bwrap hat kein Launcher-seccomp; `applySandbox` leitet bubblewrap/strict an `applyBwrapSandbox`, der den Launcher nie aufruft) | nein (strict fügt über bwrap nur den LD_PRELOAD-Shim hinzu - nur dynamische Erweiterungen, kein seccomp im Kernel) |
| LD_PRELOAD-seccomp (interpretierte Erweiterungen) | ja | ja | ja | nein | ja |
| `-deny-inet` (kein IP-Socket für IRGENDEINE Erweiterung) | ja | ja | ja | entfällt (Netz-Namespace) | entfällt (Netz-Namespace) |
| Dateisystem-Einschluss | löst zum besten Verfügbaren auf | Landlock-Positivliste | keiner (nur DAC) | Mount-Positivliste | Mount-Positivliste |
| Netzwerk (alle Erweiterungen: kein direkter IP-Ausgang; Ausgang über den `Fetch`-Vermittler des Hosts) | `-deny-inet` | `-restrict-net` + `-deny-inet` | `-deny-inet` | `--unshare-net` | `--unshare-net` |
| Namespaces (user/pid/uts/ipc/cgroup) | entfällt | entfällt | entfällt | abgetrennt | abgetrennt |
| Ressourcengrenzen | Monitor im Userspace + `nice` | Monitor im Userspace + `nice` | Monitor im Userspace + `nice` | Monitor im Userspace + `nice` | Monitor im Userspace + `nice` |

¹ In `bubblewrap`/`strict` wird der Launcher nicht benutzt, der Wechsel über `cap_setuid` greift also nicht; stattdessen sorgt der abgetrennte **User-Namespace** für die UID-Isolation.

Hinweis: Eine echte *Positivliste* der Systemaufrufe für `strict` steht noch auf dem Fahrplan; heute ist `strict` = bubblewrap + die LD_PRELOAD-Sperrliste.

Abhängigkeiten der Umsetzung:

- `evey-plugin-launcher` (reines Go, ohne CGO), neben dem Server-Binary ausgeliefert - wendet Landlock + den seccomp-Filter im Kernel an. Benutzt von `landlock`, `auto` und `basic`.
- `bubblewrap`, installiert mit `apt-get` in der Laufzeitstufe des `Dockerfile` (Debian-Paket, ~200 KB); benutzt von `bubblewrap`/`strict`.
- Für bubblewrap: im Kernel des Hosts aktivierte User-Namespaces ohne Privilegien (`kernel.unprivileged_userns_clone=1`, wo es diesen sysctl gibt). In Docker braucht das Einrichten von Namespaces/Mounts typischerweise `--security-opt seccomp=unconfined` oder ein gelockertes AppArmor - weshalb `auto` für die Bereitstellung im Container Landlock bevorzugt.

## Netzwerkausgang der Erweiterungen (über Vermittler, seit 2026-07-11)

Erweiterungen haben **keinen direkten Netzwerkzugang**: Die Sandbox verweigert
jeder Erweiterung alle IP-Sockets (`--unshare-net` / `-deny-inet`), unabhängig
von der Berechtigung `network`. Eine Erweiterung, die das Netz braucht, ruft die
Host-RPC `HostService.Fetch` über ihren `AF_UNIX`-Host-Socket auf, geschützt
durch die Berechtigung `network`. Der Host
(`internal/adapters/driving/pluginhost/fetch.go`) führt dann in Go aus:

- er prüft, ob der Zielhost auf der angegebenen Positivliste `external_fetch`
  der Erweiterung steht (jetzt ein echter Vertrag pro Host, kein bloßer
  DNS-Hinweis) - ein exakter Host, ohne Beachtung der Groß-/Kleinschreibung
  abgeglichen (der Abgleich versteht auch eine Form `*.domain`, aber das
  Installationsprogramm lehnt Platzhalter-Einträge ab, sodass keiner ihn je
  erreicht);
- er beschränkt das Schema auf `http`/`https` und die Methode auf `GET`/`HEAD`;
- er verbindet sich über den SSRF-sicheren Client (`internal/pkg/ssrf`), der
  private/reservierte/Link-Local-IPs **nach** der DNS-Auflösung beim Verbinden
  ablehnt, sodass DNS-Rebinding und Weiterleitungen erneut geprüft werden;
- er hängt keine eigenen Zugangsdaten des Servers an und begrenzt den
  Antwortkörper.

**Härtung durch den Betreiber (empfohlen).** Der Vermittler verlagert die
SSRF-Angriffsfläche auf den Host; ergänzen Sie also auf dem Container
`evey-server` eine unabhängige Ausgangsschicht, die dem Code nicht vertraut:
Verwerfen Sie `169.254.0.0/16` (Cloud-Metadaten / Link-Local) und beschränken
Sie das interne Subnetz auf die Endpunkte von Postgres + Meilisearch. Ein Fehler
in der Prüfung des Vermittlers trifft dann immer noch auf eine Netzwerkwand; die
beiden Schichten scheitern unabhängig voneinander.

**Was der Vermittler nicht abdeckt.** Die Umgehungen der früheren
DNS-Positivliste (IP-Literale, eine Erweiterung mit eigenem Resolver, die
Wiederverwendung einer aufgelösten Verbindung über SNI-/Host-Tricks) gehörten
zum direkten Ausgang und sind mit ihm verschwunden: Der Erweiterungsprozess kann
überhaupt keinen IP-Socket öffnen. Was bleibt, ist die Grenze, die schon das
Sicherheitsmodell für Erweiterungen nennt: Der Vermittler kontrolliert, *wohin*
ein Aufruf geht, nicht *was* er mitnimmt, sodass Daten immer noch zu einem Host
auf der Positivliste abfließen können.

**Empfehlungen zur Abschwächung**:
- Behandeln Sie die Berechtigung `network` als „manuell prüfen“: Erlauben Sie
  nur Hosts, denen Sie die Daten der Erweiterung anvertrauen, und lesen Sie,
  was die Erweiterung ihnen schickt.
- Betreiber sollten den ausgehenden Verkehr des Hosts überwachen, auf dem die
  Erweiterungen laufen (Prometheus + Flow-Exporter oder einfach
  `ss -tnp | grep <plugin-pid>`).

**Fahrplan V2** (nach 1.0):
- Eine nftables-Firewall für den Ausgang auf Container-Ebene als unabhängige
  Schicht hinter dem Vermittler.
- Ein optionaler HTTP-Proxy für den Ausgang mit Prüfung auf Ebene der Anfragen.

## Netzwerkexposition (außerhalb des Bedrohungsmodells der Erweiterungen)

Alles oben betrifft Erweiterungen. Die andere Hälfte der Angriffsfläche dieses
Servers ist, wer ihn überhaupt erreichen kann, und die wird von zwei Invarianten
bestimmt, die hier genannt werden, weil jemand, der nach „wie stelle ich Evey
sicher bereit“ sucht, in dieser Datei beginnt:

- **Ein Listener ist zum Veröffentlichen gedacht, und das ist nicht der
  gRPC-Listener.** Der native gRPC-Port (`EVEY_SERVER_PORT`, Standard 9090) ist
  ein interner Transport, dessen einziger Client das HTTP-Gateway im selben
  Prozess ist; die HTTP-API ist dieser Port **plus eins**. Der gRPC-Listener
  bindet standardmäßig an Loopback (`EVEY_GRPC_BIND=127.0.0.1`), gerade weil die
  Sperre für den Fernzugriff unten eine HTTP-Middleware ist und ihn nicht
  abdeckt. Diese Bindung zu erweitern ist möglich und erzeugt beim Start eine
  Warnung; ihn per Firewall abzuschirmen ist dann Sache des Betreibers.
- **Die Sperre für den Fernzugriff entscheidet LAN oder WAN anhand der EINEN
  Client-IP, die an der Grenze des vertrauenswürdigen Proxys geprüft wurde, nie
  anhand eines Anfrage-Headers.** Ein Client kann sich nicht per
  `X-Forwarded-For` als `127.0.0.1` ausgeben, um als lokal behandelt zu werden,
  und eine Anfrage, die diese Grenze nie überquert hat, gilt als WAN
  (abgelehnt), statt als vertrauenswürdig. Beachten Sie, dass dies eine
  *Bereitstellungs*-Kontrolle ist („diese Instanz ist für mein eigenes Netz“),
  keine Authentifizierungsschicht - die Daten schützt die Authentifizierung.

Einzelheiten und die vollständige Liste der Variablen: [configuration.md](configuration.md#remote-access-who-may-connect-from-outside-your-network)
und [deployment-bare-metal.md](deployment-bare-metal.md).

## Was die Logs enthalten können

Die Logdatei des Servers ist das, was die Log-Routen der Administration
(Verfolgen, Suche, Live-Ansicht) ausliefern, was in Fehlerberichten landet und
was eine Sicherung mitnimmt; Zugangsdaten werden daher **beim Schreiben**
entfernt (`internal/pkg/logger/redact.go`): Ein Attribut, dessen Schlüssel ein
Geheimnis benennt (`authorization`, `token`, `access_token`, `refresh_token`,
`api_key`, `password`, `pin`, `secret`, ...), wird vollständig ersetzt, und jede
Zeichenkette - die Nachricht eingeschlossen -, die einen Bearer, ein JWT oder
ein Paar `secret=value` enthält, wird bereinigt. Die Log-Routen der
Administration wenden dieselbe Regel beim Ausliefern auf Zeilen an, die vor
dieser Bereinigung geschrieben wurden. Zwei Folgen für den Code: Ein nicht
geheimer Wert darf nicht unter einem geheimen Schlüsselnamen geloggt werden (er
würde maskiert - `token_info`, `device_pin` sind die heute benutzten Namen), und
eine neue Art von Zugangsdaten muss in diese Liste aufgenommen werden, bevor sie
jemals geloggt wird.

## Eine Schwachstelle melden

Melden Sie vertraulich, nie in einem öffentlichen Issue: `security@eveyproject.org`.
Das ist die Adresse, die die [Sicherheitsrichtlinie](../SECURITY.md) des
Repositorys nennt, und sie gilt für Authentifizierung, Berechtigungen und
Sandbox-Ausbrüche von Erweiterungen wie für alles andere. Testen Sie nicht gegen
geteilte oder öffentliche Evey-Instanzen, die nicht Ihnen gehören.
