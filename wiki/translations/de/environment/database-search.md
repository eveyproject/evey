---
source_sha: 6efce4118fc9a8bfa56a70e2648cc5f619c415c257832c5a9ea6da0e9e98e645
reviewed: true
---
# Datenbank und Suche

Die Variablen für die Postgres-Datenbank und den Meilisearch-Index.
Standardmäßig betreibt das Image beides selbst und erzeugt die Passwörter:
Eine normale Installation setzt keine dieser Variablen. Sie dienen einer
Installation, die ihre eigene Datenbank oder Suchmaschine mitbringt oder die
eingebetteten feinjustiert. Die Vorgehensweisen stehen unter
[Erweiterte Bereitstellungen](../operations/advanced-deployments.md). Keine
von ihnen hat ein Gegenstück im Verwaltungsbereich.

## Datenbank

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_EMBEDDED_POSTGRES` | nicht gesetzt | Nicht gesetzt: eingebettet, wenn `EVEY_DB_HOST` nicht gesetzt ist, sonst extern. `true` oder `false`, um es ausdrücklich festzulegen. Wird vom Startskript gelesen. | keine |
| `EVEY_POSTGRES_DATA_DIR` | `/data/postgres` | Wo der eingebettete Cluster liegt. Er muss auf einem lokalen Dateisystem liegen, nicht auf einer Netzwerkfreigabe. Wird vom Startskript gelesen. | keine |
| `EVEY_POSTGRES_IMPORT` | `/data/postgres-import.sql.gz` | Ein mit gzip gepackter `pg_dump`, der in den eingebetteten Cluster eingespielt wird, wenn dieser zum ersten Mal entsteht. Danach wird die Datei in `*.imported-<date>` umbenannt. Wird mit einer Meldung ignoriert, wenn der Cluster schon existiert. Wird vom Startskript gelesen. | keine |
| `EVEY_DB_HOST` | `localhost` | Der Datenbank-Host. **Wird er gesetzt, ist die eingebettete Datenbank aus.** | keine |
| `EVEY_DB_PORT` | `5432` | Der Datenbank-Port. | keine |
| `EVEY_DB_USER` | `evey` | Der Datenbank-Login. Ist er ein Superuser (der eingebettete Cluster und die mitgelieferte Compose-Datei), behält der Server ihn nur für die Wiederherstellung und läuft als `EVEY_DB_APP_USER`. | keine |
| `EVEY_DB_APP_USER` | `evey_app` | Die Rolle ohne Sonderrechte, unter der der Server und seine Migrationen laufen, wenn `EVEY_DB_USER` ein Superuser ist. Wird beim Start angelegt, bekommt die Datenbank und ihre Objekte übertragen und bei jedem Start ein neues Zufallspasswort (das nirgends gespeichert wird). Eine SQL-Injection kann dann keine Programme auf dem Host starten und keine Dateien dort lesen. | keine |
| `EVEY_DB_PRIVILEGE_SPLIT` | `true` | `false` lässt alles als `EVEY_DB_USER` laufen, auch als Superuser. Eine Ausweichlösung, falls eine Datenbank die Trennung ablehnt; sonst bricht der Start ab und nennt diese Variable. | keine |
| `EVEY_DB_NAME` | `evey` | Der Name der Datenbank. | keine |
| `EVEY_DB_PASSWORD` | erzeugt | Das Passwort. Bei einer externen Datenbank verweigert der Server den Start mit dem Wert `evey` und warnt bei `changeme`. | keine |
| `EVEY_DB_PASSWORD_FILE` | `/data/keys/postgres.password` | Eine Datei mit dem Passwort, gelesen, wenn `EVEY_DB_PASSWORD` nicht gesetzt ist. Vom Image gesetzt. | keine |
| `EVEY_DB_SSLMODE` | `disable` bei Loopback, sonst `prefer` | `disable`, `prefer`, `require` oder `verify-full`. `prefer` gegenüber einem entfernten Host erzeugt eine Warnung im Protokoll: Verwenden Sie `require` oder `verify-full`. | keine |
| `EVEY_DB_MAX_OPEN_CONNS` | `25` | Offene Verbindungen zur Datenbank. Jede kostet auf der Postgres-Seite Speicher; erhöhen Sie den Wert nur für eine große Installation. | keine |
| `EVEY_DB_MAX_IDLE_CONNS` | `10` | Verbindungen, die im Pool ungenutzt offen bleiben. Höchstens so viele wie die offenen Verbindungen. | keine |

## Suche

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_EMBEDDED_MEILI` | `true` | `false` verwendet statt der eingebetteten Instanz das Meilisearch unter `EVEY_MEILI_URL`. Wird vom Startskript gelesen. | keine |
| `EVEY_MEILI_URL` | `http://localhost:7700` | Die Adresse von Meilisearch. | keine |
| `MEILI_MASTER_KEY` | erzeugt | Der Meilisearch-Schlüssel. Bei einer externen Instanz verhindert ein leerer Schlüssel oder ein Platzhalter den Start; ein Schlüssel unter 16 Zeichen erzeugt eine Warnung im Protokoll. | keine |
| `MEILI_MASTER_KEY_FILE` | `/data/keys/meili.key` | Eine Datei mit dem Schlüssel, gelesen, wenn `MEILI_MASTER_KEY` nicht gesetzt ist. Vom Image gesetzt. | keine |
| `MEILI_DATA_DIR` | `/data/meili` | Wo der eingebettete Index liegt. Wird vom Startskript gelesen. | keine |
| `MEILI_MAX_INDEXING_MEMORY` | etwa 20 % des Speichers des Containers, mindestens 128 MiB | Der Speicher, den das eingebettete Meilisearch zum Indexieren nutzen darf. Wird aus der Grenze des Containers berechnet, nicht aus dem Speicher der Maschine, damit ein Scan einen kleinen Container nicht erschöpft. Ein ausdrücklich gesetzter Wert wird unverändert weitergegeben. | keine |

<!-- sources
internal/adapters/driven/postgres/approle.go - EnsureAppRole, HandOverToAppRole (approle_test.go)
cmd/evey/database.go - appLogin
internal/config/config.go - EVEY_DB_*, defaultSSLMode, envSecret (_FILE), EVEY_MEILI_URL, MEILI_MASTER_KEY ; WarnValidate (password 'evey' / 'changeme', sslmode prefer, idle > open, key length)
docker/entrypoint.sh - EVEY_EMBEDDED_POSTGRES, EVEY_EMBEDDED_MEILI, EVEY_POSTGRES_DATA_DIR, EVEY_POSTGRES_IMPORT, MEILI_DATA_DIR, MEILI_MAX_INDEXING_MEMORY
Dockerfile - EVEY_DB_PASSWORD_FILE, MEILI_MASTER_KEY_FILE
-->
