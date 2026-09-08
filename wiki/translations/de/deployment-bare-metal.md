---
source_sha: 8e943dfd72345f44add4c70011cd163dc77c351f5f410f69d2165ba7c88ea11d
---
# Installation ohne Container (ohne Docker)

Hier geht es darum, Evey direkt auf einem Host zu betreiben (per systemd, über
ein Init-Skript oder von Hand) statt über das
[Docker-Image](installation.md). Empfohlen ist der Weg über das Docker-Image, es
bringt die Sandbox für Erweiterungen, die Härtung und die Laufzeitumgebungen
fertig verdrahtet mit. Verzichten Sie auf Container, müssen **Sie** dieselbe
Ausgangslage herstellen: die richtigen Binärdateien nebeneinander, ein eigener
Benutzer ohne Root-Rechte, das Sandbox-Backend, das Ihr Kernel unterstützt, und
Verzeichnisse für Geheimnisse, die nur dem Eigentümer offenstehen. Diese Seite
führt Sie Schritt für Schritt hindurch.

Wenn Sie einfach nur die bequeme Installation wollen, hören Sie hier auf und
greifen Sie zu [installation.md](installation.md).

## 1. Voraussetzungen

### Binärdateien (aus dem Quellcode bauen)

Bauen Sie beide Go-Binärdateien und **legen Sie sie in dasselbe Verzeichnis**.
Der Runner für Erweiterungen sucht den Launcher zuerst über
`$EVEY_PLUGIN_LAUNCHER`, dann neben der eigenen ausführbaren Datei
(`os.Executable()`) und zuletzt über `$PATH` - beide nebeneinander zu legen ist
deshalb die einfachste korrekte Anordnung.

```bash
# aus dem Wurzelverzeichnis des Repositorys
CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(git describe --tags)" -o evey ./cmd/evey
CGO_ENABLED=0 go build -ldflags="-s -w"                                        -o evey-plugin-launcher ./cmd/evey-plugin-launcher

# nebeneinander installieren, zum Beispiel:
sudo install -m 0755 evey evey-plugin-launcher /opt/evey/
```

Suchreihenfolge in `launcherPath()` (`internal/adapters/driven/pluginrunner/runner_landlock.go`):

1. `$EVEY_PLUGIN_LAUNCHER` (muss eine ausführbare Datei sein) - ausdrückliche
   Übersteuerung.
2. `evey-plugin-launcher` im **selben Verzeichnis** wie die laufende
   `evey`-Binärdatei.
3. `evey-plugin-launcher` irgendwo im `$PATH`.

### Der seccomp-Shim über LD_PRELOAD

Bauen Sie den Shim `libevey-sandbox.so` (eine kleine C-Datei, ein C-Compiler
wird gebraucht):

```bash
gcc -shared -fPIC -nostartfiles -o libevey-sandbox.so sandbox/sandbox.c
sudo install -m 0644 libevey-sandbox.so /app/libevey-sandbox.so
```

Der Runner erwartet ihn unter dem **fest verdrahteten Pfad
`/app/libevey-sandbox.so`** (`sandboxLibPath` in `runner.go`). Fehlt er,
**starten Erweiterungen nicht** (fail-closed) - außer im Entwicklungsmodus.
Dieser Shim ergänzt eine seccomp-Sperrliste für dynamisch gelinkte Erweiterungen
und liegt als weitere Schicht über Landlock beziehungsweise bubblewrap.

> Das Docker-Image legt die quelloffenen Binärdateien, das SDK und diesen Shim
> unter `/app` ab (Geheimnisse liegen dort keine). Ohne Container können Sie
> denselben Pfad nachbilden (`/app/libevey-sandbox.so`) oder einen Symlink
> setzen.

### Laufzeitumgebungen

Installieren Sie alles, was der `entrypoint:` Ihrer Erweiterungen aufruft. Der
Runner kennt diese Interpreter (`knownInterpreters` in `runner.go`): `python3`,
`python`, `node`, `php`, `ruby`, `perl`, `lua`, `bash`, `sh`. Mindestens
gebraucht wird:

- **`python3`** - für die Mehrzahl der Erweiterungen aus der Gemeinschaft.
- **`node`**, **`php`**, **`ruby`**, **`lua`** - nur, wenn Ihre Erweiterungen
  sie verwenden.

Dazu die Abhängigkeiten des Kerns:

- **ffmpeg** (Transkodierung; das Image bringt zusätzlich die VAAPI-Treiber und
  `chromaprint` für die Intro-Erkennung mit).
- **PostgreSQL** (Server und eine Datenbank).
- **Meilisearch** (Suchindex; alternativ eingebettet über die offizielle
  Binärdatei).
- **`bwrap`** (bubblewrap) - nur, wenn Sie bubblewrap als Sandbox-Backend
  einsetzen wollen (siehe unten).
- **`libcap`** (`setcap`) - um die Datei-Capability aus §4 zu vergeben.

## 2. Als eigener Benutzer ohne Root-Rechte betreiben

**Betreiben Sie den Server niemals als root.** Legen Sie zwei Benutzer ohne
Sonderrechte an, deren UIDs zu denen des Plugin-Hosts passen
(`pluginUID = 1001`, `pluginGID = 1001` in `pluginhost/host_server.go`):

```bash
sudo useradd --system --uid 1000 --shell /usr/sbin/nologin evey
sudo useradd --system --uid 1001 --gid evey --shell /usr/sbin/nologin evey-plugin
```

- `evey` (UID 1000) betreibt den Server.
- `evey-plugin` (UID 1001) ist die UID, auf die Erweiterungen herabgestuft
  werden (durch den Launcher, sofern die UID-Trennung aktiv ist). Eine
  kompromittierte Erweiterung landet damit als anderer, unprivilegierter
  Benutzer, der die Geheimnisse des Servers schon über die gewöhnlichen
  Dateirechte nicht lesen kann. Siehe §4.

Der Server markiert sich beim Start als **nicht dumpbar**
(`PR_SET_DUMPABLE=0`, `cmd/evey/hardening_linux.go`), damit eine daneben
laufende Erweiterung nicht `/proc/<server-pid>/{mem,environ}` auslesen kann.

## 3. Das Sandbox-Backend passend zum Kernel wählen

Setzen Sie `EVEY_PLUGINS_SANDBOX_MODE`. Der Standardwert `auto` arbeitet die
Reihe **landlock → bubblewrap → basic** ab und nimmt das stärkste Verfahren, das
Ihr Host beherrscht (`resolveAutoMode()` in `runner_landlock.go`). Wählen Sie
ausdrücklich, wenn Sie fail-closed-Verhalten wollen.

### Landlock (empfohlen)

Eine Positivliste des Dateisystems je Pfad (Erweiterungen sehen nur ihr eigenes
Verzeichnis und das Socket-Verzeichnis, nie Ihre Daten oder Geheimnisse) plus
ein TCP-Egress-Filter nach Landlock v4. Erfordert **keine Sonderrechte, keinen
Mount, keine Namespaces** - der Launcher wendet das Verfahren auf sich selbst an
und ruft danach die Erweiterung per `execve` auf.

**Kernel-Voraussetzung** - Landlock muss einkompiliert *und* in der aktiven
LSM-Liste eingetragen sein. Das `landlockAvailable()` des Runners
(`landlock_probe_linux.go`) probiert `landlock_create_ruleset(VERSION)` aus und
bekommt einen errno zurück, wenn der Systemaufruf fehlt (Kernel `<5.13`) oder
Landlock nicht in der aktiven LSM-Liste steht. So prüfen Sie das:

```bash
uname -r                              # want >= 5.13
cat /sys/kernel/security/lsm          # must include "landlock"
```

Fehlt `landlock` in dieser Liste, aktivieren Sie es über die Kernel-Befehlszeile
und starten neu (die genaue Liste unterscheidet sich je nach Distribution -
behalten Sie die vorhandenen Einträge und hängen Sie `landlock` an):

```
# e.g. in GRUB_CMDLINE_LINUX:
lsm=lockdown,capability,yama,apparmor,bpf,landlock
```

Danach:

```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock    # ausdrücklich und fail-closed (verweigert eine Erweiterung, wenn Landlock nicht durchgesetzt werden kann)
# oder beim Standardwert belassen:
EVEY_PLUGINS_SANDBOX_MODE=auto        # löst sich zu landlock auf, sofern verfügbar
```

Der Modus `landlock` übergibt `-require-landlock` an den Launcher. Der beendet
sich dann mit einem Fehlercode, statt eine Erweiterung unbeschränkt laufen zu
lassen, wenn Landlock nicht durchgesetzt wird.

### bubblewrap

Wenn Landlock partout nicht läuft, installieren Sie die Binärdatei `bwrap`.
bubblewrap steckt jede Erweiterung in Namespaces für user, pid, uts, ipc und
(net) und beschränkt die Mounts über eine Positivliste. Vorausgesetzt werden
**aktivierte unprivilegierte User-Namespaces** und ein **verfügbarer
`mount`-Systemaufruf** - auf den meisten Linux-Systemen ohne Container gegeben,
in einem nicht privilegierten Docker-Container dagegen blockiert (genau deshalb
ist Landlock im Container der Standard). `auto` wählt bubblewrap nur, wenn
Landlock **fehlt** und ein Wegwerf-Test mit bwrap tatsächlich gelingt
(`ProbeSandbox` / `sandbox_probe.go`).

```bash
EVEY_PLUGINS_SANDBOX_MODE=bubblewrap  # bwrap-Namespaces
# oder:
EVEY_PLUGINS_SANDBOX_MODE=strict      # bubblewrap plus den seccomp-Shim über LD_PRELOAD
```

Prüfen Sie zur Sicherheit, ob der Host das Gespann aus Namespaces und Mount
wirklich zulässt:

```bash
bwrap --unshare-user --unshare-net --ro-bind / / /bin/true && echo "bwrap OK"
```

### basic (letzter Ausweg)

`EVEY_PLUGINS_SANDBOX_MODE=basic` - Herabstufung der UID plus der seccomp-Shim
über LD_PRELOAD; der Launcher richtet zusätzlich die seccomp-Sperrliste des
Kernels ein und nimmt Landlock mit, wenn es vorhanden ist. Eine **Beschränkung
des Dateisystems je Pfad gibt es nicht**: Eine Erweiterung läuft unter UID 1001,
abgesichert allein durch seccomp. Greifen Sie dazu nur auf alten oder streng
abgeriegelten Kerneln, auf denen weder Landlock noch bubblewrap zur Verfügung
steht, und stützen Sie den Schutz der Geheimnisse auf die UID-Trennung aus §4.

> **Eine Erweiterung mit der Berechtigung `subprocess` braucht den Launcher**,
> auch im Modus basic. Der LD_PRELOAD-Shim überlebt das `exec` eines
> Kindprozesses nicht und kann eine Erweiterung, die Unterprozesse starten darf,
> deshalb nicht einhegen. Das leistet nur das seccomp des Kernels, das der
> Launcher unmittelbar vor dem `exec` einrichtet und das darüber hinaus vererbt
> wird. Fehlt `evey-plugin-launcher` (siehe §1), **startet eine solche
> Erweiterung im Produktivbetrieb nicht** (fail-closed), statt unbeschränkt zu
> laufen. Erweiterungen ohne diese Berechtigung fallen wie oben beschrieben auf
> den LD_PRELOAD-Shim zurück.

## 4. UID-Trennung (Geheimnisse ohne Root schützen)

Bei der UID-Trennung läuft jede Erweiterung unter einem eigenen Benutzer
(UID 1001, `evey-plugin`), getrennt vom Server (UID 1000). Eine Erweiterung kann
`jwt.key`, Sicherungen oder Protokolle des Servers dann schon über die
gewöhnlichen Dateirechte (DAC) nicht mehr lesen. Am wichtigsten ist das im Modus
**`basic`** (keine Pfadbeschränkung durch Landlock); unter Landlock oder
bubblewrap ist das Dateisystem ohnehin abgeschottet, dort ist die Trennung
überflüssig.

Gesteuert wird sie über `EVEY_PLUGINS_UID_SEPARATION` (`auto` | `on` | `off`,
Standardwert `auto`):

- `auto` - nur dann aktiv, wenn die Sandbox sich zu `basic` auflöst. Auf einem
  aktuellen Kernel ohne Container (Landlock) bleibt sie bewusst aus.
- `on` - immer, auch unter Landlock oder bubblewrap (eine zusätzliche,
  unabhängige Schicht).
- `off` - nie.

**Die Herabstufung nimmt der Launcher vor, nicht der Server.** Der Server bleibt
ohne Root-Rechte und **ohne** jede Capability; die kleine Binärdatei
`evey-plugin-launcher` trägt eine einzige Datei-Capability und setzt sich selbst
per `setuid` auf 1001, unmittelbar bevor sie die Erweiterung per exec startet:

```bash
sudo setcap cap_setuid+ep /opt/evey/evey-plugin-launcher
```

So bleibt der Server vollständig ohne Sonderrechte - eine RCE im Server kann
kein `setuid(0)` ausführen, weil der Server keine Capabilities hält. Die hat nur
der winzige Launcher, der keinerlei Eingaben entgegennimmt (und der jede
Ziel-UID unter 1000 ablehnt). Das Ganze ist auf **bestes Bemühen** angelegt:
Kann der Launcher nicht herabstufen (fehlende Capability oder gesetztes
`no_new_privs`), schreibt er eine Warnung und startet die Erweiterung unter der
UID des Servers, statt abzubrechen. Der Start einer Erweiterung scheitert daran
also nie.

> **Unverträglich mit `no_new_privs`.** Starten Sie den Server mit
> `no-new-privileges` (oder mit `NoNewPrivileges=yes` in systemd), wird die
> Datei-Capability beim exec ignoriert und die UID-Trennung fällt stillschweigend
> auf dieselbe UID zurück (immerhin mit einer Warnung). Entscheiden Sie sich für
> das eine oder das andere.

Läuft der Server stattdessen **als root** (nicht empfohlen), stuft er
Erweiterungen selbst auf 1001 herab und die Capability des Launchers wird nicht
gebraucht.

Die allgemein verständliche Anleitung zeichnet das Bild für jedes einzelne
Szenario: [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md).

## 5. Rechte auf Dateien mit Geheimnissen

Bilden Sie nach, was das Docker-Image tut (`Dockerfile`): Verzeichnisse, die
Geheimnisse enthalten, bleiben **allein dem Eigentümer zugänglich**, damit eine
auf UID 1001 herabgestufte Erweiterung sie auch ohne Landlock über die
gewöhnlichen DAC-Rechte nicht lesen kann.

```bash
sudo chown -R evey:evey /var/lib/evey
# Der SERVER besitzt plugins/ (er entpackt die Installationen dorthin); die
# Gruppe evey - zu der evey-plugin gehört - darf die installierten Dateien nur
# lesen und ausführen:
sudo chmod 750 /var/lib/evey/plugins
# nur der Eigentümer (weder Gruppe noch andere) auf den Geheimnis-Verzeichnissen:
sudo chmod 700 /var/lib/evey/keys /var/lib/evey/logs /var/lib/evey/backups
# der JWT-Signaturschlüssel selbst:
sudo chmod 600 /var/lib/evey/jwt.key      # (wird beim ersten Start automatisch mit 0600 erzeugt)
```

(Passen Sie die Pfade an das an, worauf Sie `EVEY_*_DIR` beziehungsweise
`EVEY_JWT_PRIVATE_KEY_PATH` zeigen lassen.)

## 6. Umgebung und Konfiguration

Die gesamte Laufzeitkonfiguration läuft über Umgebungsvariablen mit dem Präfix
`EVEY_*`. Datenbank, Suche, Speicherpfade, Pfad zum JWT-Schlüssel und die
Variablen für den Reverse Proxy setzen Sie genauso wie beim Betrieb mit Docker -
die vollständige Tabelle finden Sie in
[configuration.md](configuration.md), die Anleitung zu Datenbank, Suche und
Reverse Proxy in [installation.md](installation.md). Die Tabelle wird hier
bewusst nicht wiederholt. Diese Punkte gelten eigens für den Betrieb ohne
Container:

- Lassen Sie `EVEY_DB_HOST` / `EVEY_MEILI_URL` auf Ihr lokales PostgreSQL
  beziehungsweise Meilisearch zeigen.
- Der Server liefert weiterhin **unverschlüsseltes HTTP** aus - setzen Sie für
  TLS einen Reverse Proxy davor (`EVEY_SERVER_BASE_URL` mit dem Schema
  `https://`, dazu `EVEY_TRUSTED_PROXIES`). Siehe
  [reverse-proxy.md](reverse-proxy.md).
- **Der Server öffnet ZWEI Ports, und nur einer davon darf nach außen sichtbar
  sein.** `EVEY_SERVER_PORT` (Standardwert `9090`) ist der native
  gRPC-Listener; die HTTP-API, mit der Ihre Clients und Ihr Reverse Proxy
  sprechen, liegt auf **diesem Port plus eins** (`9091`). Richten Sie Reverse
  Proxy und Firewall ausschließlich auf den HTTP-Port. Der gRPC-Port ist ein
  interner Transportweg: Die Netzwerksperre für den Fernzugriff, die darüber
  entscheidet, ob Clients außerhalb Ihres LANs überhaupt eine Verbindung
  bekommen, hängt in der HTTP-Kette und **deckt den gRPC-Listener nicht ab**.
  Deshalb lauscht der gRPC-Listener standardmäßig **nur auf Loopback**
  (`EVEY_GRPC_BIND=127.0.0.1`), sodass ein Betrieb ohne Container ihn nicht aus
  Versehen nach außen öffnen kann. Wenn Sie einen Grund haben, ihn zu
  veröffentlichen - etwa einen nativen gRPC-Client auf einem anderen Host -,
  setzen Sie `EVEY_GRPC_BIND=0.0.0.0`. Der Server warnt beim Start, dass die
  Sperre dafür nicht greift, und das Absichern per Firewall liegt dann bei
  Ihnen.
- Migrationen laufen beim Start automatisch, ein eigener Migrationsschritt
  entfällt.
- Die Ressourcengrenzen für Erweiterungen (`EVEY_PLUGINS_MEMORY_MB`,
  `_PIDS_MAX`, `_DISK_MAX_MB`, `_CPU_NICE`) und der Sandbox-Modus lassen sich
  zur Laufzeit unter Admin → Plugins anpassen; die Werte aus der Umgebung sind
  lediglich die Ausgangswerte.

## 7. Die Sandbox überprüfen

Der Server protokolliert, auf welches Sandbox-Backend er sich festgelegt hat.
Sehen Sie nach dem Start der ersten Erweiterung ins Serverprotokoll:

- **Auflösung von auto** (einmal protokolliert):
  `plugin sandbox auto-detection complete  selected_mode=landlock`
  (oder `bubblewrap` / `basic`).
- **je Erweiterung** (bei jedem Start einer Erweiterung):
  `plugin sandbox applied  sandbox_mode=landlock plugin_id=...`
- **Rückfall auf basic** (eine Warnung, die Sie *nicht* sehen wollen, wenn Sie
  mit Landlock gerechnet haben): `plugin sandbox auto-detection fell back to
  basic mode: no per-path filesystem isolation available …` - das heißt, es
  wurde weder Landlock noch ein funktionierendes bwrap gefunden. Prüfen Sie
  §3 noch einmal.

Überzeugen Sie sich unabhängig davon von den Bausteinen im Kernel:

```bash
cat /sys/kernel/security/lsm          # Landlock active? (should list "landlock")
grep Seccomp: /proc/<plugin-pid>/status   # 2 = filter mode active on a running plugin
```

Eine Zeile `Seccomp: 2` bei einem laufenden Erweiterungsprozess belegt, dass der
seccomp-Filter des Kernels eingerichtet ist. Haben Sie
`EVEY_PLUGINS_SANDBOX_MODE=landlock` festgelegt und Landlock wird nicht
durchgesetzt, **startet die Erweiterung nicht** - so soll der
fail-closed-Vertrag genau funktionieren.

---

Für alles Weitere (Sicherungen, Aktualisierungen, Wiederherstellung über die
CLI, Fehlersuche) gelten die auf Docker gemünzten Abschnitte aus
[installation.md](installation.md) gleichermaßen - die Unterbefehle von
`evey admin` und die Mechanik der Sicherungen stecken in derselben Binärdatei.
