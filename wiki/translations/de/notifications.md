---
source_sha: 437e3591ffafde2e124be29a142fc1cfa7ecb154ea3e77dfbcb32e462668f452
reviewed: false
---
# Benachrichtigungen

Evey sagt jedem Profil Bescheid, wenn neue Filme, Serien, Staffeln, Folgen oder
Alben eintreffen. Administratoren erfahren außerdem von fehlgeschlagenen
Sicherungen, Plugin-Fehlern und neuen Medienanfragen, und wer eine Anfrage
gestellt hat, hört zurück, sobald sie beantwortet ist.

## Wo Benachrichtigungen erscheinen

- **In jeder App**: die Glocke mit der Zahl der ungelesenen. Immer aktiv,
  nichts einzurichten. Eine Benachrichtigung wird 30 Tage aufbewahrt. Die Liste
  zeigt nur Titel aus Bibliotheken, die das Konto öffnen darf.
- **Browser-Push**: Im Web schaltet jedes Profil **Push-Benachrichtigungen**
  unter **Einstellungen → Benachrichtigungen** ein, einmal pro Browser. Der
  Server erzeugt seine Push-Schlüssel beim ersten Start; siehe
  [E-Mail und Benachrichtigungen](environment/notifications.md#browser-push),
  um eigene mitzubringen. Browser erlauben Push nur unter einer HTTPS-Adresse
  (oder `localhost`); anderswo zeigt die Einstellung **Push-Benachrichtigungen brauchen eine HTTPS-Adresse: Öffnen Sie diesen Server über HTTPS, um sie einzuschalten.** Die Android- und TV-Apps erhalten
  keine Push-Benachrichtigungen.
- **E-Mail**: Sobald E-Mail eingerichtet ist, geht jede Benachrichtigung auch
  an die Adresse des Kontos, in der Sprache des Profils und mit einem Link zum
  Server. Ein Konto mit mehreren Profilen erhält eine E-Mail pro Ereignis. Es
  gibt keine Abmeldung pro Benutzer.

## E-Mail einrichten

**Admin → Serverkonfiguration → E-Mail / SMTP**: der Mailserver, sein Port, das
Anmeldekonto, die Absenderadresse und der TLS-Modus. Speichern gilt sofort,
ohne Neustart. **Test-E-Mail senden** schickt mit den gespeicherten
Einstellungen eine kurze Nachricht an die Adresse Ihres eigenen Kontos und zeigt
bei einem Fehler die Antwort des Mailservers.

`starttls` hebt eine Klartextverbindung auf TLS (meist Port 587), `ssl` nutzt
TLS von Anfang an (meist Port 465), `none` sendet ohne Verschlüsselung.

Die Variablen `EVEY_SMTP_*` sind der Rückfall, solange die Karte keinen Host hat:

```env title=".env"
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=notifications@example.com
EVEY_SMTP_PASSWORD=your-smtp-password
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls
```

Sobald in der Karte ein Host gespeichert ist, kommen alle Felder aus der Karte
und die Variablen werden ignoriert: Der Server schickt die Zugangsdaten der
Variablen also nie an einen im Admin-Bereich eingetragenen Mailserver.

E-Mail dient Benachrichtigungen und dem Zurücksetzen von Passwörtern;
Einladungen sind Links, die Sie selbst weitergeben.

## Was jedes Profil erhält

Unter **Einstellungen → Benachrichtigungen** wählt jedes Profil eine Stufe für
**Filme**, **Serien** und **Musik**:

| Stufe | Gemeldete neue Titel |
| --- | --- |
| **Alle** | Jeder. Die Vorgabe. |
| **Relevant** | Nur solche, die ein Genre mit den üblichen Genres des Profils teilen. |
| **Nur wichtige** | Derzeit dasselbe wie Relevant. |
| **Deaktiviert** | Keiner. |

Ein Genre gilt als üblich, wenn es in mindestens 3 der letzten 200 Wiedergaben
des Profils vorkommt. Ein Titel ohne Genres wird auf den Stufen Relevant und Nur
wichtige nie gemeldet.

Mehrere Folgen einer Serie, die innerhalb von 24 Stunden eintreffen, ergeben
eine einzige Benachrichtigung, die sie zählt, und keine wird gesendet, wenn die
Serie oder die Staffel gerade angekündigt wurde.

## Passwort-Wiederherstellung

1. Richten Sie E-Mail ein (siehe oben).
2. Schalten Sie **Passwort-Wiederherstellung** unter **Admin →
   Serverkonfiguration → Servereinstellungen** ein. Das gilt sofort.
3. Die Anmeldeseite zeigt dann **Passwort vergessen?**, das einen Link zum
   Zurücksetzen per E-Mail schickt.

Ohne SMTP bleibt der Link verborgen, auch wenn die Option eingeschaltet ist.
