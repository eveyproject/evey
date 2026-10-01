---
source_sha: 0ac15ab64c61bb43ae41b6ec652dda6261a8eed8431ec891b4f9ad3b33dd7d21
reviewed: false
---
# Plugin-Sandbox und Bedrohungsmodell

Diese Seite beschreibt, was der Server tut, um ein Plugin einzusperren: die
Prüfungen bei der Installation, die Schichten zur Laufzeit, die Sandbox-Modi und
was jede davon noch nicht verhindern kann. Wenn Sie ein Plugin schreiben und
wissen wollen, was Sie angeben dürfen und was abgelehnt wird, lesen Sie
[Berechtigungen und Slots](plugins/security-model.md).

## Bedrohungsmodell

Plugins laufen als eigene Prozesse, die über gRPC auf Unix-Sockets mit dem
Server sprechen. Der Server vertraut standardmäßig keinem Plugin-Code. Die
Risiken, gegen die er sich wehrt, vom schwersten zum leichtesten:

1. **Identitätsdiebstahl** - ein Plugin gibt sich als anderes Plugin aus, um
   dessen Berechtigungen zu nutzen.
2. **Rechteausweitung** - ein Plugin bricht aus seiner Sandbox aus und führt
   Befehle als Server aus.
3. **Dateisystemzugriff** - ein Plugin liest oder schreibt außerhalb seines
   eigenen Verzeichnisses, insbesondere die Geheimnisse des Servers.
4. **Datenabfluss über das Netzwerk** - ein Plugin sendet Daten an einen Host
   seiner Wahl.
5. **Ressourcenmissbrauch** - Mining, Fork-Bomben, Erschöpfung von Speicher oder
   Festplatte.
6. **Lieferkette** - verschleierter oder kompilierter Code, den die Analyse
   nicht lesen kann.

## Bei der Installation

### Entpacken des ZIP

Bevor eine Datei geschrieben wird:

- höchstens 5.000 Einträge, 100 MB pro Datei, insgesamt 500 MB entpackt und ein
  Kompressionsverhältnis von 100:1 oberhalb von 1 MB entpackt
  (Dekompressionsbomben);
- Einträge, deren Pfad das Archiv verlässt (`..`), werden abgelehnt;
- symbolische Links werden abgelehnt;
- setuid-, setgid- und Sticky-Bits werden entfernt.

Quelle: `internal/adapters/driven/pluginrunner/zip.go`.

### Manifest und Berechtigungen

Das Manifest gibt die Berechtigungen an, die ein Plugin verlangt. Eine unbekannte
Berechtigung lehnt die Installation ab. Sie werden dann durchgesetzt:

- **`subprocess`** erlaubt Kindprozesse. Ohne sie verweigert der Kernel die
  Prozesserzeugung (`-deny-fork`), und der `LD_PRELOAD`-Filter verweigert außerdem
  `execve` für interpretierte Plugins.
- **`native-binary`** erlaubt, kompilierten Code oder Archive mitzuliefern, die
  die Analyse nicht lesen kann.
- **API-Berechtigungen** (`api.library`, `api.user`, `api.metadata`,
  `media.read`, `media.write`, `notification.sender`...) werden bei jedem Aufruf
  des Hosts von
  [`PluginPermissionInterceptor`](../internal/adapters/driving/grpcapi/plugin_interceptor.go)
  geprüft.

Die vollständige Liste der Berechtigungen steht unter [Berechtigungen und Slots](plugins/security-model.md#permissions-catalog).

### Statische Analyse

Jede Datei des Pakets wird von
[`pluginrunner.AnalyzePlugin`](../internal/adapters/driven/pluginrunner/analyzer.go)
durchsucht, nach einem Normalisierungsdurchlauf, der gängige Verschleierungen
rückgängig macht (Aliase, Verkettung von Zeichenketten, f-Strings):

| Kategorie | Schwere | Beispiele |
|---|---|---|
| `shell_exec` | CRITICAL | `os.system`, `subprocess.run`, `child_process.exec`, PHP `system()`/`shell_exec()`/`proc_open()`, `preg_replace /e`, Ruby-Backticks, `open('\|cmd')` |
| `network` | WARNING / CRITICAL | `requests.get`, `fetch`, Reverse- und Bind-Shells |
| `filesystem` | WARNING / CRITICAL | `/etc/passwd`, `.ssh/`, `rm -rf`, Operationen mit symbolischen Links |
| `obfuscation` | CRITICAL | `eval`, base64-Dekodierung, Hex-Sequenzen, `__builtins__`, `base64 -d \| bash` |
| `privilege` | CRITICAL | `setuid`, `chmod 4777`, `sudo`, Signale an PID 1 |
| `cryptomining` | CRITICAL | `stratum+tcp`, `xmrig`, `cryptonight` |
| `runtime_install` | CRITICAL | `pip install`, `npm install`, `curl ... \| sh` |
| `dynamic_import` | CRITICAL | `__import__()`, `importlib.import_module` |
| `sandbox_escape` | CRITICAL | `vm.runIn*`, `process.binding`, `new Function`, `__subclasses__()`, `__mro__` |
| `deserialization` | CRITICAL / WARNING | `pickle.loads`, `marshal.loads`, `__reduce__`; WARNING: `yaml.load()` ohne Loader |
| `native_code` | CRITICAL / WARNING | `ctypes.CDLL`, `ffi.Library`; WARNING: Go `unsafe` |
| `compiled_binary` | WARNING, dann BLOCK | `.so`, `.dll`, `.exe`, `.wasm`, `.jar` |
| `embedded_archive` | WARNING, dann BLOCK | `.zip`, `.tar.gz` |
| `mime_mismatch` | wie CRITICAL behandelt | eine Endung, die ein Bild vorgibt, mit anderem Inhalt |
| `suspicious_url` | WARNING | ein Nicht-HTTP-Schema in einem URL-Literal |
| `unvendored_deps` | WARNING | `requirements.txt` oder `package.json` ohne mitgelieferte Module |

Ein CRITICAL-Fund oder ein `mime_mismatch` lehnt die Installation ab.
`compiled_binary` und `embedded_archive` lehnen sie ab, außer das Manifest gibt
`native-binary` an.

**Was die Analyse nicht garantieren kann.** Sie arbeitet mit Mustern. Sie löst
bekannte Verschleierungsklassen auf (Verkettung von Zeichenketten zu einem
Aufrufbaren über mehrere Schritte, Nachschlagen eingebauter Funktionen per Name,
Ausbrüche über `Function.constructor`), jede mit einer Regressionsvorlage in
`tests/fixtures/plugins/malicious/`, aber sie kann nicht jede zur Laufzeit
vorgenommene Umwandlung verfolgen (`strrev("metsys")`, Dekodieren einer aus einer
Datei gelesenen Nutzlast). Sie trifft auch in Kommentaren: Ein Kommentar
`# pip install foo` löst `runtime_install` aus. **Die Grenze der Durchsetzung ist
die Sandbox zur Laufzeit, nicht die Analyse**: Ein Plugin, das ein Muster an ihr
vorbeischmuggelt, kann trotzdem die Daten des Servers nicht lesen, keinen
verweigerten Systemaufruf machen, seine Ressourcengrenzen nicht überschreiten und
keine Host-Methode aufrufen, für die es keine Berechtigung hat.

### Signatur

Ein Paket kann mit cosign signiert werden. `EVEY_PLUGINS_COSIGN_POLICY` legt
fest, was eine fehlgeschlagene Prüfung bewirkt:

| Richtlinie | Wirkung |
|---|---|
| `off` | Die Signatur wird nicht geprüft. |
| `warn` (Standard) | Ein Fehlschlag wird festgehalten und dem Administrator gezeigt, ohne zu blockieren. Ohne konfigurierten öffentlichen Schlüssel wird nichts geprüft. |
| `enforce` | Ein Fehlschlag lehnt die Installation oder das Update ab. |

Den öffentlichen Schlüssel liefert `EVEY_PLUGINS_COSIGN_PUBLIC_KEY`. Quelle:
`internal/adapters/driven/pluginrunner/cosign.go`.

### Vorschau für den Administrator

`POST /api/v1/admin/plugins/preview` führt die Analyse aus, ohne zu installieren.
Der Administrator sieht die verlangten Berechtigungen, jeden Fund mit Datei,
Zeile und Kategorie, die im Code gefundenen URLs und den SHA256 des Pakets;
derselbe Hash wird beim Anwenden der Installation geprüft, sodass das analysierte
Paket das installierte ist. Ein Update zeigt die Unterschiede bei
Berechtigungen, Slots und Hosts, und eine neue Berechtigung verlangt eine
ausdrückliche Bestätigung.

Der Administrator ist die Vertrauensgrenze für erweiterte Berechtigungen: Evey
zeigt die Angriffsfläche, der Administrator nimmt an oder lehnt ab.

## Zur Laufzeit

### Dateisystem

Ein Plugin sieht sein eigenes Verzeichnis und sein Socket-Verzeichnis, nicht die
Daten des Servers. Wie das durchgesetzt wird, hängt vom
[Sandbox-Modus](#sandbox-modes) ab: eine Landlock-Erlaubnisliste oder eine
bubblewrap-Liste erlaubter Einbindungen. Im Modus `basic` auf einem Kernel ohne
Landlock gibt es keine Einschränkung pro Pfad: Dann schützen der eigene Benutzer
und die Dateirechte die Geheimnisse des Servers (unten).

### Netzwerk

Ein Plugin hat **keinen direkten Netzwerkzugang**, was auch immer es angibt:
Jeder IP-Socket wird verweigert (`-deny-inet` in `landlock` und `basic`, ein
Netzwerk-Namespace in `bubblewrap` und `strict`). Ein Plugin mit der Berechtigung
`network` ruft die Methode `Fetch` des Hosts über seinen Unix-Socket auf. Der
Host, in `internal/adapters/driving/pluginhost/fetch.go`:

- prüft das Ziel gegen die Liste `external_fetch` des Plugins (exakte Hosts,
  ohne Beachtung der Groß- und Kleinschreibung);
- erlaubt nur `http` und `https`, und nur `GET` und `HEAD`;
- verbindet über einen SSRF-sicheren Client (`internal/pkg/ssrf`), der private,
  reservierte und Link-Local-Adressen nach der DNS-Auflösung verweigert, sodass
  DNS-Rebinding und Weiterleitungen erneut geprüft werden;
- fügt keine Zugangsdaten des Servers hinzu und begrenzt die Größe der Antwort.

Das schließt die Wege, die direkter ausgehender Verkehr offen ließ
(IP-Literale wie `169.254.169.254`, der eigene Resolver eines Plugins). Was
bleibt: Der Broker kontrolliert, *wohin* ein Aufruf geht, nicht *was* er trägt.
Siehe [Bekannte Grenzen](#known-limits).

**Empfohlen für Betreiber.** Der Broker verlagert die SSRF-Angriffsfläche auf den
Host. Eine Ausgangsregel am Container `evey-server`, unabhängig vom Code, fügt
eine zweite Mauer hinzu: `169.254.0.0/16` verwerfen und das interne Netzwerk auf
die Endpunkte von Datenbank und Suche beschränken.

### Systemaufrufe

Zwei seccomp-Filter verweigern dieselben gefährlichen Systemaufrufe:

1. **Der Kernelfilter**, installiert von
   [`evey-plugin-launcher`](../cmd/evey-plugin-launcher) direkt bevor er das
   Plugin startet ([`seccomp_linux.go`](../cmd/evey-plugin-launcher/seccomp_linux.go)).
   Er wird vom Plugin und seinen Kindern geerbt und **deckt auch kompilierte,
   statische Programme ab**. Eine abweichende Architektur in einem Systemaufruf
   beendet den Prozess. In `landlock` und `basic` angewendet.
2. **Der `LD_PRELOAD`-Filter**, [`sandbox/sandbox.c`](../sandbox/sandbox.c), in
   interpretierte Plugins geladen, die `subprocess` nicht angeben. Er verweigert
   außerdem `execve`/`execveat` und markiert das Plugin als nicht dumpbar. In
   `landlock`, `basic` und `strict` angewendet. Fehlt die Bibliothek, wird der
   Start eines solchen Plugins verweigert (`plugin.error`).

Verweigerte Systemaufrufe:

| Kategorie | Systemaufrufe |
|---|---|
| Debugging | `ptrace`, `process_vm_readv`, `process_vm_writev` |
| Signale | `kill`; der Kernelfilter verweigert auch `tgkill`, der `LD_PRELOAD`-Filter verweigert `tkill`/`tgkill` an PID 1 |
| Einbindungen | `mount`, `umount2`, `pivot_root`, `chroot` |
| Namespaces | `unshare`, `setns` |
| Kernelmodule | `init_module`, `finit_module`, `delete_module` |
| BPF, perf | `bpf`, `perf_event_open` |
| Hardware-E/A | `ioperm`, `iopl` (nur x86-64, wo es sie gibt) |
| Swap | `swapon`, `swapoff` |
| Systemsteuerung | `syslog`, `reboot`, `settimeofday`, `clock_settime`, `clock_adjtime`, `adjtimex` |
| Kexec | `kexec_load`, `kexec_file_load` |
| Primitive für Kernel-Exploits | `io_uring_setup`, `io_uring_enter`, `io_uring_register`, `userfaultfd` |
| Identität | `setuid`, `setgid`, `setreuid`, `setregid`, `setresuid`, `setresgid`, `capset` |
| Schlüsselbund | `add_key`, `request_key`, `keyctl` |

Für ein Plugin ohne `subprocess` verweigert der Kernel außerdem die
Prozesserzeugung (`fork`, `vfork`, `clone` ohne `CLONE_THREAD`); Threads
funktionieren weiter.

Das ist eine **Sperrliste**: `open`, `read`, `write`, `mmap`, Unix-`socket` und
`connect` bleiben erlaubt. Pfade werden von der Dateisystemschicht eingeschränkt,
nicht von seccomp. In `bubblewrap` wird der Launcher nicht genutzt: Ein
statisches Programm erhält dort keinen seccomp-Filter und wird von den Namespaces
und der Liste erlaubter Einbindungen eingeschränkt.

### Ein eigener Benutzer

Ein Plugin kann als Benutzer 1001 (`evey-plugin`) laufen, getrennt vom Server
(1000), sodass die Geheimnisse des Servers allein durch die Dateirechte für es
unlesbar sind. `EVEY_PLUGINS_UID_SEPARATION` steuert das:

- `auto` (Standard): nur wenn die Sandbox zu `basic` aufgelöst wird, dem einen
  Fall, in dem die Dateirechte die einzige Mauer um die Geheimnisse sind;
- `on`: immer, als zusätzliche Schicht;
- `off`: nie.

Den Wechsel vollzieht der Launcher, der `cap_setuid` hält; der Server selbst
läuft ohne Rechte oder Capabilities. Der Launcher verweigert jedes Ziel unter
1000. Kann er nicht wechseln, wird ein Plugin in `basic` verweigert (der Benutzer
ist dort die einzige Mauer), während es in `landlock` mit einer Warnung als
Benutzer des Servers läuft. In `bubblewrap` und `strict` übernimmt der
User-Namespace diese Rolle.

Ist sie aktiv, sind der Signaturschlüssel (`0600`) und die Sicherungen
(Verzeichnis `0700`, Dateien `0600`) zur Laufzeit nur für den Besitzer
zugänglich, was auch auf einem per Bind eingebundenen `/data` gilt. Alle Plugins
teilen den Benutzer 1001: Sie sind durch die Dateisystemschicht voneinander
isoliert, nicht durch den Benutzer.

### Authentifizierung am Host-Socket

Jedes Plugin hat seinen eigenen Socket, `/tmp/evey/plugins/<plugin_id>/host.sock`,
bedient von seinem eigenen gRPC-Server. Bei jedem Aufruf liest der Server die PID
des Aufrufers aus dem Kernel (`SO_PEERCRED`, was der Userspace nicht fälschen
kann) und vergleicht sie mit der beim Start des Plugins festgehaltenen PID. Die
Identität des Plugins kommt aus dem Socket, nie aus einem Header. Ein Plugin kann
sich daher nicht als anderes ausgeben, nicht den Socket eines anderen Plugins
aufrufen und keine Aufrufe über einen Kindprozess leiten.

### Ressourcengrenzen

Ein Monitor im Userspace (cgroups sind in einem Container ohne Rechte nicht
verfügbar) liest etwa alle 2 Sekunden `/proc/<pid>` und stoppt ein Plugin, das
eine Grenze überschreitet:

- Speicher (RSS), standardmäßig 256 MiB;
- Prozesse und Threads, standardmäßig 64;
- von seinem Verzeichnis belegter Plattenplatz, standardmäßig 100 MiB, alle 30
  Sekunden geprüft;
- CPU-Priorität, standardmäßig `nice` 10, beim Start angewendet.

Die Grenzen werden pro Plugin unter **Admin → Plugins** festgelegt. Siehe
[ADR-0013](adr/0013-userspace-plugin-resource-monitor.md).

### Isolation der Oberfläche

Die Oberfläche eines Plugins läuft in einem `<iframe sandbox="allow-scripts">`,
ohne Zugriff auf denselben Ursprung, ohne Formulare oder Pop-ups, unter einer
Content-Security-Policy, die Verbindungen nur zu seinen Hosts in
`external_fetch` erlaubt. Die Brücke begrenzt, was es von der Seite verlangen
kann: `toast` höchstens 3 pro Sekunde, 200 Zeichen, kein HTML; `navigate`
akzeptiert nur `/home`, `/library`, `/media`, `/player`, `/settings` und
`/search`; `resize` zwischen 50 und 800 px. Die Slots werden bei der Installation
und erneut beim Zeichnen geprüft.

### Härtung des Servers

Beim Start markiert sich der Server als nicht dumpbar (`PR_SET_DUMPABLE=0`,
[`hardening_linux.go`](../cmd/evey/hardening_linux.go)): Ein Prozess, der als
derselbe Benutzer läuft, kann seinen Speicher oder seine Umgebung nicht über
`/proc` lesen, was den Signaturschlüssel im Speicher sowie die Passwörter von
Datenbank und Suche schützt, selbst auf einem Kernel ohne Landlock.

### Absturzmeldung

Der Runner veröffentlicht ein Ereignis `plugin.error`, wenn die
Sandbox-Bibliothek für ein Plugin fehlt, das sie braucht, oder wenn ein Plugin mit
einem Fehler endet. Der Benachrichtigungsdienst leitet es über die eingerichteten
Kanäle an den Administrator weiter. Eine abweichende PID an einem Host-Socket wird
als Warnung protokolliert.

## Sandbox-Modi

Der Modus wählt, wie Dateisystem und Netzwerk eingeschränkt werden.
`EVEY_PLUGINS_SANDBOX_MODE` setzt den Anfangswert (Standard `auto`); er lässt sich
unter **Admin → Plugins** ändern. Der Runner wendet ihn in
[`applySandbox`](../internal/adapters/driven/pluginrunner/runner_sandbox.go) an.
In jedem Modus: kein direkter IP-Ausgang, der Ressourcenmonitor und die
Authentifizierung am Host-Socket.

### `auto` (Standard)

Wird einmal, beim ersten Start eines Plugins, zum stärksten Modus aufgelöst, den
der Host unterstützt: **landlock**, dann **bubblewrap**, dann **basic**. Landlock
kommt zuerst, weil es keine Rechte braucht und unter dem Standardsicherheitsprofil
von Docker läuft. bubblewrap wird nur gewählt, wenn Landlock fehlt und sich eine
Test-Sandbox tatsächlich aufbauen lässt
([`ProbeSandbox`](../internal/adapters/driven/pluginrunner/sandbox_probe.go)), was
in der Praxis Betrieb ohne Container oder einen privilegierten Container bedeutet.
Der aufgelöste Modus wird protokolliert und in der Zeile **Host-Sandbox** unter
**Admin → Plugins → Sicherheits-Dashboard** angezeigt.

### `landlock`

- **Dateisystem:** eine vom Launcher angewendete Landlock-Erlaubnisliste.
  Systemverzeichnisse (`/usr`, `/lib`, `/lib64`, `/bin`, `/sbin`, `/etc`, `/app`)
  schreibgeschützt; aus `/dev` nur `null`, `zero`, `full`, `random` und
  `urandom`, sodass die Knoten der Grafikkarten und das Terminal des Servers außer
  Reichweite bleiben; das Verzeichnis des Plugins und sein Socket-Verzeichnis mit
  Schreibzugriff; alles andere, einschließlich `/data` und anderer Plugins,
  verweigert. Ein Plugin, dem `hardware.gpu` gewährt wurde, erhält seinen
  Render-Knoten als einzige Ausnahme.
- **Netzwerk:** `-restrict-net` und `-deny-inet`; Ausgang nur über `Fetch`.
- **Systemaufrufe:** der Kernelfilter und der `LD_PRELOAD`-Filter.
- **Erfordert:** einen Kernel ab 5.13 mit Landlock in der aktiven LSM-Liste und
  den Launcher (beides trifft auf das offizielle Image mit einem aktuellen Host
  zu). Ein ausdrückliches `landlock` verweigert den Start eines Plugins, das es
  nicht einschränken kann.

### `bubblewrap`

- **Dateisystem und Prozesse:** sechs getrennte Namespaces (`user`, `pid`,
  `uts`, `ipc`, `cgroup`, `net`) und eine Liste erlaubter Einbindungen; sonst
  existiert darin nichts.

  | Pfad | Zugriff |
  |---|---|
  | `/usr`, `/bin`, `/sbin`, `/lib`, `/lib64` | schreibgeschützt |
  | `/etc/ssl`, `/etc/ca-certificates`, `/etc/resolv.conf`, `/etc/nsswitch.conf`, `/etc/{alpine,os}-release` | schreibgeschützt (nie `/etc` als Ganzes: kein `passwd`, `shadow`, `ssh`) |
  | das Verzeichnis des Plugins, sein Socket-Verzeichnis | mit Schreibzugriff |
  | `/proc` | seine eigene Einbindung |
  | `/dev` | eine minimale synthetische Auswahl (null, zero, random, urandom, tty, pts) |
  | `/tmp` | ein leeres tmpfs |

  `/data`, `/var`, `/home`, `/root`, `/run` und die Verzeichnisse anderer Plugins
  existieren darin nicht. Ein Kanarienvogel-Plugin
  (`tests/fixtures/plugins/canary-sandbox-runtime/`) prüft diesen Aufbau gegen
  einen laufenden Stack. Quelle: `buildBwrapCmd` in
  `internal/adapters/driven/pluginrunner/runner_bwrap.go`.
- **Netzwerk:** der Netzwerk-Namespace, für jedes Plugin.
- **Systemaufrufe:** kein seccomp-Filter; die Namespaces sorgen für die Isolation.
- **Erfordert:** das Programm `bwrap` und die Systemaufrufe `mount`/`unshare`. In
  Docker braucht das ein gelockertes Sicherheitsprofil, es ist also faktisch ein
  Modus für den Betrieb ohne Container.

### `strict`

`bubblewrap` plus der `LD_PRELOAD`-Filter (nur interpretierte Plugins).

### `basic`

Der letzte Ausweg von `auto`: der eigene Benutzer, der Kernelfilter, der
`LD_PRELOAD`-Filter, `-deny-inet` und Landlock, wenn der Kernel es hat. Auf einem
Kernel ohne Landlock gibt es keine Einschränkung pro Pfad: Ein Plugin kann für
alle lesbare, nicht sensible Dateien lesen und die Prozessliste sehen. Die
Geheimnisse des Servers bleiben durch den eigenen Benutzer und `PR_SET_DUMPABLE`
geschützt, das Netzwerk ist gekappt, und gefährliche Systemaufrufe werden
verweigert, kompilierte Programme eingeschlossen.

### Übersicht

| Schicht | landlock | basic | bubblewrap | strict |
|---|---|---|---|---|
| Dateisystem | Landlock-Erlaubnisliste | keine ohne Landlock (nur Dateirechte) | Liste erlaubter Einbindungen | Liste erlaubter Einbindungen |
| Netzwerk | `-restrict-net` + `-deny-inet` | `-deny-inet` | Netzwerk-Namespace | Netzwerk-Namespace |
| Kernel-seccomp (auch statische Programme) | ja | ja | nein | nein |
| `LD_PRELOAD`-seccomp (interpretierte Plugins ohne `subprocess`) | ja | ja | nein | ja |
| Eigener Benutzer | mit `on` | ja (`auto`) | User-Namespace | User-Namespace |
| Ressourcengrenzen | ja | ja | ja | ja |

| Bereitstellung | Was Sie bekommen |
|---|---|
| Docker auf einem aktuellen Kernel (`auto` ergibt `landlock`) | Einschränkung pro Pfad, kein direkter Ausgang, gefährliche Systemaufrufe verweigert, auch für statische Programme. Der Normalfall. |
| Docker auf einem alten Kernel ohne Landlock, z. B. einem alten NAS (`auto` ergibt `basic`) | Geheimnisse geschützt und Netzwerk gekappt, aber für alle lesbare Dateien sichtbar. Siehe [Plugin-Sandbox einrichten](plugins/sandbox-deployment.md). |
| Ohne Container (`auto` ergibt `bubblewrap`) | Die stärkste Isolation von Dateisystem und Prozessen, ohne seccomp-Filter für statische Programme. |

## Konfiguration

| Variable | Standard | Wirkung |
|---|---|---|
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto`, `landlock`, `bubblewrap`, `strict` oder `basic`. Auch unter **Admin → Plugins** änderbar. |
| `EVEY_PLUGINS_UID_SEPARATION` | `auto` | `auto`, `on` oder `off`. Nicht vereinbar mit `no_new_privs` am Container. |
| `EVEY_PLUGINS_COSIGN_POLICY` | `warn` | `off`, `warn` oder `enforce`. |
| `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` | keiner | Der Schlüssel, gegen den Signaturen geprüft werden. |

Das offizielle Image liefert `evey-plugin-launcher` (Landlock und der
Kernelfilter) und `bubblewrap` mit. bubblewrap braucht außerdem User-Namespaces
ohne Rechte auf dem Host. Welche Einrichtung zu welcher Maschine passt, in
einfachen Worten: [Plugin-Sandbox einrichten](plugins/sandbox-deployment.md).

## Bekannte Grenzen

| Grenze | Auswirkung | Was sie heute begrenzt |
|---|---|---|
| Keine Einschränkung pro Pfad in `basic` auf einem Kernel ohne Landlock | Ein Plugin kann für alle lesbare, nicht sensible Dateien und die Verzeichnisse anderer Plugins lesen | Geheimnisse durch den eigenen Benutzer und `PR_SET_DUMPABLE` geschützt; `auto` bevorzugt Landlock oder bubblewrap |
| Alle Plugins teilen den Benutzer 1001 | Ein Plugin kann seinen Geschwistern Signale senden (Signale an den Server werden verweigert) | `PR_SET_DUMPABLE` verhindert das Lesen anderer Prozesse über `/proc`; Landlock und bubblewrap isolieren die Plugin-Verzeichnisse |
| Seccomp ist eine Sperrliste | Datei- und Unix-Socket-Aufrufe bleiben erlaubt | Pfade werden von Landlock oder bubblewrap eingeschränkt; IP-Sockets werden verweigert |
| Der Broker verlagert die SSRF-Angriffsfläche auf den Host | Ein Fehler in der Prüfung des Brokers könnte eine interne Adresse erreichen | Erlaubnisliste und SSRF-sicherer Client in Go; eine Ausgangsregel am Container als unabhängige Schicht |
| Der Broker kontrolliert, wohin ein Aufruf geht, nicht was er trägt | Daten können zu einem Host auf der Liste des Plugins abfließen | Nur Hosts erlauben, denen Sie diese Daten anvertrauen |
| Die Analyse arbeitet mit Mustern | Bewusste Verschleierung kann Code verstecken | Vorschau für den Administrator; die Sandbox, nicht die Analyse, ist die Grenze |
| `LD_PRELOAD` deckt nur dynamische, interpretierte Plugins ab | Ein statisches Programm in `basic` ohne den Launcher entgeht ihm | Das offizielle Image liefert immer den Launcher mit, dessen Kernelfilter statische Programme abdeckt |

## Netzwerkexposition (außerhalb des Bedrohungsmodells der Plugins)

Zwei Invarianten regeln, wer den Server überhaupt erreichen kann:

- **Nur der HTTP-Port soll veröffentlicht werden.** Der gRPC-Port
  (`EVEY_SERVER_PORT`, Standard 9090) ist ein interner Transport, dessen einziger
  Client das HTTP-Gateway im selben Prozess ist; die HTTP-API ist dieser Port plus
  eins. gRPC bindet sich standardmäßig an Loopback (`EVEY_GRPC_BIND=127.0.0.1`),
  weil die Sperre für den Fernzugriff eine HTTP-Middleware ist und ihn nicht
  abdeckt. Eine Erweiterung der Bindung protokolliert eine Warnung; die Firewall
  ist dann Sache des Betreibers.
- **Die Sperre für den Fernzugriff entscheidet anhand der einen Client-Adresse,
  die an der Grenze der vertrauenswürdigen Proxys geprüft wurde**, nie anhand
  eines Headers einer Anfrage. Ein Client kann sich über `X-Forwarded-For` nicht
  als `127.0.0.1` ausgeben, und eine Anfrage, die diese Grenze nie überquert hat,
  zählt als entfernt. Das ist eine Kontrolle der Bereitstellung, keine
  Authentifizierung.

Siehe [Fernzugriff und HTTPS](administration/remote-access.md) und die
[Konfigurationsreferenz](environment/server.md#remote-access).

## Was die Logs enthalten dürfen

Das Server-Log speist **Protokolle & Debug**, Fehlerberichte und Sicherungen,
daher werden Zugangsdaten beim Schreiben einer Zeile entfernt
(`internal/pkg/logger/redact.go`): Ein Attribut, dessen Schlüssel ein Geheimnis
benennt (`authorization`, `token`, `api_key`, `password`, `pin`, `secret`...),
wird vollständig ersetzt, und jede Zeichenkette mit einem Bearer-Token, einem JWT
oder einem Paar `secret=value` wird bereinigt. Die Log-Routen wenden dieselbe
Regel auf ältere Zeilen beim Ausliefern an. Für Code: Protokollieren Sie nie einen
nicht geheimen Wert unter einem geheimen Schlüsselnamen (er würde maskiert), und
fügen Sie eine neue Art von Zugangsdaten dieser Liste hinzu, bevor sie je
protokolliert wird.

## Eine Schwachstelle melden

Melden Sie privat, nie in einem öffentlichen Issue: `security@eveyproject.org`,
die Adresse der [Sicherheitsrichtlinie](../SECURITY.md) des Repositorys. Sie
umfasst die Authentifizierung der Plugins, Berechtigungen und Ausbrüche aus der
Sandbox wie alles andere. Testen Sie nicht gegen Evey-Instanzen, die nicht Ihnen
gehören.
