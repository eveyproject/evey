---
source_sha: 6c5be1c7b73eceb506748f84e685e858d94021c34fe6715f0d4127dfd10d261f
reviewed: false
---
# Erweiterte Bereitstellungen

Die [Installation in einem Container](../installation.md) passt zu den meisten
Servern. Diese Seite behandelt die Aufbauten, die ein Betreiber bevorzugen kann:
Datenbank oder Suchmaschine als eigene Dienste, ARM-Hosts, die Image-Varianten
und die Plugin-Sandbox auf ungewöhnlichen Hosts. Um Evey ganz ohne Docker zu
betreiben, siehe [Ohne Container](../deployment-bare-metal.md).

## Die eingebettete Datenbank

Der Datenbankcluster liegt in `./evey-data/postgres` (Postgres 17, beim ersten
Start angelegt, im Besitz des Serverbenutzers). Er lauscht nur auf dem Loopback
des Containers und verlangt das erzeugte Passwort, sodass ein Plugin, das unter
einem anderen Benutzer läuft, ihn nicht öffnen kann. `docker stop` fährt ihn
sauber herunter, nachdem der Server seine Verbindungen geschlossen hat.

Das Datenverzeichnis muss auf einem lokalen Dateisystem liegen (ext4, btrfs,
xfs...), nicht auf einer SMB/CIFS- oder NFS-Freigabe.

Um eine SQL-Shell zu öffnen:

```bash
docker exec -it evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

### Postgres-Hauptversions-Upgrade

Das Image legt die Hauptversion des eingebetteten Postgres fest (17). Ein Image
mit einer neueren Hauptversion verweigert das Öffnen des alten Clusters, mit
einer Meldung, die hierher verweist, statt ein Datenverzeichnis zu betreiben,
das es nicht lesen kann. Der Weg hinüber ist ein Dump mit dem alten Image und
eine Wiederherstellung mit dem neuen:

```bash
docker compose exec evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_dump -h 127.0.0.1 -U evey -Fc evey' > evey-before-upgrade.dump
docker compose down
mv evey-data/postgres evey-data/postgres.old      # behalten, bis die Wiederherstellung geprüft ist
# das neue Image-Tag in die Compose-Datei eintragen, dann:
docker compose up -d                              # legt einen leeren Cluster mit der neuen Hauptversion an
docker compose exec -T evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_restore -h 127.0.0.1 -U evey -d evey --no-owner' < evey-before-upgrade.dump
docker compose restart evey
```

## Externes Postgres

Um den Lebenszyklus der Datenbank selbst zu verwalten (ein verwaltetes
Postgres, eine festgelegte Hauptversion, Ihre eigene Dump-Strategie), betreiben
Sie sie als eigenen Dienst und richten den Server darauf aus. **Das Setzen von
`EVEY_DB_HOST` schaltet die eingebettete Datenbank ab.**

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... wie in der Datei mit einem Container, dazu:
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
    EVEY_DB_PASSWORD=<strong-password>   # der Server startet nicht mit dem Standardwert 'evey'
    ```

## Externes Meilisearch

Dasselbe Prinzip für die Suchmaschine: die eingebettete abschalten, den Server
auf Ihre ausrichten und den Schlüssel teilen. Ein ausdrücklicher
`MEILI_MASTER_KEY` hat Vorrang vor dem erzeugten `./evey-data/keys/meili.key`.

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

Der Index wird aus der Datenbank neu aufgebaut (`evey admin reindex-search`),
`meili-data` kann also jederzeit verworfen werden.

## Auf die eingebettete Datenbank umziehen

Eine Installation, die Postgres als eigenen Dienst betreibt, kann auf den
einzelnen Container umziehen. Die eingebettete Datenbank importiert einmal einen
Dump, den sie unter `./evey-data/postgres-import.sql.gz` findet, in den Cluster,
den sie beim ersten Start anlegt, bevor der Server läuft; die Migrationen des
Servers werden danach darauf angewendet.

```bash
# 1. die aktuelle Datenbank sichern (reines SQL, gzip-komprimiert)
docker compose exec -T postgres pg_dump -U evey --no-owner --no-privileges evey \
  | gzip > evey-data/postgres-import.sql.gz
docker compose down

# 2. die Compose-Datei durch die mit einem Container ersetzen, dabei Ihre
#    Medienordner, Reverse-Proxy-Einstellungen und Hardware-Zeilen behalten.

# 3. erster Start: Cluster anlegen, importieren, migrieren, Server starten
docker compose up -d && docker compose logs -f evey     # "evey: import done"

# 4. ein externer Meilisearch-Index wird nicht übernommen: neu aufbauen
docker exec evey-server evey admin reindex-search
```

`EVEY_DB_PASSWORD`, `MEILI_MASTER_KEY` und die Einstellungen `EVEY_DB_HOST` /
`EVEY_MEILI_URL` werden nicht mehr gebraucht. Behalten Sie `./postgres-data`,
bis Sie den Server geprüft haben. Ein Dump neben einem bestehenden Cluster wird
gemeldet und ignoriert, nie zusammengeführt; ein fehlgeschlagener Import
verwirft den neuen Cluster, damit der nächste Start es erneut versucht.

## ARM-Hosts

Images aus einem Release-Tag (`0.2.0`, Vorabversionen wie `0.2.0-beta.1`
eingeschlossen) sind Multi-Architektur-Images (`linux/amd64` und
`linux/arm64`); Docker wählt das passende. Branch-Tags wie `beta-0.0.1` gibt es
nur für amd64: Legen Sie auf einem ARM-Host ein Release-Tag fest.

- **Hardware-Transkodierung auf ARM** ist auf Rockchip-SoCs (Klasse RK3588,
  RK3576, RK3568) über RKMPP beschränkt. Reichen Sie `/dev/mpp_service`,
  `/dev/dri` und, wo der Kernel es bereitstellt, `/dev/rga` durch; Evey prüft
  und vermisst den Encoder, bevor er ihn nutzt. RKMPP wurde vom Projekt noch
  nicht auf echter Hardware validiert. Ein Raspberry Pi oder ein ARM-NAS mit
  Realtek-Chip hat keinen Hardware-Encoder, den Evey nutzen kann: Diese Server
  setzen auf Direktwiedergabe und Software-Transkodierung, bemessen durch
  dieselbe Kalibrierung wie jeder andere Host.
- **HDR-Tone-Mapping** nutzt auf ARM (NEON) denselben SIMD-Filter `tonemapx` wie
  auf x86-64 (AVX2).

## Image-Tags

Das Image ist `gravityz/evey`.

- Release-Tags `vX.Y.Z` veröffentlichen `X.Y.Z`, `X.Y`, `X` und `latest`.
  `latest` ist immer die neueste stabile Version: Eine Vorabversion
  (`vX.Y.Z-beta.N`) veröffentlicht nur ihr eigenes Tag und verschiebt es nie.
- Branch-Builds veröffentlichen den Branch-Namen (zum Beispiel `beta-0.0.1`) und
  `sha-<commit>`.
- **Plugin-Laufzeiten.** Das Standard-Image bringt Python 3 für Plugins mit. Die
  Variante `-nodejs` fügt Node.js hinzu; `-full` fügt PHP, Ruby und Lua hinzu.
  Nutzen Sie eine Variante nur, wenn ein installiertes Plugin sie braucht.
- **GPU-Familien.** Veröffentlichte Images enthalten die Treiber für jede
  unterstützte Intel- und AMD-Generation. Ein selbst gebautes Image kann mit
  `docker build --build-arg GPU=intel-legacy .` (Intel Gen9 bis Gen11),
  `GPU=intel` (Gen12 und neuer), `GPU=amd` oder `GPU=none` nur eine behalten.
  NVIDIA braucht keine Variante: Das NVIDIA Container Toolkit bindet den
  Treiber des Hosts zur Laufzeit ein.

## Plugin-Sandbox

Das Image enthält alles, um Plugins einzusperren, ohne privilegierten Modus.
Der Standardmodus der Sandbox (`auto`) wählt das stärkste Backend, das der Host
erlaubt: Landlock auf jedem modernen Kernel. Prüfen Sie den tatsächlich
geltenden Modus im **Sicherheits-Dashboard** unter **Admin → Plugins**. Ein
Host, der `basic` anzeigt, kann den Dateizugriff von Plugins nicht isolieren:
siehe [Plugin-Sandbox einrichten](../plugins/sandbox-deployment.md) für die
Bedeutung und was zu ändern ist. Die Schichten selbst beschreibt
[Sicherheitsmodell und Plugin-Sandbox](../SECURITY.md).
