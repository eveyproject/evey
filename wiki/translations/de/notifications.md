---
source_sha: eea86697845889523ecbd07dd3d88da0143ac37f477404249051c73e160c80c8
---
# Benachrichtigungen

Evey kann Ihre Benutzer benachrichtigen, wenn neue Medien hinzukommen, eine
Sicherung scheitert oder andere Ereignisse eintreten.

## Kanäle

### Benachrichtigungen in der Anwendung
Immer aktiv. Die Benutzer sehen ein Glockensymbol mit der Anzahl der ungelesenen
Benachrichtigungen. Einzurichten ist dafür nichts.

### E-Mail (SMTP)
Verschickt eine E-Mail, sobald eine Benachrichtigung entsteht. Voraussetzung ist
eine eingerichtete SMTP-Verbindung:

```env title=".env"
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=notifications@example.com
EVEY_SMTP_PASSWORD=your-smtp-password
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls
```

TLS-Modi:
- `starttls` - unverschlüsselt verbinden und danach auf TLS umstellen (Port 587, der übliche Fall)
- `ssl` - direkt über TLS verbinden (Port 465)
- `none` - ohne Verschlüsselung (nicht empfohlen)

### Web Push
Push-Benachrichtigungen des Browsers. Die VAPID-Schlüssel entstehen beim ersten
Start von selbst. Die Benutzer schalten den Kanal in ihren Profileinstellungen
selbst ein.

## Passwort zurücksetzen

E-Mails zum Zurücksetzen des Passworts setzen eine eingerichtete
SMTP-Verbindung voraus. Zusätzlich muss ein Administrator die Funktion
einschalten:

1. SMTP einrichten (siehe oben)
2. Im Administrationsbereich in der Serverkonfiguration
   `auth.password_recovery_enabled` auf `true` setzen
3. Auf der Anmeldeseite erscheint daraufhin der Link „Passwort vergessen?“

Wird die SMTP-Verbindung später wieder entfernt, schaltet sich das Zurücksetzen
des Passworts von selbst ab.

## Benachrichtigungsstufen

Die Benutzer legen je Medientyp fest, wie viel sie hören wollen:
- **Alle** - über alles benachrichtigen
- **Relevant** - nur, wenn die Genres zum bisher Gesehenen passen
- **Nur Wichtiges** - dieselbe Prüfung, strenger angelegt
- **Deaktiviert** - für diesen Typ gar keine Benachrichtigungen
