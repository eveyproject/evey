---
source_sha: 379364a92d0108cdaeaf5ac5c24f9bf18ca1e30aa45e876c7e38da2ea3d133ca
reviewed: false
---
# Überwachung und Logs

Wo man sieht, was der Server tut, vom Unmittelbarsten bis zum Zusammengefassten.
Für ein zu untersuchendes Symptom beginnen Sie mit der
[Fehlerbehebung](troubleshooting.md).

## Was gerade läuft

**Admin → Aktuelle Wiedergabe** listet jeden laufenden Stream: wer, was, auf
welchem Gerät und wie, Spur für Spur (kopiert, transkodiert und das Backend,
das kodiert). Ein Administrator kann eine Sitzung **Stoppen** oder dem
Zuschauer eine **Nachricht senden**.

Die Karte **Wer ist online** im Dashboard listet, wer gerade verbunden ist,
nach Profil, mit der Zeit seit dem letzten Lebenszeichen jeder App. Ein
Chromecast, dessen Fernseher Eveys Receiver geladen hält, bleibt nach dem Ende
seiner Wiedergabe gelistet, auch wenn er aus **Aktuelle Wiedergabe** verschwunden
ist; **Diesen Chromecast stoppen** schickt ihm einen Stoppbefehl, den der
Fernseher innerhalb von 30 Sekunden ausführt.

**Admin → Systemzustand** zeigt Prozessor, Speicher, Bandbreite und
Speicherplatz, und wie viel der Transkodierungskapazität genutzt wird.

<figure class="evey-shot" markdown="span">
  ![Admin → Systemzustand auf dem Demo-Server](../images/admin-system-health.webp){ loading=lazy }
  <figcaption><strong>Admin → Systemzustand</strong>, darunter die laufenden Hintergrundaufgaben (Oberfläche auf Englisch).</figcaption>
</figure>

## Logs

**Admin → Protokolle & Debug** durchsucht die Logs des Servers und verfolgt sie
live, nach Bereich gefiltert (Wiedergabe, Transkodierung, Scan, Untertitel...).
Hier sieht man zuerst nach, wenn ein Benutzer ein Problem meldet. Dieselben Logs
gibt es mit `docker compose logs evey`.

## Wie die Wiedergabe tatsächlich lief

**Admin → Streaming-Qualität** baut auf dem auf, was die Player während der
Wiedergabe melden (Pufferstand, erneutes Puffern, Durchsatz, im CMCD-Format):
wie oft Zuschauer gewartet haben, die Titel, die am häufigsten stocken, und die
Gerätearten, die Mühe haben, über den letzten Tag oder die letzte Woche.

Der Server handelt auch danach. Lässt eine Art von Quelle Zuschauer immer wieder
warten, senkt er für eine Weile die Höchstqualität für diese Art von Quelle,
zusätzlich zur kalibrierten Obergrenze, und hebt die Grenze dann von selbst
wieder auf. Die aktiven Grenzen stehen unter **Automatische Qualitätsgrenzen**,
mit Grund und Ablauf; **Zurücksetzen** entfernt eine sofort.

## Verlauf und Statistiken

- **Admin → Analysen**: Wiedergaben heute und in dieser Woche, aktive Benutzer,
  die meistgesehenen Titel der letzten 30 Tage.
- **Admin → Aktivität**: der Verlauf der Wiedergaben und der administrativen
  Aktionen, und die laufenden Hintergrundaufgaben.

Wie lange jede Art von Verlauf aufbewahrt wird, legt **Admin →
Serverkonfiguration → Servereinstellungen** fest (Wiedergabesitzungen,
Audit-Log, Servermetriken, Streaming-Qualität). Eine Änderung gilt ab der
nächsten täglichen Bereinigung.

## Kommandozeile

`evey admin doctor` prüft den ganzen Server in einem Bericht; die anderen
Diagnosebefehle stehen unter [Kommandozeile](../runbooks/cli.md).
