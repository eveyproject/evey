---
source_sha: 0b2dda412b186b3b40f33ea4027ceb0d328e5f8f39f87d2312a8c7bdab2edbed
reviewed: true
---
# Ohne Container

Wie Sie Evey direkt auf einem Host betreiben (systemd, ein Init-Skript oder
von Hand) statt mit dem [Docker-Image](installation.md). Das Image ist der
empfohlene Weg: Es bringt die Plugin-Sandbox, die Härtung und die
Laufzeitumgebungen für Plugins fertig eingerichtet mit. Ohne Container müssen
Sie diesen Aufbau selbst nachbilden: die richtigen Programme nebeneinander,
einen eigenen Benutzer ohne root-Rechte, das Sandbox-Verfahren, das Ihr Kernel
unterstützt, und Verzeichnisse für Geheimnisse, die nur ihr Besitzer lesen
kann. Diese Seite geht das der Reihe nach durch.

## Die Programme bauen und installieren

Bauen Sie die beiden Go-Programme und installieren Sie sie im selben
Verzeichnis: Der Plugin-Runner sucht den Launcher zuerst in
`$EVEY_PLUGIN_LAUNCHER`, dann neben seinem eigenen Programm, dann im `$PATH`.

```bash
# vom Wurzelverzeichnis des Repositorys aus
CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(git describe --tags --always) -X main.commit=$(git rev-parse HEAD)" -o evey ./cmd/evey
CGO_ENABLED=0 go build -ldflags="-s -w" -o evey-plugin-launcher ./cmd/evey-plugin-launcher
sudo install -m 0755 evey evey-plugin-launcher /opt/evey/

# optional: Shell-Vervollständigung und Handbuchseite, beide vom Programm ausgegeben
/opt/evey/evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
/opt/evey/evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null
```

`evey version` gibt Version, Commit und Build-Datum aus, ganz ohne
Konfiguration.

### Die seccomp-Zwischenschicht

Bauen Sie `libevey-sandbox.so` (eine C-Datei, braucht einen C-Compiler) und
installieren Sie sie unter dem Pfad, den der Runner erwartet,
`/app/libevey-sandbox.so`:

```bash
gcc -shared -fPIC -nostartfiles -o libevey-sandbox.so sandbox/sandbox.c
sudo install -D -m 0644 libevey-sandbox.so /app/libevey-sandbox.so
```

Der Pfad ist im Runner fest eingetragen. Ohne die Datei weigern sich Plugins
zu starten (fail closed), es sei denn, der Server läuft im
Entwicklermodus. Die Zwischenschicht fügt für dynamisch gelinkte Plugins eine
seccomp-Sperrliste hinzu, zusätzlich zu Landlock oder bubblewrap. Das Image
legt sie mit dem SDK unter `/app` ab; ein symbolischer Link funktioniert auch.

### Laufzeitumgebungen und Dienste

Installieren Sie, was der `entrypoint:` Ihrer Plugins aufruft. Der Runner
erkennt `python3`, `python`, `node`, `php`, `ruby`, `perl`, `lua`, `bash` und
`sh`; `python3` deckt die meisten Community-Plugins ab. Dann die Dienste, mit
denen der Server kommuniziert:

| Abhängigkeit | Wozu |
|---|---|
| **ffmpeg** und **ffprobe** | Transkodierung, Analyse der Dateien; das Image bringt außerdem VAAPI-Treiber und `fpcalc` (chromaprint) für die Intro-Erkennung mit |
| **PostgreSQL** (Server und eine Datenbank) und **`pg_dump`** | die Datenbank; `pg_dump` schreibt den automatischen Dump vor jeder Migration |
| **Meilisearch** | der Suchindex; ohne ihn weicht die Suche auf SQL aus |
| **`bwrap`** (bubblewrap) | nur für das Sandbox-Verfahren bubblewrap |
| **`setcap`** (libcap) | um dem Launcher seine Datei-Capability zu geben |

Die eingebettete Datenbank und Suchmaschine sind eine Funktion des
Startskripts im Image, nicht des Programms: Ohne Container betreiben Sie beide
selbst und richten `EVEY_DB_HOST` und `EVEY_MEILI_URL` auf sie.

## Unter einem eigenen Benutzer ohne root-Rechte laufen

Starten Sie den Server nie als root. Legen Sie eine Gruppe und zwei Benutzer
ohne Sonderrechte an, wie das Image es tut:

```bash
sudo groupadd --gid 1000 evey
sudo useradd --system --uid 1000 --gid evey --shell /usr/sbin/nologin evey
sudo useradd --system --uid 1001 --gid evey --shell /usr/sbin/nologin evey-plugin
```

- `evey` (UID 1000) führt den Server aus.
- `evey-plugin` (UID 1001) ist der Benutzer, zu dem Plugins wechseln, wenn die
  UID-Trennung aktiv ist. Ein kompromittiertes Plugin läuft so als ein
  anderer Benutzer ohne Sonderrechte, der die Geheimnisse des Servers schon
  wegen der normalen Dateirechte nicht lesen kann.

!!! warning
    Die Gruppen-ID ist wichtig: Der Launcher lässt ein Plugin zu UID 1001 und
    zur eigenen Gruppe des Servers wechseln und lehnt jede UID oder GID unter
    1000 ab. Eine Gruppe, die nur durch `useradd --system` entsteht, bekommt
    eine System-ID unter 1000, und Plugins im Modus `basic` werden dann
    abgelehnt.

Der Server markiert sich beim Start als nicht auslesbar
(`PR_SET_DUMPABLE=0`). Ein Plugin auf demselben Rechner kann daher
`/proc/<server-pid>/{mem,environ}` nicht lesen.

## Das Sandbox-Verfahren für Ihren Kernel wählen

Setzen Sie `EVEY_PLUGINS_SANDBOX_MODE`. `auto` (der Standard) prüft der Reihe
nach landlock, bubblewrap, basic und wählt das stärkste Verfahren, das der
Host unterstützt. Setzen Sie einen Modus ausdrücklich, wenn Sie ein
fail-closed-Verhalten wollen.

### Landlock (empfohlen)

Eine Positivliste für das Dateisystem, Pfad für Pfad (ein Plugin sieht sein
eigenes Verzeichnis und das Socket-Verzeichnis, nie Ihre Daten oder
Geheimnisse), dazu ein Landlock-v4-Filter für ausgehende TCP-Verbindungen. Es
braucht keine Sonderrechte, kein Mount und keinen Namespace: Der Launcher
wendet es auf sich selbst an und führt dann das Plugin aus.

Landlock muss in den Kernel einkompiliert sein und in der Liste der aktiven
LSM stehen. Der Runner prüft `landlock_create_ruleset`. Die Prüfung schlägt
fehl, wenn der Systemaufruf fehlt (Kernel älter als 5.13) oder wenn Landlock
nicht in der aktiven Liste steht. Prüfen Sie:

```bash
uname -r                              # 5.13 oder neuer
cat /sys/kernel/security/lsm          # muss "landlock" enthalten
```

Fehlt `landlock` in dieser Liste, fügen Sie es auf der Kernel-Befehlszeile
hinzu und starten Sie neu (behalten Sie die vorhandenen Einträge, die Liste
unterscheidet sich je nach Distribution):

```
# z. B. in GRUB_CMDLINE_LINUX:
lsm=lockdown,capability,yama,apparmor,bpf,landlock
```

Dann behalten Sie entweder `auto` oder legen den Modus fest:

```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock    # fail closed: ein Plugin wird abgelehnt, wenn Landlock nicht durchgesetzt werden kann
```

`landlock` übergibt dem Launcher `-require-landlock`. Der Launcher beendet
sich dann mit einem Fehlercode, statt ein Plugin ohne Isolierung zu starten.

### bubblewrap

Wenn Landlock nicht verfügbar ist, installieren Sie `bwrap`. Es setzt jedes
Plugin in eigene user-, pid-, uts-, ipc- und Netzwerk-Namespaces, mit einer
Positivliste für Mounts. Es braucht User-Namespaces ohne Sonderrechte und den
Systemaufruf `mount`. Das ist auf den meisten Linux-Systemen ohne Container
gegeben, aber in einem Container ohne Sonderrechte gesperrt. Deshalb ist
Landlock der Standard im Container. `auto` wählt bubblewrap nur, wenn Landlock
fehlt und ein einmaliger Testlauf von `bwrap` gelingt.

```bash
EVEY_PLUGINS_SANDBOX_MODE=bubblewrap  # Namespaces
EVEY_PLUGINS_SANDBOX_MODE=strict      # Namespaces plus die seccomp-Zwischenschicht
```

Prüfen Sie, ob der Host die Einrichtung der Namespaces und Mounts erlaubt:

```bash
bwrap --unshare-user --unshare-net --ro-bind / / /bin/true && echo "bwrap OK"
```

### basic (letzte Möglichkeit)

`EVEY_PLUGINS_SANDBOX_MODE=basic` ist ein Wechsel der UID plus die
seccomp-Zwischenschicht. Ist der Launcher vorhanden, installiert er trotzdem
seine seccomp-Sperrliste im Kernel und, so weit möglich, Landlock. Es gibt
keine Beschränkung des Dateisystems pro Pfad: Ein Plugin läuft mit UID
1001 und nur mit seccomp. Verwenden Sie diesen Modus auf alten oder
stark eingeschränkten Kerneln, auf denen weder Landlock noch bubblewrap funktioniert,
und verlassen Sie sich auf die UID-Trennung unten, um die Geheimnisse zu
schützen.

Ein Plugin mit der Berechtigung `subprocess` braucht den Launcher auch im
Modus `basic`: Die Zwischenschicht überlebt das `exec` eines Kindprozesses
nicht, nur das seccomp des Launchers im Kernel tut das. Ohne den Launcher
weigert sich ein solches Plugin zu starten, statt ohne Isolierung zu laufen.

## Geheimnisse außer Reichweite der Plugins halten

Die UID-Trennung startet jedes Plugin unter `evey-plugin` (UID 1001), getrennt
vom Server (UID 1000). So kann ein Plugin `jwt.key`, die Sicherungen oder die
Protokolle schon wegen der normalen Dateirechte nicht lesen. Das zählt im
Modus `basic`. Unter Landlock oder bubblewrap ist das Dateisystem bereits
beschränkt, dort ist die Trennung also redundant.

`EVEY_PLUGINS_UID_SEPARATION`:

| Wert | Verhalten |
|---|---|
| `auto` (Standard) | nur an, wenn die Sandbox zu `basic` aufgelöst wird |
| `on` | immer, als zusätzliche, unabhängige Schicht |
| `off` | nie |

Den Wechsel übernimmt der Launcher, nicht der Server. Der Server bleibt ohne
root-Rechte und ohne Capability. Der Launcher trägt eine Datei-Capability und
wechselt kurz vor dem Start des Plugins zu UID 1001:

```bash
sudo setcap cap_setuid+ep /opt/evey/evey-plugin-launcher
```

Der Launcher lehnt jede Ziel-UID oder -GID unter 1000 ab. Kann er nicht
wechseln (Capability fehlt, `no_new_privs` gesetzt), wird ein Plugin im Modus
`basic` abgelehnt, weil die UID dort sein einziger Schutz ist. Unter
Landlock schreibt er eine Warnung ins Protokoll und startet das Plugin mit der
UID des Servers.

!!! warning
    Die UID-Trennung ist nicht mit `no_new_privs` vereinbar. Läuft der
    Server unter systemd mit `NoNewPrivileges=yes`, wird die
    Datei-Capability beim exec ignoriert: Plugins werden im Modus `basic`
    abgelehnt, und unter Landlock fällt die UID-Trennung auf dieselbe UID
    zurück. Entscheiden Sie sich für eines von beiden.

Läuft der Server stattdessen als root (nicht empfohlen), lässt er Plugins
direkt zu 1001 wechseln, und die Capability wird nicht gebraucht. Die Übersicht in
einfachen Worten, Fall für Fall, steht unter
[Plugin-Sandbox einrichten](plugins/sandbox-deployment.md).

## Dateirechte für Geheimnisse

Machen Sie es wie das Image: Verzeichnisse mit Geheimnissen sind nur für ihren
Besitzer zugänglich. So kann ein Plugin mit UID 1001 sie auch ohne Landlock
nicht lesen.

```bash
sudo chown -R evey:evey /var/lib/evey
# plugins/ gehört dem Server (er entpackt Installationen dorthin); die Gruppe evey,
# zu der evey-plugin gehört, liest und führt die installierten Dateien nur aus:
sudo chmod 750 /var/lib/evey/plugins
# nur für den Besitzer, auf den Verzeichnissen mit Geheimnissen:
sudo chmod 700 /var/lib/evey/keys /var/lib/evey/logs /var/lib/evey/backups
# der JWT-Signaturschlüssel selbst (beim ersten Start mit 0600 erzeugt):
sudo chmod 600 /var/lib/evey/jwt.key
```

Passen Sie die Pfade an die Orte an, auf die Sie die Variablen `EVEY_*_DIR`
und `EVEY_JWT_PRIVATE_KEY_PATH` richten.

## Die Umgebung konfigurieren

Die gesamte Konfiguration besteht aus `EVEY_*`-Umgebungsvariablen; die
[Konfigurationsreferenz](configuration.md) hat die vollständige Tabelle.
Punkte, die nur ohne Container gelten:

- Richten Sie `EVEY_DB_HOST` und `EVEY_MEILI_URL` auf Ihr PostgreSQL und Ihr
  Meilisearch. `EVEY_DB_PASSWORD` darf nicht der Standard `evey` sein.
- Die Go-Standardwerte für Pfade wurden für den Container gewählt: Setzen Sie
  `EVEY_LOG_DIR` (Standard ist das relative `logs`), `EVEY_BACKUP_DIR`
  (`/data/backups`), `EVEY_TRANSCODING_CACHE_DIR` (`/data/transcode`),
  `EVEY_TRICKPLAY_CACHE_DIR` (`/cache/trickplay`) und
  `EVEY_JWT_PRIVATE_KEY_PATH` (`/data/jwt.key`) auf Ihre eigenen
  Verzeichnisse.
- Der Server liefert einfaches HTTP aus: Stellen Sie für TLS einen Reverse
  Proxy davor, mit einer `https://`-**Externe URL** und dem Proxy unter
  **Vertrauenswürdige Proxys** (**Verwaltung → Fernzugriff**;
  `EVEY_SERVER_BASE_URL` und `EVEY_TRUSTED_PROXIES` sind der Ersatz, wenn dort
  nichts eingetragen ist). Siehe [Reverse Proxy](reverse-proxy.md).
- **Der Server öffnet zwei Ports, und nur einer darf erreichbar sein.**
  `EVEY_SERVER_PORT` (Standard `9090`) ist der eigentliche gRPC-Listener. Die
  HTTP-API, mit der Ihre Clients und Ihr Reverse Proxy kommunizieren, liegt auf
  diesem Port plus eins (`9091`). Die Sperre für den Fernzugriff, die
  entscheidet, ob Clients außerhalb Ihres LAN sich verbinden dürfen, sitzt in
  der HTTP-Kette und gilt nicht für den gRPC-Listener. Deshalb lauscht dieser
  standardmäßig nur auf Loopback (`EVEY_GRPC_BIND=127.0.0.1`). Wenn Sie einen
  Grund haben, ihn zu veröffentlichen, setzen Sie `EVEY_GRPC_BIND=0.0.0.0`.
  Der Server warnt beim Start, dass die Sperre für ihn nicht gilt, und den
  Schutz per Firewall übernehmen dann Sie.
- Die Migrationen laufen beim Start; es gibt keinen eigenen Migrationsschritt.
- Die Ressourcengrenzen der Plugins (`EVEY_PLUGINS_MEMORY_MB`, `_PIDS_MAX`,
  `_DISK_MAX_MB`, `_CPU_NICE`) und der Sandbox-Modus lassen sich im laufenden
  Betrieb unter **Verwaltung → Plugins → Ressourcenlimits** ändern. Die Werte
  aus der Umgebung sind die Anfangswerte.
- Der Serverneustart aus dem Verwaltungsbereich braucht den Supervisor des
  Images und meldet sich ohne Container als nicht verfügbar: Starten Sie den
  Dienst selbst neu.

## Die Sandbox prüfen

Der Server schreibt das gewählte Verfahren ins Protokoll. Suchen Sie nach dem
Start des ersten Plugins nach:

- einmal, bei `auto`: `plugin sandbox auto-detection complete
  selected_mode=landlock` (oder `bubblewrap`, `basic`);
- bei jedem Plugin-Start: `plugin sandbox applied sandbox_mode=landlock
  plugin_id=...`;
- die Warnung, die Sie nicht sehen wollen, wenn Sie Landlock erwartet haben:
  `plugin sandbox auto-detection fell back to basic mode: no per-path
  filesystem isolation available ...`. Sie bedeutet, dass weder Landlock noch
  ein funktionierendes `bwrap` gefunden wurde. Prüfen Sie den Abschnitt zu den
  Verfahren oben erneut.

Prüfen Sie die Seite des Kernels unabhängig davon:

```bash
cat /sys/kernel/security/lsm              # listet "landlock"
grep Seccomp: /proc/<plugin-pid>/status   # 2 = ein seccomp-Filter ist für das Plugin aktiv
```

Eine Zeile `Seccomp: 2` bei einem laufenden Plugin-Prozess bestätigt, dass der
Kernel-Filter installiert ist. Mit `EVEY_PLUGINS_SANDBOX_MODE=landlock` und
nicht durchgesetztem Landlock weigert sich das Plugin zu starten: So
funktioniert die fail-closed-Zusage.

Für Sicherungen, Updates und die Befehle zur Wiederherstellung gelten die
Seiten für den Container unverändert: [Sicherungen und Updates](backups.md),
[Wiederherstellung](runbooks/recovery.md) und die
[Kommandozeile](runbooks/cli.md) beschreiben dasselbe Programm `evey admin`.

<!-- sources
launcher lookup: internal/adapters/driven/pluginrunner/runner_landlock.go - launcherPath, launcherBinaryName, resolveAutoMode
shim path and modes: internal/adapters/driven/pluginrunner/runner.go - sandboxLibPath, sandboxModeStrict, pluginUID ; sandbox/sandbox.c ; Dockerfile (gcc line, /app/libevey-sandbox.so)
interpreters: internal/adapters/driven/pluginrunner/runner_start.go - knownInterpreters
basic sandbox, subprocess refusal, uid separation policy: internal/adapters/driven/pluginrunner/runner_sandbox.go - applyBasicSandbox, pluginDropArgs, uidSepApplies, WithUIDSeparation, effectiveMode ("auto-detection complete")
landlock probe: internal/adapters/driven/pluginrunner/landlock_probe_linux.go - landlockAvailable ; bwrap probe: sandbox_probe.go - ProbeSandbox
launcher: cmd/evey-plugin-launcher/main.go (-require-landlock, -uid, -uid-besteffort) ; cmd/evey-plugin-launcher/droppriv_linux.go (uid/gid >= 1000)
hardening: cmd/evey/hardening_linux.go - PR_SET_DUMPABLE
cli: cmd/evey/cli.go (version, completion bash, man)
ports and defaults: internal/config/config.go (EVEY_SERVER_PORT, EVEY_GRPC_BIND, EVEY_LOG_DIR, EVEY_BACKUP_DIR, EVEY_TRANSCODING_CACHE_DIR, EVEY_TRICKPLAY_CACHE_DIR, EVEY_JWT_PRIVATE_KEY_PATH, EVEY_PLUGINS_*, hardFail on password "evey") ; cmd/evey/main.go - httpPort = Port+1
pre-migration dump needs pg_dump: internal/adapters/driven/postgres/migrate_backup.go - dumpDatabase
restart RPC needs the supervisor: docker/entrypoint.sh (EVEY_SUPERVISED)
image permissions: Dockerfile (chmod 750 /data/plugins, chmod 700 keys/logs/backups, setcap)
labels: i18n/en/admin.yaml - remoteAccess.externalUrl, remoteAccess.trustedProxies, plugins.resourceLimits.title
-->
