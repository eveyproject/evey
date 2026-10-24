---
source_sha: 9ec00e9464587406ecdffdd125543fdaf6988a5989c62a715f8a88417d946b9f
reviewed: true
---

# Bekannte Einschränkungen { #known-limitations }

Was Evey noch nicht kann, klar gesagt, damit es niemand erst an dem Abend
merkt, an dem er einen Film schauen wollte. Jeder Punkt sagt, was Sie
stattdessen verwenden können.

## Keine App für iPhone, iPad oder Apple TV { #no-app-for-iphone-ipad-or-apple-tv }

Es gibt eine Evey-App für Android-Handys und -Tablets und eine für Android TV
und Google TV. Für iPhone, iPad oder Apple TV gibt es keine. Öffnen Sie auf
diesen Geräten die Adresse des Servers im Webbrowser.

Mehr: [Apps verbinden](apps.md).

## Die Android-Apps warten auf die Prüfung durch Google { #the-android-apps-are-waiting-for-googles-review }

Die Apps für Android und Android TV wurden bei Google Play eingereicht. Bis
die Prüfung bestanden ist, ist der öffentliche Eintrag nicht sichtbar, und nur
die zum Test eingeladenen Konten können sie installieren; der Link auf der
Seite [Apps](apps.md) wird zu diesem Zeitpunkt aktiv. Die Web-App und die
Seite `/tv` im Browser eines Fernsehers brauchen keine Installation und
funktionieren für alle.

Mehr: [Apps verbinden](apps.md).

## Ohne Verbindung schauen: nur mit der Android-App { #watching-without-a-connection-android-app-only }

Nur die Android-App speichert eine Kopie eines Films oder einer Folge auf dem
Gerät (**Downloads**), 30 Tage lang abspielbar, ohne den Server zu
kontaktieren. Der Web-Player, die Android-TV-App und die Seite `/tv` spielen
vom Server ab und brauchen die Verbindung die ganze Zeit.

Mehr: [Was jede App mitbringt](apps.md#what-each-app-adds).

## Keine Benachrichtigungen auf dem Handy, wenn die App geschlossen ist { #no-notifications-on-the-phone-while-the-app-is-closed }

Die Apps für Android und Android TV zeigen ihre Benachrichtigungen in ihrem
Bildschirm **Benachrichtigungen**, solange sie geöffnet sind. Bei
geschlossener App melden sie sich weder auf dem Handy noch auf dem Fernseher.
Benachrichtigungen außerhalb der App kommen vom Webbrowser (sobald sie
aktiviert sind, über `https://`) oder per E-Mail, wenn die Person, die Evey
betreibt, sie eingerichtet hat.

Mehr: [Wo Benachrichtigungen erscheinen](../notifications.md#where-notifications-appear).

## Profile werden im Web oder auf dem Handy verwaltet { #profiles-are-managed-on-the-web-or-on-the-phone }

Die Android-TV-App wechselt zwischen Profilen und ändert einige Einstellungen
des aktiven Profils, kann aber kein Profil anlegen oder löschen. Erledigen
Sie das in der Web-App oder in der Android-App.

Mehr: [Benutzer, Profile und Kindersicherung](../administration/users.md#profiles-kids-profiles-and-the-pin).

## Streamen: aus Chrome oder der Android-App { #casting-from-chrome-or-the-android-app }

Die Schaltfläche **Streamen** gibt es im Web-Player in Chrome und in der
Android-App. Firefox hat keine, und die Android-TV-App auch nicht, denn sie
ist schon der Fernseher. Safari sowie Chrome auf einem Android-Handy nutzen die
eigene Fernwiedergabe des Browsers, die anders funktioniert.

Mehr: [Streamen auf Chromecast](../playback/casting.md).

## Evey aktualisiert sich nie selbst { #evey-never-updates-itself }

Der Server prüft, ob es eine neuere Version gibt, und zeigt das im
Admin-Dashboard an, installiert aber nichts von selbst: Die Person, die ihn
betreibt, aktualisiert, wann es ihr passt, mit zwei Befehlen. Die
Android-Apps zeigen einen Hinweis, wenn sie älter als der Server sind.

Mehr: [Evey aktualisieren](../backups.md#updating-evey) und
[Die Apps aktuell halten](apps.md#keeping-the-apps-up-to-date).
