---
source_sha: c781817bb39af138ca65b5a030c42b0e6e199a7cd2613b2415b3dcba8e7427be
---
# Installation

Evey läuft als **ein einziger Container**: Das Image liefert API und
Weboberfläche auf einem Port aus und betreibt Postgres und Meilisearch in sich
selbst, als Serverbenutzer ohne root, mit beim ersten Start erzeugten
Geheimnissen. Die Compose-Datei in Abschnitt 1 ist die ganze Installation. Sie
braucht keine `.env`, kein `--privileged` und keine Einstellung für Speicher
oder CPU.

> Die `docker-compose.yml` im Wurzelverzeichnis des Git-Repositorys ist **nicht**
> diese Datei. Sie ist das Betreiber-Layout (Abschnitt 7): Postgres als
> eigener Dienst, ein mitgeliefertes Caddy und eine `.env`, ohne die sie nicht
> startet. Kopieren Sie den Block unten, nicht die Datei des Repositorys.

Um Evey ohne Docker zu betreiben, siehe [deployment-bare-metal.md](deployment-bare-metal.md).

## Voraussetzungen

- Docker + Docker Compose v2 (`docker compose`, nicht das alte `docker-compose`).
- Ein Ort zum Speichern der Daten auf einer dauerhaften Platte (die Compose-Datei
  unten nutzt Bind-Mounts im Arbeitsverzeichnis).
- **Empfohlen:** ein Reverse Proxy für HTTPS - Evey liefert nur einfaches HTTP
  aus. Siehe [reverse-proxy.md](reverse-proxy.md).

## 1. Die Compose-Datei

Speichern Sie dies als `docker-compose.yml`, mit Ihrem Medienpfad in der letzten
Volume-Zeile. Die Geheimnisse landen beim ersten Start in `./evey-data/keys`;
nichts muss erfunden werden.

```yaml
services:
  evey:
    image: gravityz/evey:latest        # pin a tag here (e.g. 0.2.0-beta.1, sha-1a2b3c4)
    container_name: evey-server
    ports:
      - "9091:9091"
    # environment:
    #   EVEY_SERVER_BASE_URL: http://<your-ip>:9091   # e-mail links, OIDC redirects, cast/device URLs
    volumes:
      - ./evey-data:/data              # database, JWT key, generated secrets, search index, logs, backups
      - evey-cache:/cache              # transcode + trickplay cache (safe to prune)
      # Read-write so the seek-index repair rebuilds files in place; append
      # `:ro` for a read-only library and accept that those repairs fail.
      - /path/to/your/media:/media
    # The embedded database is stopped cleanly AFTER the server on `docker
    # stop`; a large catalogue needs more than Docker's 10s default.
    stop_grace_period: 60s
    restart: unless-stopped

volumes:
  evey-cache:
```

Halten Sie die Umgebung **minimal** - nur Geheimnisse, die Sie selbst
mitbringen, die Datenbankverbindung, wenn sie extern ist, Speicherpfade und
Angaben zum Reverse Proxy gehören in die Compose-Datei (oder für die Geheimnisse
in eine `.env`). Fast alles andere (Servername, gleichzeitige Transkodierungen,
Ratenbegrenzungen, Log-Level, Intervalle für Scan/Überwachung, Zeitplan der
Sicherungen, API-Schlüssel) lässt sich **zur Laufzeit unter Admin →
Serverkonfiguration ändern** und gehört NICHT in die Compose-Datei gestopft.
`PUID`/`PGID`: standardmäßig nicht gesetzt → der Einstiegspunkt übernimmt den
Besitzer Ihrer `/media`-Mounts, damit Reparaturen an Ort und Stelle
funktionieren; überschreiben Sie es mit `id -u`/`id -g`. Der Container startet
nur als root, um die Zuordnung anzuwenden, und gibt dann seine Privilegien ab -
der Server läuft nie als root.

**Auf einem NAS (Synology Container Manager, QNAP Container Station):**

- Legen Sie den Datenordner **vor** dem ersten Start an (`mkdir evey-data` neben
  der Compose-Datei oder über die File Station): Diese Oberflächen verweigern
  einen Bind-Mount, dessen Ordner nicht existiert, anders als die Docker-CLI.
  Sonst ist nichts vorab anzulegen - der Einstiegspunkt erstellt `plugins/`,
  `keys/`, `postgres/` selbst.
- Setzen Sie `PUID`/`PGID` auf den DSM-Benutzer, dem die Medien gehören
  (`id <user>` per SSH; bei Synology typischerweise um `1026` und Gruppe `100` =
  `users`), statt sich auf die automatische Erkennung zu verlassen: Eine
  DSM-Freigabe gehört oft `root`, den der Einstiegspunkt nie übernimmt.
- Intel-NAS mit Quick Sync: Fügen Sie die Zeilen `devices:` und `group_add:` aus
  `docker-compose.gpu.yml` hinzu (die GID der Render-Gruppe liefert
  `getent group render` oder `stat -c %g /dev/dri/renderD128`).
- Es ist keine `.env` nötig - die `.env`-pflichtige `docker-compose.yml` im
  Wurzelverzeichnis des Repositorys ist das Betreiber-Layout, nicht dieses.

## 2. Erster Start

```bash
docker compose up -d
docker compose logs -f evey
```

Beim ersten Start:

1. **erzeugt der Server den JWT-Signierschlüssel** automatisch unter
   `EVEY_JWT_PRIVATE_KEY_PATH` (`/data/jwt.key`, in `./evey-data`). Sie legen ihn
   nicht von Hand an. Bewahren Sie `evey-data` gut auf - wer den Schlüssel
   verliert, meldet alle ab.
2. **führt er die Datenbankmigrationen automatisch aus** (eingebaut, beim Start
   angewandt; kein eigener Migrationsschritt). Siehe
   [runbooks/migration-runbook.md](runbooks/migration-runbook.md).

Öffnen Sie dann `http://<ihre-ip>:9091`. Solange es keinen Administrator gibt,
landen Sie im **Einrichtungsassistenten** (`/setup`): Legen Sie das erste
Administratorkonto an. Melden Sie sich danach an und **fügen Sie eine Bibliothek
hinzu** (Admin → Bibliotheken), die auf einen Pfad unter `/media` zeigt (Ihr
`MEDIA_DIR`, mit Schreibzugriff eingehängt). Der erste Scan startet automatisch.
Assistent, Bibliotheken, das Einladen von Menschen und was Sie vor dem Teilen
der Adresse prüfen sollten, stehen in den [ersten Schritten](guide/first-steps.md).

Die Compose-Datei oben hängt sie absichtlich mit Schreibzugriff ein: Die
Reparatur des Sprungindex baut Dateien an Ort und Stelle neu, und ein
`:ro`-Mount lässt diese Reparaturen scheitern. Fügen Sie `:ro` nur hinzu, wenn
Sie auf sie verzichten wollen.

## 3. Häufige Varianten

**Varianten, jeweils ein paar Zeilen in dieser Datei:**

- *Ein zweiter Medienordner*: fügen Sie unter `volumes:` `- /other/path:/media2`
  hinzu (`/media3`, ... ebenso). Hängen Sie sie mit Schreibzugriff ein, siehe den
  Kommentar oben.
- *Hinter einem Reverse Proxy, den Sie schon betreiben* (Nginx Proxy Manager,
  Traefik, ... in einem Docker-Netz): entfernen Sie `ports:`, treten Sie dem Netz
  des Proxys bei und nennen Sie dem Server seine öffentliche URL und den Nachbarn,
  dem er vertrauen soll:
  ```yaml
  services:
    evey:
      environment:
        EVEY_SERVER_BASE_URL: https://media.example.com
        EVEY_TRUSTED_PROXIES: 172.16.0.0/12     # narrow to the proxy's subnet once known
  networks:
    default:
      name: proxy-net        # the network your proxy is on
      external: true
  ```
  Der Proxy zielt dann auf `evey-server:9091`. Einstellungen je Proxy:
  [reverse-proxy.md](reverse-proxy.md).
- *HTTPS ohne bestehenden Proxy*: fügen Sie den mitgelieferten Caddy-Dienst aus
  [reverse-proxy.md](reverse-proxy.md) hinzu (automatisches Let's-Encrypt-Zertifikat).
- *Postgres oder Meilisearch als eigene Dienste*: Abschnitt 7.

**HTTPS und Reverse Proxys.** Evey liefert auf 9091 einfaches HTTP aus. Ohne
Proxy besorgt der mitgelieferte Caddy-Dienst aus
[reverse-proxy.md](reverse-proxy.md) Ihnen ein Let's-Encrypt-Zertifikat (die
Ports 80/443 müssen den Host erreichen). Mit Ihrem eigenen Proxy (Nginx,
Traefik, Nginx Proxy Manager): Setzen Sie `EVEY_SERVER_BASE_URL` auf Ihre
öffentliche `https://`-URL (dieses Schema schaltet Secure-Cookies und HSTS ein)
und vertrauen Sie dem Proxy mit `EVEY_TRUSTED_PROXIES`. Streaming-Endpunkte
brauchen ausgeschaltetes Puffern und kein Timeout - die fertigen Konfigurationen
erledigen das. Vollständige Setups: [reverse-proxy.md](reverse-proxy.md).

## 4. Evey aktualisieren

```bash
docker compose pull && docker compose up -d
docker compose logs -f evey      # until "healthy" (a migration on a large catalogue takes minutes)
```

Mehr ist nicht nötig. Das neue Image wendet beim Start ausstehende Migrationen an
und sichert vorher die Datenbank (`./evey-data/backups/pre-migration-*`), die
Gesundheitsprüfung des Containers wird grün, sobald er ausliefert, und `up -d`
startet nur den Dienst `evey` neu - eine benachbarte Datenbank oder ein Proxy
wird nie angefasst.

**Zurückgehen.** Setzen Sie den vorherigen Tag in die `image:`-Zeile (oder in
`EVEY_VERSION`, wenn Ihre Compose-Datei ihn einsetzt) und führen Sie erneut
`docker compose up -d` aus. Hat die neuere Version das Schema migriert, stellen
Sie zuerst deren Sicherung von vor der Migration wieder her -
`docker exec evey-server evey admin rollback` gibt die genauen Schritte aus,
mit dem Pfad der Sicherung.

**Prüfen, was läuft.** `docker exec evey-server evey version` gibt Version und
Commit des laufenden Binarys aus (`/healthz` meldet dasselbe), sodass „welcher
Code ist das?“ nie von einem Tag abhängt.

> Betreiber, die aus einem Checkout des Repositorys arbeiten, haben dasselbe
> Verfahren als Skript: `scripts/evey-update.sh` (standardmäßig Probelauf;
> Zustand, Sicherung vor dem Update, Pull, Warten auf Gesundheit,
> Commit-Prüfung, Rollback-Zeile) und `scripts/evey-rollback.sh`. Sie sind
> Hilfsmittel für die eigenen Bereitstellungen des Repositorys, nichts, was eine
> Installation braucht.

**Legen Sie fest, was läuft.** Setzen Sie einen Release-Tag (`0.2.0-beta.1`)
oder einen unveränderlichen `sha-<short>`-Tag in die `image:`-Zeile; ein
Branch-Tag wie `beta-0.0.1` wandert mit jedem Push, sodass zwei Pulls im Abstand
eines Tages unterschiedlichen Code laufen lassen können und „zurück auf
`beta-0.0.1`“ nirgendwohin führt. Das Skript gibt den gestarteten Tag aus, damit
Sie ihn festlegen können.

**Image-Tags** (`gravityz/evey`):

- Release-Tags `vX.Y.Z` veröffentlichen `X.Y.Z`, `X.Y`, `X` und `latest`. `latest`
  bedeutet immer das neueste semver-**Release**.
- Branch-Builds veröffentlichen den Branchnamen (z. B. `beta-0.0.1`) plus
  `sha-<short>`.
- **Laufzeit-Varianten** - dieselbe App, andere mitgelieferte
  Erweiterungs-Laufzeiten:
  - ohne Suffix (Standard / „lean“) - nur Python 3.
  - `-nodejs` - ergänzt Node für JS-Erweiterungen.
  - `-full` - ergänzt zusätzlich PHP + Ruby + Lua.
  Nehmen Sie `-nodejs` / `-full` nur, wenn Ihre Erweiterungen diese Laufzeiten
  brauchen; sonst ist der Standard am kleinsten. Setzen Sie z. B.
  `EVEY_VERSION=latest-nodejs`.
- **GPU-Varianten** - die veröffentlichten Tags bringen jede
  Beschleunigerfamilie mit (Intel Gen9 bis heute, AMD), allein etwa 900 MB an
  OpenCL-Compilern von Intel. Ein selbst gebautes Image kann nur die Familie
  seines Hosts behalten, mit `docker build --build-arg GPU=intel-legacy .`
  (Intel Gen9 bis Gen11, die UHD 6xx), `GPU=intel` (Gen12 und neuer), `GPU=amd`
  oder `GPU=none`. Das Image-Label `org.evey.gpu` hält die Wahl fest. NVIDIA
  braucht keine Variante: In kein Image ist eine NVIDIA-Bibliothek eingebaut,
  das NVIDIA Container Toolkit spritzt den Treiber des Hosts zur Laufzeit ein
  (`--gpus all` oder eine Gerätereservierung in Compose), sodass NVENC mit jeder
  Variante funktioniert, `none` eingeschlossen. Auf einer Maschine mit Intel-iGPU
  und NVIDIA-Karte wählt der `auto`-Modus der Kalibrierung zuerst VAAPI; legen
  Sie den Hardware-Modus im Transkodierungsbereich der Administration auf
  `nvenc` fest, um die Karte zu nutzen.

## 5. Sicherungen & Wiederherstellung

Schalten Sie geplante Datenbanksicherungen ein (`EVEY_BACKUP_INTERVAL_HOURS`,
abgelegt in `EVEY_BACKUP_DIR` → `/data/backups`, innerhalb von `evey-data`) oder
lösen Sie eine im Administrationsbereich aus. Sicherungen decken alle
Benutzerdaten ab, nicht Ihre Mediendateien. Einzelheiten:
[backups.md](backups.md).

Für ausgesperrte Konten, einen verlorenen Administrator oder eine
Wiederherstellung bringt das Image eine `evey admin`-CLI mit
(`reset-password`, `rotate-jwt-key`, `restore-backup`):
[runbooks/recovery.md](runbooks/recovery.md). Sicherheit bei Migrationen:
[runbooks/migration-runbook.md](runbooks/migration-runbook.md).

## 6. Fehlersuche

Erste Prüfungen, wenn etwas nicht stimmt:

```bash
docker compose ps                       # is every service Up / healthy?
docker compose logs --tail=200 evey     # server logs (migration errors, config validation)
```

- **evey startet nicht / Konfigurationsfehler.** Mit einer externen Datenbank
  verweigert der Server den Start, wenn `EVEY_DB_PASSWORD` noch der Standard
  `evey` ist; mit einem externen Meilisearch, wenn `MEILI_MASTER_KEY` leer oder
  ein Platzhalter ist (ein kurzer, vom Standard abweichender Schlüssel warnt nur).
  Lesen Sie die geloggte Prüfmeldung und korrigieren Sie `.env`.
- **Die eingebettete Datenbank startet nicht.** Die `evey:`-Zeilen am Anfang des
  Logs sagen warum: ein Cluster aus einer anderen Hauptversion (siehe den
  Abschnitt zum Upgrade oben), ein Datenverzeichnis, das kein Postgres-Cluster
  ist, oder ein Fehler mit dem Präfix `postgres:` von Postgres selbst (Platte
  voll, Rechte auf `./evey-data/postgres`, ein Dateisystem auf einer
  Netzwerkfreigabe).
- **Neustartschleife mit `permission denied` auf `/cache/...` im Log.** Das
  Volume `evey-cache` gehört einer anderen UID als der, unter der der Server läuft
  (`PUID` oder der übernommene Medienbesitzer). Images von vor dem 2026-09-17
  haben es nur neu vergeben, wenn auch `./evey-data` den Besitzer wechselte,
  sodass ein unter `PUID` vorab angelegter Datenordner (der NAS-Fall oben) den
  Cache zurückließ. Aktualisieren Sie das Image; bei einem älteren führen Sie
  einmal `docker run --rm -v evey_evey-cache:/c alpine chown -R <PUID>:<PGID> /c`
  aus (das Volume heißt `<project>_evey-cache`).
- **Externe Datenbank nicht erreichbar.** Prüfen Sie, ob Postgres gesund ist und
  das Passwort passt:
  ```bash
  docker exec evey-postgres pg_isready -U "$EVEY_DB_USER"
  ```
- **Suche kaputt / leer.** Prüfen Sie die Gesundheit von Meilisearch und lösen
  Sie dann eine Neuindizierung aus - in den Sucheinstellungen der Administration
  oder mit `docker exec evey-server evey admin reindex-search`. Der Index wird aus
  Postgres neu aufgebaut, `meili-data` zu löschen ist also unbedenklich.
- **Gesundheits-Endpunkt:** `curl -sf http://localhost:9091/healthz && echo OK`.

### Welche Volumes sich gefahrlos löschen lassen

| Volume | Inhalt | Gefahrlos löschbar? |
|--------|----------|-----------------|
| `evey-data` | Datenbank (`postgres/`), JWT-Schlüssel + erzeugte Geheimnisse (`keys/`), Logs, Sicherungen, Erweiterungen, Suchindex (`meili/`) | Nein - Daten, JWT-Schlüssel + Erweiterungen verloren. Nur `meili/`: ja, mit `evey admin reindex-search` neu aufgebaut |
| `postgres-data` | Datenbank (nur externes Layout) | Nein - alle Daten verloren |
| `evey-cache` | Cache für Transkodierung + Trickplay | Ja - bei Bedarf neu erzeugt |

## 7. Betreiber-Layout: externes Postgres und Meilisearch

**Wie sich die eingebettete Datenbank verhält.** Der Cluster liegt in
`./evey-data/postgres` (Postgres 17, `initdb` beim ersten Start, im Besitz des
Serverbenutzers). Er lauscht nur auf dem Loopback des Containers und verlangt
über TCP das erzeugte Passwort, sodass eine auf eine andere UID herabgestufte
Erweiterung ihn nicht öffnen kann. `docker stop` erreicht ihn als geordnetes
schnelles Herunterfahren, nachdem der Server seine Verbindungen geschlossen hat.
Das Datenverzeichnis muss auf einem echten Dateisystem liegen (ext4, btrfs,
xfs, ...) - nicht auf einer CIFS/NFS-Freigabe. Sicherungen (`evey admin`, der
Administrationsbereich, das geplante `pg_dump`) funktionieren genau wie mit einer
externen Datenbank. Um eine SQL-Shell zu öffnen:

```bash
docker exec -it evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

### Externes Postgres

Betreiber, die den Lebenszyklus der Datenbank selbst in der Hand haben wollen
(ein verwaltetes Postgres, eine festgelegte Hauptversion, ein eigenes Volume,
Dump+Restore nach eigenen Regeln), betreiben sie als benachbarten Dienst: Fügen
Sie den Dienst hinzu und richten Sie den Server darauf aus - **dass
`EVEY_DB_HOST` gesetzt ist, schaltet die eingebettete Datenbank ab**, weshalb
jede Installation von vor der eingebetteten Datenbank unverändert weiterläuft:

```yaml
services:
  evey:
    # ... as above, plus:
    environment:
      EVEY_DB_HOST: postgres
      EVEY_DB_SSLMODE: disable          # internal Docker network; the bundled Postgres has no TLS
    depends_on:
      postgres: { condition: service_healthy }

  postgres:
    image: postgres:16-alpine
    container_name: evey-postgres
    environment:
      POSTGRES_DB: ${EVEY_DB_NAME:-evey}
      POSTGRES_USER: ${EVEY_DB_USER:-evey}
      POSTGRES_PASSWORD: ${EVEY_DB_PASSWORD:?EVEY_DB_PASSWORD must be set in .env}
    volumes:
      - ./postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${EVEY_DB_USER:-evey}"]
      interval: 5s
      timeout: 3s
      retries: 5
    restart: unless-stopped
```

```env
EVEY_DB_PASSWORD=<strong-password>   # must NOT be the default 'evey' - the server refuses to start otherwise
```

**Externes Meilisearch, dieselbe Idee.** Ein eigener Suchcontainer behält seinen
eigenen Lebenszyklus und sein Volume (und lässt Sie mit
`docker compose restart meilisearch` nur den Index zurücksetzen). Schalten Sie
den eingebetteten ab, richten Sie den Server darauf aus und teilen Sie den
Schlüssel - ein ausdrückliches `MEILI_MASTER_KEY` gewinnt immer gegen den
erzeugten `./evey-data/keys/meili.key`:

```yaml
services:
  evey:
    # ... plus:
    environment:
      EVEY_EMBEDDED_MEILI: "false"
      EVEY_MEILI_URL: http://meilisearch:7700
    depends_on:
      meilisearch: { condition: service_healthy }

  meilisearch:
    image: getmeili/meilisearch:v1.12
    container_name: evey-meilisearch
    environment:
      MEILI_MASTER_KEY: ${MEILI_MASTER_KEY:?MEILI_MASTER_KEY must be set in .env}
      MEILI_NO_ANALYTICS: "true"
    volumes:
      - ./meili-data:/meili_data
    healthcheck:
      test: ["CMD", "curl", "-sf", "http://localhost:7700/health"]
      interval: 5s
      retries: 5
    restart: unless-stopped
```

```env
MEILI_MASTER_KEY=<run: openssl rand -hex 16>
```

Geben Sie diesem Container möglichst einen `user:` ohne root und ein
`mem_limit` - das offizielle Image läuft als root, und sein residenter Speicher
(speichergemappter Index) sieht groß aus, ist aber größtenteils freigebbarer
Page Cache. Der Index wird aus Postgres neu aufgebaut
(`evey admin reindex-search`), `meili-data` ist also so oder so entbehrlich.

Nur diese Umgebungsvariablen sind Infrastruktur/Geheimnis und **gehören in
Compose/.env** - sie sind nie im Administrationsbereich änderbar:
`EVEY_DB_PASSWORD`, `MEILI_MASTER_KEY`, `EVEY_DB_HOST`, `EVEY_DB_USER`,
`EVEY_DB_NAME`, `EVEY_DB_SSLMODE`, `EVEY_EMBEDDED_POSTGRES`,
`EVEY_EMBEDDED_MEILI`, `EVEY_MEILI_URL`, die `*_DIR`-Pfade,
`EVEY_JWT_PRIVATE_KEY_PATH`, `EVEY_SERVER_BASE_URL`, `EVEY_TRUSTED_PROXIES` und
der Hauptschalter der Erweiterungen `EVEY_PLUGINS_ENABLED`.
(`EVEY_PLUGINS_SANDBOX_MODE`, `EVEY_PLUGINS_UID_SEPARATION` und die
Ressourcengrenzen dürfen hier als Anfangswerte stehen, sind aber zur Laufzeit
unter Admin → Plugins änderbar.) Vollständige Referenz:
[configuration.md](configuration.md).

### Eine bestehende Installation auf die eingebettete Datenbank umziehen

Eine Installation, die Postgres als benachbarten Dienst betreibt (das Layout
aller Versionen vor der eingebetteten Datenbank), kann auf den einzelnen
Container umziehen: Die Daten sind nur einen Dump entfernt, alles andere in
`./evey-data` (JWT-Schlüssel, Erweiterungen, Sicherungen, Trickplay) wird
unverändert übernommen. Die eingebettete Datenbank importiert einen Dump, der
unter `./evey-data/postgres-import.sql.gz` liegt, **einmal, in den Cluster, den
sie bei ihrem ersten Start anlegt**, bevor der Server läuft - die eigenen
Migrationen des Servers werden danach darauf angewandt, wie nach jedem Update.

```bash
# 1. dump the current database (plain SQL, gzipped - what the import expects)
docker compose exec -T postgres pg_dump -U evey --no-owner --no-privileges evey \
  | gzip > evey-data/postgres-import.sql.gz
docker compose down

# 2. write the new compose (keep a copy of the old one): the single-service
#    file from section 1, with your variations (extra media folders, the
#    proxy block) and whatever you had in `environment:` (hardware
#    transcoding: `devices: /dev/dri`, `group_add`, LIBVA_DRIVER_NAME).

# 3. first boot: initdb, import, migrations, then the server
docker compose up -d && docker compose logs -f evey     # "evey: import done"

# 4. search index: an external Meilisearch's index is not carried over, rebuild it
docker exec evey-server evey admin reindex-search
```

Was NICHT mehr nötig ist: `EVEY_DB_PASSWORD`, `MEILI_MASTER_KEY` und die
Verdrahtung über `EVEY_DB_HOST` / `EVEY_MEILI_URL` (die Geheimnisse werden
erzeugt), die Dienste `postgres` und `meilisearch` und aus `.env` vorbelegte
API-Schlüssel (sie liegen schon in der gesicherten `server_config`). Behalten
Sie `./postgres-data` und `./meili-data`, bis Sie den Server geprüft haben; die
alte Compose-Datei betreibt sie noch, falls Sie zurückmüssen. Ein Dump neben
einem bereits eingerichteten Cluster wird gemeldet und ignoriert, nie
zusammengeführt; ein gescheiterter Import verwirft den neuen Cluster, sodass der
nächste Start es erneut aus dem Dump versucht.

### Upgrade der Postgres-Hauptversion (eingebettete Datenbank)

Das Image legt die Hauptversion des eingebetteten Postgres fest (heute 17). Ein
neueres Image mit einer neueren Hauptversion **weigert sich, den alten Cluster
zu öffnen**, mit einer Meldung, die diesen Abschnitt nennt - es betreibt nie
stillschweigend ein Datenverzeichnis, das es nicht lesen kann. Der Weg hinüber
ist ein Dump mit dem alten Image und eine Wiederherstellung mit dem neuen:

```bash
docker compose exec evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_dump -h 127.0.0.1 -U evey -Fc evey' > evey-before-upgrade.dump
docker compose down
mv evey-data/postgres evey-data/postgres.old      # keep it until the restore is verified
# set the new EVEY_VERSION in .env, then:
docker compose up -d                              # initialises an empty cluster on the new major
docker compose exec -T evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_restore -h 127.0.0.1 -U evey -d evey --no-owner' < evey-before-upgrade.dump
docker compose restart evey
```

Betreiber, die `pg_upgrade` oder eine über Jahre festgelegte Hauptversion
bevorzugen, nutzen das
[externe Postgres-Layout](#7-operator-layout-external-postgres-and-meilisearch).

## 8. ARM-Hosts (x86-64 und ARM64)

Stabile Versionen (Tags `vX.Y.Z`) veröffentlichen ein **Multi-Arch**-Image für
`linux/amd64` und `linux/arm64`; `docker pull`/`compose up` wählt automatisch
das richtige, ohne Flag. Tags für Vorabversionen (`-alpha`/`-beta`) und Builds
pro Branch sind **nur amd64** - legen Sie für einen ARM-Host (ein Raspberry Pi,
ein ARM-NAS, ein Ampere-VPS) ein stabiles `vX.Y.Z` in `EVEY_VERSION` fest.

Zwei Dinge sollten Sie auf ARM wissen:

- **Hardware-Transkodierung auf ARM: nur Rockchip, und nur, was die
  Kalibrierung beweist.** Evey verdrahtet vier Beschleuniger-Backends - Intel
  QSV, VAAPI, NVENC und Rockchip **RKMPP** (SoCs der Klasse RK3588 / RK3576 /
  RK3568, über die Encoder `h264_rkmpp` / `hevc_rkmpp` des arm64-jellyfin-ffmpeg,
  Dekodierung über `drm_prime`, Skalierung über RGA und ein OpenCL-Tone-Mapping
  auf dem Mali-ICD, wenn vorhanden). QSV und die VAAPI-Treiber von Intel gibt es
  nur für x86 (im arm64-Image bewusst nicht installiert); NVENC ist NVIDIA. Um
  RKMPP zu nutzen, reichen Sie den MPP-Dienstknoten und den DRM-Render-Knoten in
  den Container durch (`/dev/mpp_service`, `/dev/dri` und `/dev/rga`, wo der
  Kernel ihn bereitstellt); Evey erkennt ihn, führt eine echte Probe-Kodierung
  aus und vermisst und beweist dann jede Form auf Ihrer Platine, bevor es sie
  anbietet - nichts wird aus dem Namen des SoC abgeleitet, und eine Form, die
  sich nicht beweist, bleibt auf der CPU. RKMPP wurde vom Projekt noch nicht auf
  echter Hardware geprüft: Behandeln Sie es als nach bestem Bemühen und sehen Sie
  unter **Admin → Transkodierung → Encode-Kalibrierung** nach, was zertifiziert
  wurde. Der **V4L2-M2M**-Encoder des Raspberry Pi ist **nicht** verdrahtet: Auf
  einem Pi oder einem ARM-NAS der Synology-Klasse (Realtek) gibt es keinen Weg
  zur Hardware-Kodierung, also bedeutet ARM **Direktwiedergabe** (der Client
  spielt die Datei unverändert, zu fast null Kosten) plus
  **Software-Transkodierung** auf der CPU. Die Kapazität ist kalibriert, nicht
  geraten: Ein schwacher Kern behält den 720p-Software-Boden und lässt eine
  Kodierung nach der anderen zu; Sie können `transcoding.max_video_sessions`
  (Admin → Transkodierung) trotzdem senken und auf Direktwiedergabe setzen.
- **HDR-Tone-Mapping läuft auch auf ARM auf dem schnellen Weg.** Das
  mitgelieferte arm64-jellyfin-ffmpeg bringt den SIMD-Filter `tonemapx` (NEON)
  mit, so wie der amd64-Build seine AVX2-Variante - HDR→SDR-Tone-Mapping ist also
  Hardware-SIMD, nicht die langsame zscale-Kette. (Evey prüft `tonemapx` zur
  Laufzeit und fällt nur auf zscale zurück, falls ein Build ihn je nicht hat.)

Bauen Sie das Image selbst für eine nicht veröffentlichte Architektur,
funktioniert `docker buildx build --platform linux/arm64` sofort - das
Dockerfile schirmt die x86-spezifischen Teile hinter `TARGETARCH` ab.

## 9. Sandbox der Erweiterungen

Erweiterungen sind Code von Dritten, der in eigenen Prozessen läuft. Das
offizielle Image enthält alles, was nötig ist, um sie einzusperren - kein
`--privileged`, kein zusätzlicher Docker-Sicherheitsschalter - und
`EVEY_PLUGINS_SANDBOX_MODE=auto` (der Standard) wählt das stärkste Backend, das
der Host erlaubt, auf jedem aktuellen Kernel Landlock. Der Server selbst läuft
nie als root. Die Einzelheiten Schicht für Schicht (seccomp, UID-Trennung,
Ressourcengrenzen) stehen in [SECURITY.md](SECURITY.md); diese Seite sagt Ihnen
nur, was Sie prüfen sollten.

Prüfen Sie nach dem ersten Start die tatsächlich geltende Sandbox unter
**Admin → Plugins → Security Dashboard** (Karte der Host-Sandbox): `auto` sollte
auf einem aktuellen Kernel `landlock` zeigen; `basic` bedeutet, dass der Host den
Dateisystemzugriff der Erweiterungen nicht isolieren kann - siehe die Anleitung
zur Bereitstellung unten.

Welches Setup für Ihren Rechner (altes NAS, ohne Container, was ein- oder auszuschalten ist, die Risiken in einfachen Worten): [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md). Vollständiges Bedrohungsmodell und Einzelheiten je Schicht: [plugins/security-model.md](plugins/security-model.md).
