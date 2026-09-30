---
source_sha: f6d6e61c9914bed7f2744ebf8dec6a79fc1769dd750e8b0f2109b4ab14d44526
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
