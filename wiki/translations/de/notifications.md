---
source_sha: 0f3c964e32d232266d53c9ebfe192d6a3f21996e2437d7ec2cc38945c049774c
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
  (oder `localhost`); anderswo zeigt die Einstellung **Push-Benachrichtigungen
  sind auf diesem Server nicht konfiguriert.** Die Android- und TV-Apps erhalten
  keine Push-Benachrichtigungen.
- **E-Mail**: Sobald SMTP eingerichtet ist, geht jede Benachrichtigung auch an
  die Adresse des Kontos. Es gibt keine Abmeldung pro Benutzer, und ein Konto
  mit mehreren Profilen erhält eine E-Mail pro Profil.

## E-Mail einrichten

SMTP wird beim Start des Servers aus diesen Variablen gelesen; starten Sie ihn
nach einer Änderung neu:

```env title=".env"
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=notifications@example.com
EVEY_SMTP_PASSWORD=your-smtp-password
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls
```

`EVEY_SMTP_TLS` ist `starttls` (eine Klartextverbindung wird auf TLS gehoben,
meist Port 587), `ssl` (TLS von Anfang an, meist Port 465) oder `none` (keine
Verschlüsselung). Die Karte **E-Mail / SMTP** unter **Admin →
Serverkonfiguration** speichert dieselben Felder, aber der Server liest sie
nicht: Verwenden Sie die Variablen.

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
