---
source_sha: df7413e4af441d34d3b23275ec280881e68c4257572b636312f678009cf80c2e
reviewed: false
---
# Bereitstellung ohne Container (kein Docker)

Evey direkt auf einem Host betreiben (systemd, ein Init-Skript oder von Hand)
statt mit dem [Docker-Image](installation.md). Das Docker-Image ist der
empfohlene Weg - es bringt die Plugin-Sandbox, die Härtung und die Laufzeiten
fertig verdrahtet mit. Ohne Container stellen **Sie** diese Haltung selbst her:
die richtigen Programme nebeneinander, ein eigener Benutzer ohne root-Rechte,
das Sandbox-Backend, das Ihr Kernel unterstützt, und Verzeichnisse mit
Geheimnissen, auf die nur ihr Besitzer zugreifen kann. Diese Seite ist die Schritt-für-Schritt-Anleitung
dafür.

Wenn Sie nur die einfache Installation wollen, hören Sie hier auf und nutzen
[installation.md](installation.md).

## 1. Voraussetzungen

### Programme (aus dem Quellcode bauen)

Bauen Sie beide Go-Programme und **legen Sie sie in dasselbe Verzeichnis**. Der
Plugin-Runner findet den Launcher, indem er zuerst `$EVEY_PLUGIN_LAUNCHER`
prüft, dann neben seinem eigenen Programm (`os.Executable()`) sucht, dann im
`$PATH` - beide nebeneinander zu halten ist also die einfachste korrekte
Anordnung.

```bash
# aus dem Wurzelverzeichnis des Repositorys
CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(git describe --tags --always) -X main.commit=$(git rev-parse HEAD)" -o evey ./cmd/evey
CGO_ENABLED=0 go build -ldflags="-s -w"                                        -o evey-plugin-launcher ./cmd/evey-plugin-launcher

# nebeneinander installieren, z. B.:
sudo install -m 0755 evey evey-plugin-launcher /opt/evey/

# optional: Shell-Vervollständigung und Handbuchseite, beide vom Programm ausgegeben
/opt/evey/evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
/opt/evey/evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null
```

Suchreihenfolge in `launcherPath()` (`internal/adapters/driven/pluginrunner/runner_landlock.go`):

1. `$EVEY_PLUGIN_LAUNCHER` (muss eine ausführbare Datei sein) - ausdrückliche Übersteuerung.
2. `evey-plugin-launcher` im **selben Verzeichnis** wie das laufende Programm `evey`.
3. `evey-plugin-launcher` irgendwo im `$PATH`.

### Der LD_PRELOAD-seccomp-Shim

Bauen Sie den Shim `libevey-sandbox.so` (eine kleine C-Datei; braucht einen C-Compiler):

```bash
gcc -shared -fPIC -nostartfiles -o libevey-sandbox.so sandbox/sandbox.c
sudo install -m 0644 libevey-sandbox.so /app/libevey-sandbox.so
```

Der Runner sucht ihn unter dem **fest verdrahteten Pfad `/app/libevey-sandbox.so`**
(`sandboxLibPath` in `runner.go`). Fehlt er, **verweigern Plugins den Start**
(fail-closed) - außer im Entwicklungsmodus. Dieser Shim fügt für dynamisch
gelinkte Plugins eine seccomp-Sperrliste hinzu und liegt über Landlock/bubblewrap.

> Das Docker-Image hält die Open-Source-Programme, das SDK und diesen Shim unter
> `/app` (dort liegen keine Geheimnisse). Ohne Container können Sie diesen Pfad
> nachbilden (`/app/libevey-sandbox.so`) oder einen symbolischen Link setzen.

### Laufzeiten

Installieren Sie, was das `entrypoint:` Ihrer Plugins aufruft. Der Runner kennt
diese Interpreter (`knownInterpreters` in `runner_start.go`): `python3`,
`python`, `node`, `php`, `ruby`, `perl`, `lua`, `bash`, `sh`. Mindestens:

- **`python3`** - die Mehrheit der Community-Plugins.
- **`node`**, **`php`**, **`ruby`**, **`lua`** - nur wenn Ihre Plugins sie nutzen.

Dazu die grundlegenden Laufzeitabhängigkeiten:

- **ffmpeg** (Transkodierung; das Image bringt außerdem VAAPI-Treiber und
  `chromaprint` für die Intro-Erkennung mit).
- **PostgreSQL** (Server und eine Datenbank).
- **Meilisearch** (Suchindex; oder eingebettet über das offizielle Programm).
- **`bwrap`** (bubblewrap) - nur wenn Sie das Sandbox-Backend bubblewrap nutzen
  wollen (siehe unten).
- **`libcap`** (`setcap`) - um die in §4 beschriebene Datei-Capability zu
  vergeben.

## 2. Als eigener Benutzer ohne root-Rechte laufen

**Lassen Sie den Server nie als root laufen.** Legen Sie eine Gruppe und zwei
Benutzer ohne Rechte an, wie es das Docker-Image tut; Plugins werden auf die
UID 1001 herabgestuft (`pluginUID` in `pluginhost/host_server.go`):

```bash
sudo groupadd --gid 1000 evey
sudo useradd --system --uid 1000 --gid evey --shell /usr/sbin/nologin evey
sudo useradd --system --uid 1001 --gid evey --shell /usr/sbin/nologin evey-plugin
```

Die ID der Gruppe zählt: Der Launcher stuft ein Plugin auf die UID 1001 und auf
die eigene Gruppe des Servers herab und verweigert jede ID unter 1000. Eine
Gruppe, die nur von `useradd --system` angelegt wird, erhält eine System-ID
unter 1000, und Plugins im Modus `basic` werden dann verweigert.

- `evey` (UID 1000) führt den Server aus.
- `evey-plugin` (UID 1001) ist die UID, auf die Plugins herabgestuft werden (vom
  Launcher, wenn die UID-Trennung aktiv ist), sodass ein kompromittiertes Plugin
  als anderer Benutzer ohne Rechte landet, der die Geheimnisse des Servers über
  einfache Dateirechte nicht lesen kann. Siehe §4.

Der Server markiert sich beim Start als **nicht dumpbar** (`PR_SET_DUMPABLE=0`,
`cmd/evey/hardening_linux.go`), sodass ein daneben laufendes Plugin
`/proc/<server-pid>/{mem,environ}` nicht auslesen kann.

## 3. Das Sandbox-Backend für Ihren Kernel wählen

Setzen Sie `EVEY_PLUGINS_SANDBOX_MODE`. `auto` (der Standard) löst
**landlock → bubblewrap → basic** auf und wählt das stärkste, das Ihr Host
unterstützt (`resolveAutoMode()` in `runner_landlock.go`). Wählen Sie
ausdrücklich, wenn Sie ein Fail-closed-Verhalten wollen.

### Landlock (empfohlen)

Eine Erlaubnisliste für Dateisystempfade (Plugins sehen nur ihr eigenes
Verzeichnis und das Socket-Verzeichnis, nie Ihre Daten oder Geheimnisse) plus ein
Landlock-v4-Filter für ausgehendes TCP. Braucht **keine Rechte, kein mount,
keine Namespaces** - der Launcher wendet es auf sich selbst an und führt dann das
Plugin per `execve` aus.

**Kernelvoraussetzung** - Landlock muss sowohl einkompiliert *als auch* in der
aktiven LSM-Liste sein. Das `landlockAvailable()` des Runners
(`landlock_probe_linux.go`) prüft `landlock_create_ruleset(VERSION)`; es liefert
einen errno, wenn der Systemaufruf fehlt (Kernel `<5.13`) oder Landlock nicht in
der aktiven LSM-Liste steht. Prüfen:

```bash
uname -r                              # gewünscht >= 5.13
cat /sys/kernel/security/lsm          # muss "landlock" enthalten
```

Fehlt `landlock` in dieser Liste, aktivieren Sie es auf der Kernel-Kommandozeile
und starten Sie neu (die genaue Liste hängt von der Distribution ab - behalten Sie
die vorhandenen Einträge und hängen Sie `landlock` an):

```
# z. B. in GRUB_CMDLINE_LINUX:
lsm=lockdown,capability,yama,apparmor,bpf,landlock
```

Dann:

```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock    # ausdrücklich + fail-closed (verweigert ein Plugin, wenn Landlock nicht durchgesetzt werden kann)
# oder auf dem Standard lassen:
EVEY_PLUGINS_SANDBOX_MODE=auto        # wird zu landlock aufgelöst, wenn verfügbar
```

Der Modus `landlock` übergibt dem Launcher `-require-landlock`, der mit einem Wert
ungleich null endet, statt ein Plugin uneingeschränkt laufen zu lassen, wenn
Landlock nicht durchgesetzt wird.

### bubblewrap

Wenn Sie Landlock nicht bekommen, installieren Sie das Programm `bwrap`.
bubblewrap steckt jedes Plugin in user/pid/uts/ipc/(net)-Namespaces mit einer
Erlaubnisliste für Einbindungen. Es braucht **aktivierte User-Namespaces ohne
Rechte** und den **verfügbaren Systemaufruf `mount`** - auf den meisten Linux-
Systemen ohne Container gegeben, in einem nicht privilegierten Docker-Container
aber blockiert (genau deshalb ist Landlock der Standard im Container). `auto`
wählt bubblewrap nur, wenn Landlock **fehlt** und eine Wegwerf-Probe mit bwrap
tatsächlich gelingt (`ProbeSandbox` / `sandbox_probe.go`).

```bash
EVEY_PLUGINS_SANDBOX_MODE=bubblewrap  # bwrap-Namespaces
# oder:
EVEY_PLUGINS_SANDBOX_MODE=strict      # bubblewrap + der LD_PRELOAD-seccomp-Shim
```

Prüfen Sie, ob der Host die Einrichtung von Namespaces und Einbindungen
tatsächlich erlaubt:

```bash
bwrap --unshare-user --unshare-net --ro-bind / / /bin/true && echo "bwrap OK"
```

### basic (letzter Ausweg)

`EVEY_PLUGINS_SANDBOX_MODE=basic` - UID-Herabstufung plus der LD_PRELOAD-seccomp-
Shim, und der Launcher installiert trotzdem die seccomp-Sperrliste des Kernels
und, soweit vorhanden, Landlock nach bestem Bemühen. Es gibt **keine
Einschränkung pro Dateisystempfad**: Ein Plugin läuft mit UID 1001 und nur mit
seccomp. Nutzen Sie das nur auf alten oder verriegelten Kerneln, auf denen weder
Landlock noch bubblewrap verfügbar ist, und stützen Sie sich zum Schutz der
Geheimnisse auf die UID-Trennung aus §4.

> **Ein Plugin mit der Berechtigung `subprocess` braucht den Launcher**, auch im
> Modus basic. Der LD_PRELOAD-Shim übersteht das `exec` eines Kindprozesses
> nicht, kann also kein Plugin einschränken, das Unterprozesse starten darf -
> nur das Kernel-seccomp des Launchers (direkt vor `exec` installiert und darüber
> hinweg vererbt) kann es. Fehlt `evey-plugin-launcher` (siehe §1), **verweigert**
> ein solches Plugin im Produktivbetrieb **den Start** (fail-closed), statt
> uneingeschränkt zu laufen. Plugins ohne diese Berechtigung fallen wie oben
> beschrieben auf den LD_PRELOAD-Shim zurück.

## 4. UID-Trennung (Geheimnisse ohne root schützen)

Die UID-Trennung führt jedes Plugin unter einem eigenen Benutzer aus (UID 1001,
`evey-plugin`), getrennt vom Server (UID 1000), sodass ein Plugin `jwt.key`,
Sicherungen oder Logs des Servers nicht über einfache Dateirechte (DAC) lesen
kann. Am wichtigsten ist sie im Modus **`basic`** (keine Pfadeinschränkung durch
Landlock); unter Landlock oder bubblewrap ist das Dateisystem bereits
abgeschottet, dort ist sie also redundant.

Gesteuert durch `EVEY_PLUGINS_UID_SEPARATION` (`auto` | `on` | `off`, Standard
`auto`):

- `auto` - nur aktiv, wenn die Sandbox zu `basic` aufgelöst wird. Auf einem
  modernen Kernel ohne Container (Landlock) bleibt sie absichtlich aus.
- `on` - immer, auch unter Landlock/bubblewrap (eine zusätzliche unabhängige
  Schicht).
- `off` - nie.

**Die Herabstufung übernimmt der Launcher, nicht der Server.** Der Server bleibt
ohne root und **ohne** Capabilities; das kleine Programm `evey-plugin-launcher`
trägt eine einzige Datei-Capability und ruft `setuid` auf 1001 direkt vor dem
Start des Plugins auf:

```bash
sudo setcap cap_setuid+ep /opt/evey/evey-plugin-launcher
```

So bleibt der Server völlig ohne Rechte - eine Codeausführung im Server kann
nicht `setuid(0)` aufrufen, weil der Server keine Capabilities hält; nur der
winzige Launcher ohne Eingaben tut es (und er verweigert jede Ziel-UID unter
1000). Kann der Launcher nicht herabstufen (Capability fehlt oder
`no_new_privs` gesetzt), wird ein Plugin im Modus `basic` **verweigert** - die UID
ist dort seine einzige Mauer um die Geheimnisse; unter Landlock wird eine Warnung
protokolliert, und das Plugin läuft mit der UID des Servers.

> **Nicht vereinbar mit `no_new_privs`.** Starten Sie den Server mit
> `no-new-privileges` (oder systemd `NoNewPrivileges=yes`), wird die
> Datei-Capability beim exec ignoriert: Plugins werden im Modus `basic`
> verweigert, und unter Landlock fällt die UID-Trennung auf dieselbe UID zurück
> (mit einer Warnung). Wählen Sie eines von beiden.

Läuft der Server stattdessen **als root** (nicht empfohlen), stuft er Plugins
direkt auf 1001 herab, und die Capability des Launchers wird nicht gebraucht.

Die allgemeinverständliche Anleitung mit dem Bild pro Szenario:
[plugins/sandbox-deployment.md](plugins/sandbox-deployment.md).

## 5. Rechte der geheimen Dateien

Bilden Sie nach, was das Docker-Image tut (`Dockerfile`): Halten Sie die
Verzeichnisse mit Geheimnissen **nur für den Besitzer** zugänglich, damit ein auf
UID 1001 herabgestuftes Plugin sie auch ohne Landlock nicht über einfache
Dateirechte lesen kann.

```bash
sudo chown -R evey:evey /var/lib/evey
# Der SERVER besitzt plugins/ (er entpackt Installationen dorthin); die Gruppe evey - zu der
# evey-plugin gehört - liest und führt die installierten Dateien nur aus:
sudo chmod 750 /var/lib/evey/plugins
# nur Besitzer (weder Gruppe noch andere) auf den geheimen Verzeichnissen:
sudo chmod 700 /var/lib/evey/keys /var/lib/evey/logs /var/lib/evey/backups
# der JWT-Signaturschlüssel selbst:
sudo chmod 600 /var/lib/evey/jwt.key      # (beim ersten Start automatisch mit 0600 erzeugt)
```

(Passen Sie die Pfade daran an, wohin Sie `EVEY_*_DIR` / `EVEY_JWT_PRIVATE_KEY_PATH`
zeigen lassen.)

## 6. Umgebung und Konfiguration

Die gesamte Laufzeitkonfiguration läuft über `EVEY_*`-Umgebungsvariablen.
Setzen Sie Datenbank, Suche, Speicherpfade, Pfad des JWT-Schlüssels und die
Reverse-Proxy-Variablen genauso wie bei der Docker-Bereitstellung - siehe
[configuration.md](configuration.md) für die vollständige Tabelle und
[installation.md](installation.md) für die Anleitung zu Datenbank, Suche und
Reverse Proxy. Die Tabelle wird hier nicht wiederholt. Besonderheiten ohne
Container:

- Richten Sie `EVEY_DB_HOST` / `EVEY_MEILI_URL` auf Ihr lokales PostgreSQL /
  Meilisearch aus.
- Der Server liefert weiterhin **einfaches HTTP** aus - stellen Sie für TLS einen
  Reverse Proxy davor (eine `https://`-Externe-URL und den Proxy unter
  Vertrauenswürdige Proxys, in **Admin → Fernzugriff**, oder
  `EVEY_SERVER_BASE_URL` und `EVEY_TRUSTED_PROXIES`). Siehe
  [reverse-proxy.md](reverse-proxy.md).
- **Der Server öffnet ZWEI Ports, und nur einer darf offenliegen.**
  `EVEY_SERVER_PORT` (Standard `9090`) ist der native gRPC-Listener; die
  HTTP-API, mit der Ihre Clients und Ihr Reverse Proxy sprechen, ist **dieser
  Port plus eins** (`9091`). Richten Sie Reverse Proxy und Firewall nur auf den
  HTTP-Port aus. Der gRPC-Port ist ein interner Transport: Die Netzwerksperre
  für den Fernzugriff, die entscheidet, ob Clients außerhalb Ihres LANs sich
  überhaupt verbinden dürfen, ist in die HTTP-Kette eingebaut und **deckt den
  gRPC-Listener nicht ab**. Deshalb bindet sich der gRPC-Listener standardmäßig
  **nur an Loopback** (`EVEY_GRPC_BIND=127.0.0.1`), sodass ein Betrieb ohne
  Container ihn nicht versehentlich freigeben kann. Haben Sie einen Grund, ihn zu
  veröffentlichen - ein nativer gRPC-Client auf einem anderen Host -, setzen Sie
  `EVEY_GRPC_BIND=0.0.0.0`; der Server warnt beim Start, dass die Sperre dort
  nicht gilt, und die Firewall liegt dann bei Ihnen.
- Migrationen laufen beim Start automatisch; kein eigener Migrationsschritt.
- Die Ressourcengrenzen der Plugins (`EVEY_PLUGINS_MEMORY_MB`, `_PIDS_MAX`,
  `_DISK_MAX_MB`, `_CPU_NICE`) und der Sandbox-Modus lassen sich zur Laufzeit
  unter Admin → Plugins ändern; die Umgebungswerte sind nur die
  Anfangsvorgaben.

## 7. Ihre Sandbox prüfen

Der Server protokolliert das aufgelöste Sandbox-Backend. Prüfen Sie nach dem
Start des ersten Plugins das Server-Log:

- **Auflösung von auto** (einmal protokolliert):
  `plugin sandbox auto-detection complete  selected_mode=landlock`
  (oder `bubblewrap` / `basic`).
- **pro Plugin** (bei jedem Plugin-Start):
  `plugin sandbox applied  sandbox_mode=landlock plugin_id=...`
- **auf basic zurückgefallen** (eine Warnung, die Sie *nicht* sehen wollen, wenn
  Sie Landlock erwartet haben): `plugin sandbox auto-detection fell back to basic mode: no per-path filesystem isolation available …` - bedeutet, dass weder Landlock noch ein
  funktionierendes bwrap gefunden wurde. Prüfen Sie §3 erneut.

Prüfen Sie die Kernelbausteine unabhängig davon:

```bash
cat /sys/kernel/security/lsm          # Landlock aktiv? (sollte "landlock" auflisten)
grep Seccomp: /proc/<plugin-pid>/status   # 2 = Filtermodus bei einem laufenden Plugin aktiv
```

Eine Zeile `Seccomp: 2` bei einem laufenden Plugin-Prozess bestätigt, dass der
seccomp-Filter des Kernels installiert ist. Haben Sie
`EVEY_PLUGINS_SANDBOX_MODE=landlock` festgelegt und Landlock wird nicht
durchgesetzt, **verweigert das Plugin den Start** - das ist der
Fail-closed-Vertrag, der wie vorgesehen greift.

---

Für alles andere (Sicherungen, Updates, Wiederherstellungs-CLI, Fehlerbehebung)
gelten die auf Docker ausgerichteten Abschnitte von
[installation.md](installation.md) ebenso - die Unterbefehle `evey admin` und
die Sicherungsmechanik stecken im selben Programm.
