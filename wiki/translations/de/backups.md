---
source_sha: 97b31b19a771829c011c797f4712f4cccf1903064069d080bb2eaadb59c386e2
reviewed: false
---
# Sicherungen und Updates

## Was eine Sicherung enthält

Eine Sicherung ist eine Kopie der **Datenbank**: Konten, Profile, Bibliotheken
und ihre Metadaten, Verlauf und Wiedergabepositionen, Sammlungen und Playlists,
Einstellungen, Plugin-Einträge, Webhooks und Audit-Logs. Sie ist eine
`.tar.gz`-Datei mit dem SQL-Dump und einem Manifest mit dem SHA256 des Dumps.

Sie enthält **nicht**:

- Ihre Mediendateien;
- den Transkodierungs-Cache (wird bei Bedarf neu erzeugt);
- den Suchindex (wird aus der Datenbank neu aufgebaut);
- die Schlüssel in `./evey-data`: `jwt.key`, der die Sitzungen signiert, und
  den Ordner `keys` (Datenbankpasswort, Suchschlüssel);
- die installierten Plugin-Dateien.

Eine Datenbanksicherung stellt Eveys Zustand also auf derselben Installation
wieder her. Um eine Maschine neu aufbauen zu können, bewahren Sie zusätzlich
eine Kopie des ganzen Ordners `./evey-data` auf: Die Datenbanksicherungen
liegen darin, in `./evey-data/backups`.

## Geplante Sicherungen

**Admin → Sicherung & Wiederherstellung → Zeitplan**: ein Cron-Ausdruck (zum
Beispiel `0 3 * * *`, jeden Tag um 3 Uhr) und eine Aufbewahrung in Tagen.
Sicherungen, die älter als die Aufbewahrung sind, werden nach jedem geplanten
Lauf gelöscht. **Jetzt ausführen** erstellt sofort eine.

<figure class="evey-shot" markdown="span">
  ![Admin → Sicherung & Wiederherstellung mit einem täglichen Zeitplan](images/admin-backup.webp){ loading=lazy }
  <figcaption>Eine Sicherung jeden Tag um 3 Uhr, 30 Tage aufbewahrt (Oberfläche auf Englisch).</figcaption>
</figure>

Der Server sichert die Datenbank außerdem von selbst, bevor er während eines
Updates eine Migration anwendet, unabhängig vom Zeitplan (siehe unten).

## Wiederherstellen

**Admin → Sicherung & Wiederherstellung** listet die Sicherungen auf und stellt
eine davon wieder her, oder eine hochgeladene Datei. Die Wiederherstellung
ersetzt die aktuelle Datenbank.

Eine Sicherung lässt sich nur auf einem Server mit demselben Datenbankschema
wiederherstellen: Der Server prüft zuerst die Prüfsumme und die Schemaversion
und verweigert eine Sicherung, die von einer Version mit anderem Schema stammt,
statt sie zu laden. Um mit ihren Daten zu einer älteren Version zurückzukehren,
siehe [Zurückgehen](#going-back) unten.

Startet der Server nicht mehr, stellt die Kommandozeile die vor einer Migration
erstellten Dumps (`pre-migration-*.sql.gz`) wieder her, nicht die
`.tar.gz`-Sicherungen dieses Bereichs: siehe [Wiederherstellung](runbooks/recovery.md).

## Evey aktualisieren

```bash
docker compose pull && docker compose up -d
docker compose logs -f evey
```

Das ist das ganze Update. Muss die neue Version die Datenbank ändern, schreibt
sie zuerst einen Dump nach `./evey-data/backups/pre-migration-*` und wendet dann
beim Start die Migrationen an; ein großer Katalog kann einige Minuten dauern,
und der Container meldet sich als gesund, sobald er wieder ausliefert. Evey
prüft alle 6 Stunden, ob ein neueres Image veröffentlicht wurde, und zeigt das
im Admin-Bereich an; er aktualisiert sich nie selbst.

**Legen Sie fest, was Sie betreiben.** `latest` folgt der neuesten Version. Für
einen Server, auf den Sie sich verlassen, setzen Sie ein Release-Tag (`0.2.0`)
oder ein unveränderliches Tag `sha-<commit>` in die Zeile `image:`, damit zwei
Pulls im Abstand eines Tages denselben Code ausführen. Ein Branch-Tag wie
`beta-0.0.1` bewegt sich mit jeder Änderung.

**Welche Version läuft.** `docker exec evey-server evey version` gibt die
Version und den Commit des laufenden Programms aus.

### Zurückgehen

Setzen Sie das vorherige Tag in die Zeile `image:` und führen Sie
`docker compose up -d` aus. Hatte die neuere Version die Datenbank migriert,
stellen Sie zuerst ihren Dump von vor der Migration wieder her:
`docker exec evey-server evey admin rollback` gibt die genauen Schritte mit dem
Pfad des Dumps aus.

Image-Tags, Laufzeitvarianten und GPU-Varianten sind unter
[Erweiterte Bereitstellungen](operations/advanced-deployments.md#image-tags)
beschrieben.
