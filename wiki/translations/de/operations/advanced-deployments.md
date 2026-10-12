---
source_sha: be2352babc32903dbb0a839c8c6754b1f9c0751d295a958a65e2ab12031c037e
reviewed: true
---
# Erweiterte Bereitstellungen

Die [Installation in einem einzigen Container](../installation.md) passt zu
den meisten Servern. Öffnen Sie diese Seite, wenn Sie die Datenbank oder die
Suchmaschine als eigene Dienste betreiben wollen, wenn der Host ein ARM-Gerät
ist, wenn Sie eine Image-Variante brauchen oder wenn die Plugin-Sandbox etwas
anderes als `landlock` meldet. Um Evey ganz ohne Docker zu betreiben, siehe
[Ohne Container](../deployment-bare-metal.md).

## Die eingebettete Datenbank

Der Cluster liegt in `./evey-data/postgres` (Postgres 17, beim ersten Start
angelegt, im Besitz des Serverbenutzers). Er lauscht nur auf dem Loopback des
Containers und verlangt das erzeugte Passwort. Ein Plugin, das unter einem
anderen Benutzer läuft, kann ihn also nicht öffnen. `docker stop` fährt ihn
sauber herunter, nachdem der Server seine Verbindungen geschlossen hat.

Das Datenverzeichnis muss auf einem lokalen Dateisystem liegen (ext4, btrfs,
xfs...), nicht auf einer SMB/CIFS- oder NFS-Freigabe. So öffnen Sie eine
SQL-Shell:

```bash
docker exec -it evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

### Auf eine neuere Postgres-Hauptversion wechseln

Das Image legt die Hauptversion der eingebetteten Datenbank fest. Ein Image
mit einer neueren Hauptversion weigert sich, den alten Cluster zu öffnen, und
zeigt eine Meldung, die auf diese Seite verweist. Es betreibt also kein
Datenverzeichnis, das es nicht lesen kann. Der Weg hinüber ist ein Dump mit
dem alten Image und eine Wiederherstellung mit dem neuen:

```bash
docker compose exec evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_dump -h 127.0.0.1 -U evey -Fc evey' > evey-before-upgrade.dump
docker compose down
mv evey-data/postgres evey-data/postgres.old      # behalten, bis die Wiederherstellung geprüft ist
# das neue Image-Tag in die Compose-Datei eintragen, dann:
docker compose up -d                              # legt einen leeren Cluster mit der neuen Hauptversion an
docker compose exec -T evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_restore -h 127.0.0.1 -U evey -d evey --no-owner' < evey-before-upgrade.dump
docker compose restart evey
```

## Postgres als eigenen Dienst betreiben

Wenn Sie den Lebenszyklus der Datenbank selbst steuern wollen (ein verwaltetes
Postgres, eine fest gewählte Hauptversion, Ihre eigene Dump-Strategie),
betreiben Sie sie als eigenen Dienst und richten den Server darauf. Erst das
Setzen von `EVEY_DB_HOST` schaltet die eingebettete Datenbank ab. Mit dem
Standardpasswort `evey` verweigert der Server den Start.

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... wie in der Datei für den einzelnen Container, dazu:
        environment:
          EVEY_DB_HOST: postgres
          EVEY_DB_SSLMODE: disable          # internes Netzwerk, kein TLS
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

=== ".env"

    ```env
    EVEY_DB_PASSWORD=<strong-password>
    ```

## Meilisearch als eigenen Dienst betreiben

Dasselbe Prinzip für die Suchmaschine: Schalten Sie die eingebettete ab,
richten Sie den Server auf Ihre eigene und verwenden Sie auf beiden Seiten
denselben Schlüssel. Ein ausdrücklich gesetzter `MEILI_MASTER_KEY` gewinnt
gegenüber dem erzeugten `./evey-data/keys/meili.key`.

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... dazu:
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

=== ".env"

    ```env
    MEILI_MASTER_KEY=<run: openssl rand -hex 16>
    ```

Der Index wird mit `docker exec evey-server evey admin reindex-search` aus der
Datenbank neu aufgebaut. `meili-data` kann also jederzeit verworfen werden.

## Zurück zur eingebetteten Datenbank

Eine Installation, die Postgres als eigenen Dienst betreibt, kann auf den
einzelnen Container umziehen. Die eingebettete Datenbank importiert einmalig
einen Dump, den sie unter `./evey-data/postgres-import.sql.gz` findet, und
zwar in den Cluster, den sie beim ersten Start anlegt, bevor der Server
läuft. Danach laufen die Migrationen des Servers darüber.

```bash
# 1. die aktuelle Datenbank exportieren (reines SQL, mit gzip gepackt)
docker compose exec -T postgres pg_dump -U evey --no-owner --no-privileges evey \
  | gzip > evey-data/postgres-import.sql.gz
docker compose down

# 2. die Compose-Datei durch die für den einzelnen Container ersetzen und dabei
#    Ihre Medienordner, Reverse-Proxy-Einstellungen und Hardware-Zeilen behalten.

# 3. erster Start: Cluster anlegen, importieren, migrieren, Server starten
docker compose up -d && docker compose logs -f evey     # "evey: import done"

# 4. ein externer Meilisearch-Index wird nicht übernommen: neu aufbauen
docker exec evey-server evey admin reindex-search
```

`EVEY_DB_PASSWORD`, `MEILI_MASTER_KEY` und die Einstellungen `EVEY_DB_HOST` /
`EVEY_MEILI_URL` werden nicht mehr gebraucht. Behalten Sie `./postgres-data`,
bis Sie den Server geprüft haben. Ein Dump, der neben einem bestehenden
Cluster liegt, wird gemeldet und ignoriert, nie zusammengeführt. Ein
fehlgeschlagener Import verwirft den neuen Cluster, damit der nächste Start
es erneut mit dem Dump versucht.

## ARM-Hosts

Images, die aus einem Release-Tag gebaut werden (`0.2.0`, auch Vorabversionen
wie `0.2.0-beta.1`), gibt es für mehrere Architekturen (`linux/amd64` und
`linux/arm64`); Docker wählt die passende. Branch-Tags wie `beta-0.0.1` gibt es
nur für amd64: Legen Sie auf einem ARM-Host ein Release-Tag fest.

- **Hardware-Transkodierung auf ARM** ist auf Rockchip-SoCs (Klasse RK3588,
  RK3576, RK3568) über RKMPP beschränkt. Geben Sie `/dev/mpp_service`,
  `/dev/dri` und, wo der Kernel es bereitstellt, `/dev/rga` an den Container
  weiter. Der Server prüft den Encoder und misst ihn, bevor er ihn nutzt.
  RKMPP wurde vom Projekt noch nicht auf echter Hardware geprüft. Ein
  Raspberry Pi oder ein ARM-NAS mit Realtek-Chip hat keinen Hardware-Encoder,
  den der Server nutzen kann: Diese Hosts setzen auf Direktwiedergabe und
  Software-Transkodierung, bemessen durch dieselbe Kalibrierung wie jeder
  andere Host.
- **HDR-Tone-Mapping** nutzt auf ARM (NEON) denselben SIMD-Filter `tonemapx`
  wie auf x86-64 (AVX2).

## Image-Tags und Varianten

Das Image heißt `eveyproject/evey`.

| Tag | Veröffentlicht durch | Hinweise |
|---|---|---|
| `X.Y.Z`, `X.Y`, `X`, `latest` | ein Release-Tag `vX.Y.Z` | `latest` ist immer die neueste stabile Version |
| `X.Y.Z-beta.N` | ein Vorabversions-Tag | veröffentlicht nur sein eigenes Tag, verschiebt nie `latest` |
| `beta-0.0.1`, `sha-<commit>` | ein Branch-Build | nur amd64 |

Jedes Tag gibt es in drei Varianten: Die Standardvariante bringt Python 3 für
Plugins mit, `-nodejs` fügt Node.js hinzu, `-full` fügt PHP, Ruby und Lua
hinzu (zum Beispiel `latest-full`). Nehmen Sie eine Variante nur, wenn ein
Plugin sie braucht: Das Installationsprogramm lehnt ein Plugin ab, dessen
angegebene Sprache im Image fehlt, und nennt in der Meldung die richtige
Variante.

Die veröffentlichten Images enthalten die Treiber für jede unterstützte
Intel- und AMD-Generation. Ein selbst gebautes Image kann nur eine Familie
behalten, mit `docker build --build-arg GPU=intel-legacy .` (Intel Gen9 bis
Gen11), `GPU=intel` (Gen12 und neuer), `GPU=amd` oder `GPU=none`. NVIDIA
braucht keine Variante: Das NVIDIA Container Toolkit bringt den Treiber des
Hosts zur Laufzeit in den Container.

## Die Plugin-Sandbox auf einem ungewöhnlichen Host

Das Image enthält alles, was nötig ist, um Plugins abzuschotten, ohne
privilegierten Modus. Der Standard-Sandbox-Modus (`auto`) wählt das stärkste
Verfahren, das der Host erlaubt: Landlock auf jedem modernen Kernel. Den
geltenden Modus sehen Sie im **Sicherheits-Dashboard** unter **Verwaltung →
Plugins**. Ein Host, der `basic` anzeigt, kann den Dateizugriff von Plugins
nicht isolieren: [Plugin-Sandbox einrichten](../plugins/sandbox-deployment.md)
erklärt, was das bedeutet und was zu ändern ist. Die Schichten selbst sind
unter [Sicherheitsmodell und Plugin-Sandbox](../SECURITY.md) beschrieben.

<!-- sources
embedded db, import, major upgrade, embedded meili: docker/entrypoint.sh (PG_MAJOR, PG_IMPORT, "evey: import done", EVEY_EMBEDDED_MEILI, MEILI_MASTER_KEY_FILE)
external db switch and default password refusal: internal/config/config.go (EVEY_DB_HOST, hardFail on "evey")
reindex: cmd/evey/admin.go - adminReindexSearch
arm, variants, GPU families: Dockerfile (TARGETARCH, RUNTIMES, GPU) ; .github/workflows/ci.yml (tags, flavor suffix, platforms) ; internal/adapters/driven/ffmpeg/hwdetect.go - detectRKMPP
sandbox auto: internal/adapters/driven/pluginrunner/runner_landlock.go - resolveAutoMode ; i18n/en/admin.yaml - plugins.security.title
-->
