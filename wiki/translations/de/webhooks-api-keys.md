---
source_sha: 44e8e90d77d0eb2abdd259fe7a12d693a5f6af4856125bd2b2da2194a7c6ef42
reviewed: true
---
# Webhooks und API-Schlüssel

Zwei Wege, Evey mit anderen Werkzeugen zu verbinden, ohne ein Plugin zu
schreiben: Ein Webhook schickt die Ereignisse des Servers an eine Adresse
Ihrer Wahl. Ein API-Schlüssel lässt ein Skript den Server in Ihrem Namen
aufrufen. Beides legt ein Administrator im Verwaltungsbereich an. Diese Seite
richtet sich an die Person, die sie einrichtet, und an die Person, die den
Empfänger oder das Skript schreibt.

## Ereignisse an eine URL schicken

**Verwaltung → Webhooks → Webhook erstellen**: ein **Name**, eine **URL**,
die **Ereignisse**, die gesendet werden, und optional ein **Geheimnis** zum
Signieren. **Testen** schickt einen Ping und zeigt die Antwort.

<figure class="evey-shot" markdown="span">
  ![Ein Webhook unter Verwaltung → Webhooks](images/admin-webhooks.webp){ loading=lazy }
  <figcaption>Ein Webhook, der Beginn und Ende jeder Wiedergabe sendet (Oberfläche auf Englisch).</figcaption>
</figure>

**Ereignisse.** Ein Webhook abonniert **Alle Ereignisse** oder eine
ausgewählte Liste. Das Formular bietet jedes Ereignis an, das der Server
veröffentlicht, unter anderem:

| Bereich | Ereignisse |
| --- | --- |
| Bibliothek | `library.scan.started`, `library.scan.completed`, `library.prune.suspended` (ein Stammordner hat auf einmal zu viele Dateien verloren, Löschungen werden gehalten, bis ein Administrator bestätigt), `media.item.added`, `media.item.updated`, `media.item.removed`, `metadata.refreshed` |
| Wiedergabe | `playback.started`, `playback.stopped`, `media.marked.watched`, `transcode.started`, `transcode.failed` |
| Konten | `user.created`, `user.login.succeeded`, `user.login.failed`, `user.profile.created`, `user.apikey.created` |
| Anfragen | `media.request.created`, `media.request.answered` |
| Server | `backup.created`, `backup.failed`, `plugin.installed`, `plugin.error`, `server.config.changed`, `transcoding.derived_cache_full` (der freie Platz des Untertitelspeichers liegt unter der vorgesehenen Reserve) |

Ein Plugin, das `EmitEvent` aufruft, veröffentlicht `plugin.event.<type>`.
Ein Webhook auf **Alle Ereignisse** empfängt auch das.

**Zustellung.** Jedes Ereignis ist ein JSON-`POST` mit diesen Headern:

- `X-Evey-Delivery`: eine Kennung, die über alle Wiederholungen einer
  Zustellung gleich bleibt. So kann Ihr Empfänger ein Duplikat ignorieren.
- `X-Evey-Signature`: `sha256=`, gefolgt vom HMAC-SHA256 des Inhalts,
  berechnet mit dem Secret des Webhooks. Prüfen Sie ihn mit demselben Secret,
  dann wissen Sie, dass die Anfrage von Ihrem Server kommt. Ohne Secret
  beweist die Signatur nichts. Sie deckt nur den Inhalt ab und verhindert daher
  nicht, dass eine abgefangene Anfrage erneut gesendet wird: Nutzen Sie lieber
  die zwei folgenden Header.
- `X-Evey-Timestamp`: der Zeitpunkt des Versuchs in Unix-Sekunden. Jede
  Wiederholung trägt einen neuen Wert.
- `X-Evey-Signature-V2`: `sha256=`, gefolgt vom HMAC-SHA256, berechnet mit dem
  Secret des Webhooks, über den Text `<timestamp>.<delivery id>.<body>` (der
  Wert von `X-Evey-Timestamp`, ein Punkt, der Wert von `X-Evey-Delivery`, ein
  Punkt, dann der rohe Inhalt).

Um eine Zustellung zu prüfen, berechnen Sie die V2-Signatur aus dem rohen
Inhalt neu und vergleichen sie in konstanter Zeit. Lehnen Sie die Anfrage dann
ab, wenn der Zeitstempel mehr als 5 Minuten von Ihrer Uhr abweicht (in beide
Richtungen). Merkt sich Ihr Empfänger zusätzlich die in diesem Zeitfenster
gesehenen Zustellungskennungen, lässt sich eine abgefangene Anfrage nicht
erneut einspielen.

Jeder Versuch wartet bis zu 30 Sekunden auf eine Antwort. Eine
fehlgeschlagene Zustellung wird nach 1, 5 und 30 Sekunden wiederholt. Jeder
Versuch wird festgehalten. Den Verlauf lesen Sie über die API
(`ListWebhookDeliveries` in der [Plugin-API](api/plugins/README.md)), nicht im
Verwaltungsbereich.

!!! warning "Nur öffentliche Adressen"
    Eine Webhook-URL muss `http` oder `https` sein und auf eine öffentliche
    Adresse zeigen. `localhost`, private und link-lokale Adressen
    (`192.168.x.x`, `10.x.x.x`, `172.16.x.x`...) und ein Name, der zu einer
    solchen Adresse aufgelöst wird, werden abgelehnt: beim Speichern des
    Webhooks und noch einmal, wenn der Server sich verbindet. So kann
    niemand den Server benutzen, um Ihr internes Netz zu erreichen. Ein
    Empfänger in Ihrem lokalen Netz braucht eine öffentliche Adresse oder
    einen Vermittler.

## Den Server aus einem Skript aufrufen

Mit einem API-Schlüssel ruft ein Skript die Server-API ohne Passwort auf.
Schlüssel legt ein Administrator unter **Verwaltung → API-Schlüssel → Neuer
API-Schlüssel** an. Sie gehören diesem Administrator.

- **Der Schlüssel wird einmal angezeigt**, beim Erstellen. Speichern Sie ihn
  dann. Er lässt sich nicht noch einmal anzeigen, nur mit **Widerrufen**
  sperren.
- **Er handelt im Namen seines Besitzers**, mit Administratorrechten, im Rahmen
  seiner Berechtigungen.
- **Berechtigungen** (Scopes) begrenzen, was der Schlüssel tun darf:

| Bereich | Erlaubt |
| --- | --- |
| `media:read` | Metadaten der Medien lesen |
| `media:write` | Medien ändern und Scans starten |
| `playback:read` | Wiedergabestand und Verlauf lesen |
| `playback:write` | Wiedergabe und Fortschritt steuern |
| `users:read` | Benutzer lesen |
| `users:write` | Benutzer ändern |
| `admin:read` | Aktivität und Statistiken der Verwaltung lesen |
| `admin:write` | Einstellungen der Verwaltung ändern |

Ein Schlüssel ohne jeden Bereich (**Alle (veraltet)**) hat vollen Zugriff auf
die Methoden außerhalb der Verwaltung, wie sein Besitzer. Verwaltungsmethoden
verlangen aber immer ihren Bereich: Ein Legacy-Schlüssel erreicht sie nicht.
Wählen Sie die Bereiche, die ein Skript braucht.

Der Schlüssel kommt in den Header `Authorization`, mit oder ohne `Bearer`:

```bash
curl -H "Authorization: Bearer evey_..." https://media.example.com/api/v1/...
```

Ein Schlüssel funktioniert sofort nicht mehr, wenn er widerrufen oder sein
Besitzer deaktiviert wird. Die Endpunkte selbst stehen in der
[Server-API](api/README.md), zusammen mit den anderen Wegen, sich
auszuweisen.

<!-- sources
form labels: i18n admin.webhooks.form.* (name, url, secret, events, allEvents), admin.webhooks.createWebhook, common.test, admin.webhooks.testPingSent
event list: frontend/packages/client/src/lib/webhookEvents.ts - WEBHOOK_EVENT_TYPES ; EventName() in internal/domain ; plugin events: internal/adapters/driving/pluginhost/host_server.go - pluginEmittedEvent.EventName
delivery, headers, timeout, retries, signature: internal/app/webhook/service.go - handleAny, attemptDeliver, sign, defaultRetryDelays, deliveryTimeout ; internal/domain/webhook/model.go - MatchesEvent ; history: proto/evey/v1/webhook.proto - ListWebhookDeliveries
public addresses only: internal/pkg/urlvalidator/validator.go - ValidateWebhookURL, isPrivateIP ; internal/pkg/ssrf/dialer.go
api keys: internal/adapters/driving/grpcapi/interceptors/auth.go - authenticateAPIKey (Bearer optional, evey_ prefix), authorize (admin methods always enforce the scope) ; internal/pkg/auth/rbac.go (scopes) ; rbac.go - CreateAPIKey RoleAdmin ; i18n auth.apiKeys.* (newKeyTitle, scopesAll, saveWarning, revoke)
-->
