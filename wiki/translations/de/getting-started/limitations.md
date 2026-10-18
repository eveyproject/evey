---
source_sha: 6a8dc35c6cee620adb9bfbba22b808325ea49623051da7c9b0980893e4c7ff09
reviewed: true
---

# Bekannte Einschränkungen

Was Evey noch nicht kann, klar gesagt, damit es niemand erst an dem Abend
merkt, an dem er einen Film schauen wollte. Jeder Punkt sagt, was Sie
stattdessen verwenden können.

## Keine App für iPhone, iPad oder Apple TV

Es gibt eine Evey-App für Android-Handys und -Tablets und eine für Android TV
und Google TV. Für iPhone, iPad oder Apple TV gibt es keine. Öffnen Sie auf
diesen Geräten die Adresse des Servers im Webbrowser.

Mehr: [Apps verbinden](apps.md).

## Die Android-Apps sind noch im Test

Die Apps für Android und Android TV sind noch nicht für alle auf Google Play.
Nur die zum Test eingeladenen Konten können sie installieren. Die Web-App und
die Seite `/tv` im Browser eines Fernsehers brauchen keine Installation und
funktionieren für alle.

Mehr: [Apps verbinden](apps.md).

## Ohne Verbindung schauen: nur mit der Android-App

Nur die Android-App speichert eine Kopie eines Films oder einer Folge auf dem
Gerät (**Downloads**), 30 Tage lang abspielbar, ohne den Server zu
kontaktieren. Der Web-Player, die Android-TV-App und die Seite `/tv` spielen
vom Server ab und brauchen die Verbindung die ganze Zeit.

Mehr: [Was jede App mitbringt](apps.md#what-each-app-adds).

## Keine Benachrichtigungen auf dem Handy, wenn die App geschlossen ist

Die Apps für Android und Android TV zeigen ihre Benachrichtigungen in ihrem
Bildschirm **Benachrichtigungen**, solange sie geöffnet sind. Bei
geschlossener App melden sie sich weder auf dem Handy noch auf dem Fernseher.
Benachrichtigungen außerhalb der App kommen vom Webbrowser (sobald sie
aktiviert sind, über `https://`) oder per E-Mail, wenn die Person, die Evey
betreibt, sie eingerichtet hat.

Mehr: [Wo Benachrichtigungen erscheinen](../notifications.md#where-notifications-appear).

## Profile werden im Web oder auf dem Handy verwaltet

Die Android-TV-App wechselt zwischen Profilen und ändert einige Einstellungen
des aktiven Profils, kann aber kein Profil anlegen oder löschen. Erledigen
Sie das in der Web-App oder in der Android-App.

Mehr: [Benutzer, Profile und Kindersicherung](../administration/users.md#profiles-kids-profiles-and-the-pin).

## Streamen: aus Chrome oder der Android-App

Die Schaltfläche **Streamen** gibt es im Web-Player in Chrome und in der
Android-App. Firefox hat keine, und die Android-TV-App auch nicht, denn sie
ist schon der Fernseher. Safari sowie Chrome auf einem Android-Handy nutzen die
eigene Fernwiedergabe des Browsers, die anders funktioniert.

Mehr: [Streamen auf Chromecast](../playback/casting.md).

## Evey aktualisiert sich nie selbst

Der Server prüft, ob es eine neuere Version gibt, und zeigt das im
Admin-Dashboard an, installiert aber nichts von selbst: Die Person, die ihn
betreibt, aktualisiert, wann es ihr passt, mit zwei Befehlen. Die
Android-Apps zeigen einen Hinweis, wenn sie älter als der Server sind.

Mehr: [Evey aktualisieren](../backups.md#updating-evey) und
[Die Apps aktuell halten](apps.md#keeping-the-apps-up-to-date).
