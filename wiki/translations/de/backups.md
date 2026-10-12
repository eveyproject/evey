---
source_sha: 3529db45dbbd12f6980bca0989017583b6ccad6c3c5a3b54597887c6b09d6285
reviewed: true
---
# Sicherungen und Updates

Zwei Gewohnheiten halten Ihren Server sicher:

- **Sichern.** Schalten Sie eine tägliche Sicherung der Datenbank ein, und
  bewahren Sie eine Kopie des Ordners `evey-data` an einem anderen Ort auf.
- **Aktualisieren.** Ein Befehl holt die neue Version. Evey sichert die
  Datenbank vorher von selbst, wann immer ein Update sie verändert.

Diese Seite zeigt beides, danach, was eine Sicherung enthält, wie Sie eine
Sicherung wiederherstellen und wie Sie zu einer älteren Version zurückkehren.

## Sicherungen planen

1. Gehen Sie zu **Verwaltung → Sicherung & Wiederherstellung**, Karte
   **Zeitplan**.
2. Legen Sie unter **Cron-Ausdruck** fest, wann sie laufen soll. `0 3 * * *`
   heißt jeden Tag um 3 Uhr. (Das Format ist Minute, Stunde, Tag, Monat,
   Wochentag.)
3. Legen Sie unter **Aufbewahrung (Tage)** fest, wie lange jede Sicherung
   aufbewahrt wird, zum Beispiel `30`. Ältere Sicherungen werden nach jedem
   geplanten Durchlauf gelöscht.
4. Setzen Sie das Häkchen bei **Aktiviert** und drücken Sie auf
   **Speichern**.

**Jetzt ausführen** erstellt sofort eine Sicherung, ebenso **Sicherung
erstellen** oben auf der Seite.

<figure class="evey-shot" markdown="span">
  ![Verwaltung → Sicherung & Wiederherstellung mit einem täglichen Zeitplan](images/admin-backup.webp){ loading=lazy }
  <figcaption>Eine Sicherung jeden Tag um 3 Uhr, 30 Tage aufbewahrt (Oberfläche auf Englisch).</figcaption>
</figure>

Sicherungen werden in `./evey-data/backups` gespeichert. Das liegt in dem
Ordner, den Sie ohnehin auf eine andere Festplatte kopieren sollten (siehe
[Was eine Sicherung enthält](#what-a-backup-contains)).

??? info "Über die Kommandozeile"

    Ein Skript oder ein Cron-Job auf dem Host kann dieselbe Sicherung
    erstellen:

    ```bash
    docker exec evey-server evey admin backup
    ```

    Der Befehl folgt dem Telemetrie-Schalter im Verwaltungsbereich (siehe
    unten); `--include-telemetry` nimmt die Telemetrie nur für diesen
    Durchlauf mit.

Der Server sichert die Datenbank außerdem von selbst, bevor ein Update sie
verändert, unabhängig vom Zeitplan (siehe [Evey aktualisieren](#updating-evey)).

## Was eine Sicherung enthält

Eine Sicherung ist eine Kopie der **Datenbank**: Konten, Profile, Bibliotheken
und ihr Katalog, Wiedergabeverlauf und Fortsetzungspunkte, Sammlungen,
Playlists und Favoriten, Einstellungen, Plugin-Einträge, Webhooks,
API-Schlüssel, Benachrichtigungen und Audit-Protokolle.

Sie enthält **nicht**:

- Ihre Mediendateien;
- den Video-Cache (er füllt sich von selbst wieder);
- den Suchindex (er wird aus der Datenbank neu aufgebaut);
- die Schlüssel in `./evey-data`: `jwt.key`, der jede Anmeldung schützt, und
  den Ordner `keys` (Datenbankpasswort, Suchschlüssel);
- die Dateien der installierten Plugins.

**Bewahren Sie deshalb auch eine Kopie des ganzen Ordners `./evey-data`
auf.** Eine Datenbanksicherung stellt Evey auf derselben Installation wieder
her. Um einen Rechner von Grund auf neu aufzubauen, brauchen Sie den Ordner.

??? info "Telemetrie und das Dateiformat"

    Messwerte zu Wiedergabe und Server (hinter den Diagrammen zu Qualität und
    Last) bleiben standardmäßig draußen: Sie wachsen am schnellsten und sagen
    nichts über Ihren Katalog. **Telemetrie in Sicherungen einschließen**
    (**Verwaltung → Serverkonfiguration**) nimmt sie wieder auf, für manuelle,
    geplante und über die Kommandozeile erstellte Sicherungen. Beim
    Wiederherstellen einer Sicherung werden diese Tabellen immer geleert.

    Eine Sicherung ist eine `.tar.gz`-Datei mit dem Datenbank-Dump und einer
    kurzen Beschreibung samt Prüfsumme (SHA256) und Schemaversion.

## Eine Sicherung wiederherstellen

**Verwaltung → Sicherung & Wiederherstellung** listet die Sicherungen auf.
Stellen Sie eine aus der Liste wieder her, oder laden Sie mit **Aus Datei
wiederherstellen** eine Datei hoch. Das Wiederherstellen ersetzt die aktuelle
Datenbank: Alles seit dieser Sicherung geht verloren.

!!! warning "Nur eine Sicherung aus einer Version mit demselben Datenbankaufbau"
    Der Server prüft zuerst die Datei und den Aufbau der Datenbank (die
    Schemaversion). Eine Sicherung aus einer Version mit anderem Aufbau lehnt
    er ab, statt sie zu laden. Wie Sie mit Ihren Daten zu einer älteren
    Version zurückkehren, steht unter [Zurückkehren](#going-back).

Startet der Server nicht mehr, kann die Kommandozeile die Sicherheitskopien
wiederherstellen, die vor einem Update angelegt wurden
(`pre-migration-*.sql.gz`), nicht die `.tar.gz`-Sicherungen aus diesem Bereich:
siehe [Wiederherstellung](runbooks/recovery.md).

## Evey aktualisieren

In dem Ordner, in dem Ihre Compose-Datei liegt:

```bash
docker compose pull && docker compose up -d
docker compose logs -f evey
```

Das ist das ganze Update.

- **Ihre Daten werden zuerst gesichert.** Muss die neue Version die Datenbank
  verändern, schreibt sie zuerst eine Kopie nach
  `./evey-data/backups/pre-migration-*.sql.gz` (die drei neuesten werden
  aufbewahrt) und nimmt die Änderung dann beim Start vor.
- **Das kann bei einem großen Katalog ein paar Minuten dauern.** Der Container
  meldet sich als gesund, sobald er wieder antwortet.
- **Evey sagt Ihnen, wenn es ein Update gibt.** Es prüft jede Stunde, ob ein
  neueres Image veröffentlicht ist, und zeigt das im Verwaltungsbereich an. Es
  aktualisiert sich nie selbst.
- **Welche Version läuft:** `docker exec evey-server evey version` gibt die
  Version und den Commit aus.

??? info "Die genaue Version festlegen, die Sie betreiben"

    Während der Beta gibt es keine nummerierten Versionen und keinen Tag
    `latest`. Der Tag `beta-0.0.1` ändert sich mit jedem Update. Für einen
    Server, auf den Sie sich verlassen, tragen Sie in der Zeile `image:` einen
    festen Tag `sha-<commit>` ein. So laufen zwei Pulls im Abstand eines Tages
    mit demselben Code. Sobald es Versionen gibt, folgt `latest` der neuesten
    stabilen Version, und ein Versions-Tag (`0.2.0`) legt sie fest.

### Zurückkehren

1. Tragen Sie den vorherigen Tag in die Zeile `image:` Ihrer Compose-Datei
   ein.
2. Hat die neuere Version die Datenbank verändert, stellen Sie zuerst deren
   Kopie von vor dem Update wieder her:
   `docker exec evey-server evey admin rollback` gibt die genauen Schritte
   aus, mit dem Pfad der Datei.
3. Führen Sie `docker compose up -d` aus.

Image-Tags, weitere Varianten und Builds für Grafikkarten sind unter
[Fortgeschrittene Bereitstellungen](operations/advanced-deployments.md#image-tags-and-variants)
beschrieben.

<!-- sources
contents: internal/app/backup/service.go - BackupTables, CreateBackup (sha256 + schema version in manifest), RestoreBackup (checksum + schema guard), CleanupOldBackups ; internal/adapters/driven/filesystem/backup_store.go
schedule: i18n admin.backup.schedule.* (cronLabel, retentionLabel, runNow) ; admin.backup.createBackup, restoreFromFile ; cmd/evey/main.go (DB cron scheduler, env fallback)
telemetry left out by default, emptied on restore: internal/adapters/driven/postgres/backup.go - DumpAll, telemetryTables ; internal/app/backup/service.go - WithIncludeTelemetry ; internal/adapters/driving/http/settings_handler.go - BackupIncludeTelemetryKey ; i18n admin.serverSettings.backupTelemetry, backupTelemetryHint
command-line backup: cmd/evey/admin_backup.go - adminBackup (--include-telemetry, else the panel setting)
keys outside the dump: internal/config/config.go (EVEY_JWT_PRIVATE_KEY_PATH /data/jwt.key, /data/keys)
pre-migration dump at boot, 3 kept: cmd/evey/database.go - RunMigrations BackupOpts Retention 3 ; internal/adapters/driven/postgres/migrate_backup.go (pre-migration-<stamp>.sql.gz, EVEY_BACKUP_DIR default /data/backups)
update check hourly, never self-updates: internal/app/updatecheck/service.go - checkInterval, firstCheck
rollback advisor: cmd/evey/admin_commands.go - adminRollback ; image tags: .github/workflows/ci.yml (beta-0.0.1 + sha-<short>, latest gated on vX.Y.Z)
schedule card controls (Enabled, Save, Run now): frontend/apps/web/src/routes/_auth/admin/backup.lazy.tsx ; i18n admin.backup.schedule.enabledLabel, save, cronHelp
-->
