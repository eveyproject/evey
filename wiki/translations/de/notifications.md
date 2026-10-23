---
source_sha: faa0ef0aa6b7e0d5694320b07792f4f14f19fdd79e035ec57246a43c7954ffb8
reviewed: true
---
# E-Mail und Benachrichtigungen { #e-mail-and-notifications }

Evey benachrichtigt jedes Profil, wenn neue Filme, Serien, Staffeln, Folgen
oder Alben hinzukommen. Außerdem benachrichtigt es:

- **die Person, die eine Anfrage gestellt hat**, wenn sie beantwortet wird;
- **Administratoren** über neue Anfragen, fehlgeschlagene Backups,
  Plugin-Fehler und einen vollen Untertitel-Cache.

Benachrichtigungen erscheinen in den Apps, ohne dass Sie etwas einrichten.
Per E-Mail, sobald Sie einen Mailserver eingerichtet haben (siehe unten).

## Wo Benachrichtigungen erscheinen { #where-notifications-appear }

- **In jeder App**: die Glocke, mit der Zahl der ungelesenen
  Benachrichtigungen. Immer an. Eine Benachrichtigung wird 30 Tage
  aufbewahrt. Man sieht nur Titel aus Bibliotheken, die das eigene Konto
  öffnen darf.
- **Im Browser (Push)**: Im Web schaltet jedes Profil
  **Push-Benachrichtigungen** unter **Einstellungen → Benachrichtigungen**
  ein, einmal pro Browser. Browser erlauben das nur bei einer
  `https://`-Adresse (oder `localhost`). Anderswo zeigt die Einstellung
  **Push-Benachrichtigungen brauchen eine HTTPS-Adresse: Öffnen Sie diesen
  Server über HTTPS, um sie einzuschalten.** Die Android- und TV-Apps
  bekommen keine Push-Benachrichtigungen.
- **Per E-Mail**: Sobald E-Mail eingerichtet ist, geht jede Benachrichtigung
  auch an die E-Mail-Adresse des Kontos, in der Sprache des Profils, mit
  einem Link zum Server. Ein Konto mit mehreren Profilen bekommt eine E-Mail
  pro Ereignis. Man kann E-Mails nicht selbst abschalten.

??? info "Push-Schlüssel"

    Der Server erzeugt die Schlüssel, die Browser-Push braucht, bei seinem
    ersten Start. Wie Sie eigene verwenden, steht unter
    [E-Mail und Benachrichtigungen](environment/notifications.md#browser-push).

## E-Mail einrichten { #set-up-e-mail }

Evey verschickt E-Mails über einen Mailserver, bei dem Sie schon ein Konto
haben: Ihren E-Mail-Anbieter oder einen Versanddienst. Die Hilfeseiten Ihres
Anbieters nennen die Werte unten.

1. Gehen Sie zu **Verwaltung → Serverkonfiguration → E-Mail / SMTP**.
2. Füllen Sie die Felder aus:

    | Feld | Was Sie eintragen |
    | --- | --- |
    | **SMTP-Host**, **Port** | Der Mailserver und sein Port. Üblich sind `587` mit `starttls` oder `465` mit `ssl`. |
    | **Benutzername**, **Passwort** | Das Konto, mit dem sich Evey anmeldet. |
    | **Absenderadresse** | Die Absenderadresse, die auf jeder Nachricht steht. |
    | **TLS-Modus** | `starttls` oder `ssl`, passend zum Port (siehe oben). `none` sendet unverschlüsselt. |

3. Drücken Sie auf **SMTP-Einstellungen speichern**. Das gilt sofort, ohne
   Neustart.
4. Drücken Sie auf **Test-E-Mail senden**. Evey schickt eine kurze Nachricht
   an die Adresse Ihres eigenen Kontos. Schlägt das fehl, zeigt es die Antwort
   des Mailservers.

Der häufigste Fehler ist ein TLS-Modus, der nicht zum Port passt.

E-Mail wird für Benachrichtigungen und zum Zurücksetzen von Passwörtern
benutzt. Einladungen sind Links, die Sie selbst verschicken.

??? info "E-Mail stattdessen in der Umgebung einstellen"

    Die Variablen `EVEY_SMTP_*`, unter `environment:` in der Compose-Datei,
    werden nur benutzt, solange in der Karte kein Host steht:

    ```yaml title="docker-compose.yml"
    services:
      evey:
        environment:
          EVEY_SMTP_HOST: smtp.example.com
          EVEY_SMTP_PORT: "587"
          EVEY_SMTP_USERNAME: notifications@example.com
          EVEY_SMTP_PASSWORD: your-smtp-password
          EVEY_SMTP_FROM: noreply@example.com
          EVEY_SMTP_TLS: starttls
    ```

    Sobald in der Karte ein Host gespeichert ist, kommt jedes Feld aus der
    Karte, und die Variablen werden ignoriert. So schickt der Server das
    Passwort aus den Variablen nie an einen Mailserver, der im
    Verwaltungsbereich eingetippt wurde. Leeren Sie den Host, um zu den
    Variablen zurückzukehren.

## Festlegen, was ein Profil bekommt { #choose-what-a-profile-receives }

Unter **Einstellungen → Benachrichtigungen** wählt jedes Profil eine Stufe für
**Filme**, **Serien** und **Musik**:

| Stufe | Über welche neuen Titel Sie benachrichtigt werden |
| --- | --- |
| **Alle** | Über jeden. Das ist die Vorgabe. |
| **Relevant** | Nur über Titel, die ein Genre mit dem teilen, was das Profil normalerweise schaut. |
| **Nur wichtige** | Vorerst dasselbe wie Relevant. |
| **Deaktiviert** | Über keinen. |

Ein Genre gilt als „normalerweise geschaut“, wenn es in mindestens 3 der
letzten 200 Wiedergaben des Profils vorkommt. Über einen Titel ohne Genres
wird auf den Stufen Relevant und Nur wichtige nie benachrichtigt.

Kommen mehrere Folgen einer Serie innerhalb von 24 Stunden hinzu, bekommen Sie
eine einzige Benachrichtigung, die sie zählt. Keine wird verschickt, wenn die
Serie oder die Staffel gerade erst angekündigt wurde.

## Benutzern das Zurücksetzen des Passworts erlauben { #let-people-reset-their-password }

1. Richten Sie E-Mail ein (siehe oben).
2. Schalten Sie **Passwort-Wiederherstellung** unter **Verwaltung →
   Serverkonfiguration → Konten & Sicherheit → Anmeldung und Schutz** ein. Das gilt sofort.
3. Die Anmeldeseite zeigt jetzt **Passwort vergessen?**. Darüber kommt per
   E-Mail ein Link, mit dem man ein neues Passwort wählt.

Ohne Mailserver bleibt der Link verborgen, auch wenn die Option an ist.

<!-- sources
web push support: frontend/apps/web/src/lib/webpush.ts isPushSupported (serviceWorker + PushManager, secure context only) ; NotificationSettings.tsx shows pushNeedsHttps when unsupported
types: internal/domain/notification/model.go (Type*, IsAdminOnly: backup_failed, plugin_error, media_request) ; emitted: internal/app/notification/event_handlers.go, batching.go
retention 30 d: internal/app/notification/service.go defaultRetentionDays ; grouping 24 h: batching.go episodeGroupWindow + shouldSkipEpisodeNotif
list filtered by library access: internal/adapters/driving/grpcapi/notification_handler.go allowedLibraryIDs
bell on TV and mobile: frontend/apps/tv/src/components/TopHeader.tsx ; frontend/apps/mobile/app/(auth)/settings/notifications.tsx
levels: batching.go shouldNotify (relevant == highly_relevant) + fetchFavoriteGenres (GetFavoriteGenresBatch minCount 3, window 200 sessions: watch_history_analyzer.go)
defaults all: internal/adapters/driven/postgres/migrations/000005_notifications.up.sql (notification_settings_global) ; UI: frontend/apps/web/src/features/user/NotificationSettings.tsx ; i18n notificationSettings.level*
email: internal/adapters/driven/email/notification_emailer.go (push title key, recipient locale, one per account per FanoutID, base URL link)
SMTP live from the card, env fallback, panel host = panel only, defaults 587/starttls: internal/app/config/smtp.go (ResolveSMTPSettings, LiveSMTP) ; test send: internal/adapters/driving/http/smtp_test_handler.go
labels: i18n admin.serverConfig emailSmtp, smtpHost, port, smtpUsername, smtpPassword, fromAddress, tlsMode, saveSmtp, sendTestEmail
no native push in the mobile/TV apps (no push client) ; VAPID generated: cmd/evey/wire_helpers.go
password recovery: admin.serverSettings.passwordRecovery ; gated on smtpConfigured (cmd/evey/wire.go) ; auth.login.forgotPassword
admin-only types include the subtitle cache: internal/domain/notification/model.go - IsAdminOnly (TypeCacheFull) ; i18n notifications push.cache_full
labels: i18n admin.serverConfig.port, smtpUsername, smtpPassword, smtpEnvHint ; auth.settings pushNotifications, pushNeedsHttps ; levelAll, levelRelevant, levelImportant, levelDisabled
no invitation e-mail: internal/adapters/driven/email (no invite sender)
-->
