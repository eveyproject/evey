---
source_sha: 5dca3b7fe64acb5cf79df57c6da42ed90ae968596cba80b381b921667c8e36e9
reviewed: false
---
# Benachrichtigungen

Evey kann Benutzer benachrichtigen, wenn neue Medien hinzukommen, Sicherungen fehlschlagen und bei anderen Ereignissen.

## Kanäle

### In der App
Immer aktiv. Benutzer sehen ein Glockensymbol mit der Zahl der ungelesenen Benachrichtigungen. Keine Konfiguration nötig.

### E-Mail (SMTP)
Sendet eine E-Mail, wenn eine Benachrichtigung entsteht. Erfordert eine SMTP-Konfiguration:

```env title=".env"
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=notifications@example.com
EVEY_SMTP_PASSWORD=your-smtp-password
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls
```

TLS-Modi:
- `starttls` - unverschlüsselt verbinden, dann auf TLS umstellen (Port 587, am häufigsten)
- `ssl` - direkt mit TLS verbinden (Port 465)
- `none` - keine Verschlüsselung (nicht empfohlen)

### Web-Push
Push-Benachrichtigungen im Browser. VAPID-Schlüssel werden beim ersten Start automatisch erzeugt. Benutzer melden sich in ihren Profileinstellungen an.

## Passwort-Wiederherstellung

E-Mails zum Zurücksetzen des Passworts erfordern eine SMTP-Konfiguration. Außerdem muss ein Administrator die Funktion einschalten:

1. SMTP konfigurieren (siehe oben).
2. **Passwort-Wiederherstellung** unter **Admin → Serverkonfiguration**
   einschalten. Das gilt sofort.
3. Die Anmeldeseite zeigt dann einen Link „Passwort vergessen?".

Wird SMTP nach dem Einschalten entfernt, wird die Passwort-Wiederherstellung automatisch deaktiviert.

## Benachrichtigungsstufen

Benutzer können ihre Benachrichtigungsvorliebe pro Medientyp festlegen:
- **Alle** - bei allem benachrichtigen
- **Relevant** - nur wenn die Genres zum Verlauf des Benutzers passen
- **Nur wichtige** - strengerer Genre-Abgleich
- **Deaktiviert** - keine Benachrichtigungen für diesen Typ
