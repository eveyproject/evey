---
source_sha: 358930f5d871a10d9184002e80eb788d14685e06174afd3a569a059380aa6d0b
reviewed: false
---
# Fehlerbehebung

Beginnen Sie beim Symptom. Jeder Eintrag sagt, was zuerst zu prüfen ist und wo
das ausführliche Vorgehen steht.

## Erste Prüfungen

Zwei Befehle beantworten die meisten Fragen zu einem Server, der sich seltsam
verhält:

```bash
docker compose logs --tail=200 evey
docker exec evey-server evey admin doctor
```

`evey admin doctor` prüft die Datenbank und ihr Schema, den Signaturschlüssel,
den Suchindex, die Medieneinbindungen (auch, ob der Server in jede Bibliothek
schreiben kann), die Cache-Verzeichnisse, ffmpeg, den Fernzugriff und den
freien Speicherplatz. Er gibt eine Zeile pro Prüfung mit einer Abhilfe aus und
endet mit einem Wert ungleich null, wenn eine fehlschlägt.

Im Admin-Bereich zeigt **Protokolle & Debug** dieselben Logs, durchsuchbar und
nach Bereich gefiltert (Wiedergabe, Transkodierung, Scan...).

## Der Server startet nicht

| Was im Log steht | Was es bedeutet |
| --- | --- |
| Eine Meldung der Konfigurationsprüfung | Mit einer externen Datenbank verweigert der Server das Standardpasswort `evey`; mit einem externen Meilisearch einen leeren oder Platzhalter-Schlüssel. Korrigieren Sie den in der Meldung genannten Wert. |
| Zeilen, die mit `evey:` beginnen, zu Postgres | Die eingebettete Datenbank wollte nicht starten: ein Cluster einer anderen Postgres-Hauptversion ([Postgres-Hauptversions-Upgrade](advanced-deployments.md#postgres-major-upgrade)), ein Datenverzeichnis, das kein Postgres-Cluster ist, eine volle Festplatte oder `./evey-data/postgres` auf einer Netzwerkfreigabe. |
| `permission denied` auf `/cache/...`, in einer Neustartschleife | Das Volume `evey-cache` gehört einem anderen Benutzer als dem, unter dem der Server läuft. Aktualisieren Sie das Image; bei einem Image von vor dem 17.09.2026 führen Sie einmal `docker run --rm -v <project>_evey-cache:/c alpine chown -R <PUID>:<PGID> /c` aus. |

Ein verlorenes Administratorpasswort, ein kompromittierter Signaturschlüssel
oder eine beschädigte Datenbank: [Wiederherstellung](../runbooks/recovery.md).

## Von außerhalb nicht erreichbar

Der Server antwortet zu Hause, aber nicht anderswo, oder die Apps zeigen eine
Meldung, dass der Fernzugriff deaktiviert ist: siehe
[Fernzugriff und HTTPS](../administration/remote-access.md). Die Admin-Seite
**Fernzugriff** zeigt in Rot an, wenn der Server aus dem Internet erreichbar
ist, während der Fernzugriff aus ist.

Gar nichts antwortet, auch zu Hause nicht:
[Server nicht erreichbar](../runbooks/incident-runbook.md#1-server-not-reachable).

## Alle sind abgemeldet, oder jede Anmeldung schlägt fehl

[Alle Anmeldungen liefern 401](../runbooks/incident-runbook.md#2-all-logins-return-401).
Die üblichen Ursachen sind die Datenbank (jede Anfrage prüft das Konto dort und
wird abgewiesen, wenn sie nicht antwortet; das Log sagt es) und ein Reverse
Proxy, der unter **Vertrauenswürdige Proxys** fehlt, wodurch alle unter ein
einziges Anmeldelimit fallen. Ein verlorener oder ersetzter Signaturschlüssel
allein meldet niemanden ab: Die Apps erneuern ihre Token.

## Die SSO-Anmeldung schlägt fehl

Die Anmeldeseite meldet „SSO-Anmeldung fehlgeschlagen":

- die Person hat bereits ein lokales Konto mit derselben E-Mail oder demselben
  Benutzernamen;
- ihre Adresse liegt außerhalb von `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS`, oder der
  Anbieter hat sie nicht als verifiziert gekennzeichnet (das Log sagt
  `oidc: email domain not allowed` oder
  `oidc: unverified email rejected under domain allowlist`).

Eine Fehlerseite des Anbieters selbst bedeutet meist, dass die dort
registrierte Weiterleitungs-URI nicht die ist, die Evey verwendet. Siehe
[OIDC / SSO einrichten](../oidc.md).

## E-Mails kommen nicht an

SMTP wird beim Start aus den Variablen `EVEY_SMTP_*` gelesen, nicht aus der
Karte **E-Mail / SMTP**. Ist es aktiv, zeigt das Startlog
`SMTP email notifications enabled` mit dem Host. Ein fehlgeschlagener Versand
wird als `email: send notification` mit der Antwort des Servers protokolliert;
ein TLS-Modus, der nicht zum Port passt (`starttls` mit 587, `ssl` mit 465), ist
der übliche Fehler. Siehe [Benachrichtigungen](../notifications.md).

## Ein Titel läuft transkodiert, obwohl Sie Direktwiedergabe erwartet hatten

Öffnen Sie **Stream-Info** im Player: Sie nennt die Methode und die Gründe.
Siehe [Wie Evey die Wiedergabeart wählt](../playback/how-playback-is-decided.md),
um sie zu lesen.

## Die Wiedergabe stoppt, puffert oder startet nicht

- Nur ein Titel: Öffnen Sie **Admin → Medienzustand**. Eine Datei ohne
  Wiedergabeindex, ein beschädigter Abschnitt oder in Blöcken gespeicherte
  Spuren erklären Aussetzer und langsame Sprünge, und die meisten davon werden
  an Ort und Stelle repariert. Siehe [Medienzustand](../runbooks/media-health.md).
- Mehrere Titel, wenn mehrere Personen schauen: Der Server ist vielleicht an
  seiner Transkodierungskapazität. Siehe
  [Transkodierung und Serverkapazität](../playback/transcoding.md).
- Die transkodierte Wiedergabe schlägt ganz fehl:
  [Die Transkodierung schlägt fehl](../runbooks/incident-runbook.md#3-transcoding-fails--videos-wont-play-transcoded).
- Es puffert, und die Logs sagen nicht, warum:
  [das Diagnoseverfahren](../runbooks/incident-runbook.md#3c-it-buffers--it-cut-out-and-the-logs-do-not-say-why).
- **Admin → Streaming-Qualität** zeigt für echte Zuschauer über den letzten Tag
  oder die letzte Woche, wie oft die Wiedergabe gewartet hat und welche Titel
  und Geräte betroffen sind.

## Kein Ton, oder Ton nicht synchron

- Nur im Browser, mit einer Dolby-Digital-Spur (AC-3/E-AC-3): Ein Reverse Proxy,
  der seine eigene Content-Security-Policy setzt, kann den Decoder blockieren,
  den der Web-Player verwendet. Siehe [Audio](../playback/audio.md#dolby-digital-in-a-browser).
- Nicht synchron bei einem Titel: **Admin → Medienzustand** erkennt Audio, das
  nach dem Bild beginnt, und Spuren, die untereinander nicht übereinstimmen.

## Untertitel erscheinen nicht

Siehe [Untertitel](../playback/subtitles.md): welche Formate die Player
zeichnen und welche der Server ins Bild einbrennen muss.

## Poster fehlen oder sind kaputt

Poster und Hintergründe werden bei Scans auf den Server kopiert, nach
`./evey-data/artworks`, und fehlende werden stündlich erneut versucht.
**Admin → Artwork-Cache** zeigt, wie viele noch keine lokale Kopie haben;
**Fehlende Artworks herunterladen** holt sie sofort, und **Beschädigte
Varianten bereinigen** entfernt verkleinerte Poster, die ein fehlgeschlagenes
Verkleinern leer hinterlassen hat und die dann bei Bedarf neu erzeugt werden.

## Die Suche liefert nichts

Bauen Sie den Index aus der Datenbank neu auf:
`docker exec evey-server evey admin reindex-search`. Der Index lässt sich immer
neu aufbauen; nichts geht verloren.

## Eine Cast-Sitzung ist eingefroren oder stumm geworden

[Ein Cast ist eingefroren, stumm geworden, oder das Telefon dreht sich endlos](../runbooks/incident-runbook.md#8-a-cast-froze-went-silent-or-the-phone-keeps-spinning).

## Ein Plugin funktioniert nicht mehr

[Ein Plugin ist abgestürzt](../runbooks/incident-runbook.md#7-plugin-crashed).
Seine eigenen Logs stehen auf seiner Karte unter **Admin → Plugins**.

## Die Festplatte läuft voll

Der Transkodierungs-Cache ist die übliche Ursache:
[Festplatte des Transkodierungs-Caches voll](../runbooks/incident-runbook.md#6-transcode-cache-disk-full).
Er lässt sich jederzeit unter **Admin → Transkodierungs-Cache** leeren.
