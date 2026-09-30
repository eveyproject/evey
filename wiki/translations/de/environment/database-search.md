---
source_sha: c19f05aea8bece6fa3bf10a82d5590583384092dde0102fcdd2192bff8c0c547
reviewed: false
---
# Datenbank und Suche

Standardmäßig betreibt das Image sein eigenes Postgres und Meilisearch und
erzeugt deren Passwörter: Keine dieser Variablen wird gebraucht. Sie dienen
einer Installation, die ihre eigene Datenbank oder Suchmaschine mitbringt oder
die eingebetteten anpasst. Siehe
[Erweiterte Bereitstellungen](../operations/advanced-deployments.md).

## Datenbank

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_EMBEDDED_POSTGRES` | nicht gesetzt | Nicht gesetzt: eingebettet, wenn `EVEY_DB_HOST` nicht gesetzt ist, sonst extern. `true` oder `false`, um ausdrücklich zu entscheiden. Vom Entrypoint gelesen. |
| `EVEY_POSTGRES_DATA_DIR` | `/data/postgres` | Wo der eingebettete Cluster liegt. Muss auf einem lokalen Dateisystem liegen, nicht auf einer Netzwerkfreigabe. |
| `EVEY_DB_HOST` | `localhost` | Der Datenbank-Host. **Ihn zu setzen schaltet die eingebettete Datenbank ab.** |
| `EVEY_DB_PORT` | `5432` | Der Datenbank-Port. |
| `EVEY_DB_USER` | `evey` | Der Datenbankbenutzer. |
| `EVEY_DB_NAME` | `evey` | Der Datenbankname. |
| `EVEY_DB_PASSWORD` | erzeugt | Das Passwort. Mit einer externen Datenbank verweigert der Server den Start mit dem Wert `evey`. |
| `EVEY_DB_PASSWORD_FILE` | `/data/keys/postgres.password` | Eine Datei mit dem Passwort, gelesen, wenn `EVEY_DB_PASSWORD` nicht gesetzt ist. Vom Image gesetzt. |
| `EVEY_DB_SSLMODE` | `disable` auf Loopback, sonst `prefer` | `disable`, `prefer`, `require` oder `verify-full`. |
| `EVEY_DB_MAX_OPEN_CONNS` | `25` | Offene Verbindungen zur Datenbank. Jede kostet auf der Seite von Postgres Speicher; erhöhen Sie den Wert nur für eine große Bereitstellung. |
| `EVEY_DB_MAX_IDLE_CONNS` | `10` | Im Pool ungenutzt gehaltene Verbindungen. |

## Suche

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_EMBEDDED_MEILI` | `true` | `false` nutzt das Meilisearch unter `EVEY_MEILI_URL` statt des eingebetteten. Vom Entrypoint gelesen. |
| `EVEY_MEILI_URL` | `http://localhost:7700` | Die Adresse von Meilisearch. |
| `MEILI_MASTER_KEY` | erzeugt | Der Meilisearch-Schlüssel. Bei einer externen Instanz verweigert ein leerer oder Platzhalter-Schlüssel den Start. |
| `MEILI_MASTER_KEY_FILE` | `/data/keys/meili.key` | Eine Datei mit dem Schlüssel, gelesen, wenn `MEILI_MASTER_KEY` nicht gesetzt ist. Vom Image gesetzt. |
| `MEILI_MAX_INDEXING_MEMORY` | etwa 20 % des Container-Speichers, mindestens 128 MiB | Der Speicher, den das eingebettete Meilisearch zum Indexieren nutzen darf. Aus der Grenze des Containers berechnet, nicht aus dem Speicher der Maschine, damit ein Scan einen kleinen Container nicht erschöpft. Ein ausdrücklicher Wert wird durchgereicht. |
