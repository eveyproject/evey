---
source_sha: f8b9121424daaf442c366f75a6973b21de77d280d45f2cb7e7dadab38b7328be
reviewed: false
---
# Webhooks und API-Schlüssel

Zwei Wege, Evey mit anderen Werkzeugen zu verbinden, ohne ein Plugin zu schreiben:
Webhooks senden die Ereignisse von Evey an eine URL von Ihnen, API-Schlüssel
lassen Ihre Skripte Evey aufrufen.

## Webhooks

**Admin → Webhooks → Webhook erstellen**: ein Name, eine URL, die zu sendenden
Ereignisse und ein optionales Geheimnis. **Testen** sendet einen Ping und meldet
die Antwort.

<figure class="evey-shot" markdown="span">
  ![Ein Webhook unter Admin → Webhooks](images/admin-webhooks.webp){ loading=lazy }
  <figcaption>Ein Webhook, der Beginn und Ende jeder Wiedergabe sendet (Oberfläche auf Englisch).</figcaption>
</figure>

**Ereignisse.** Ein Webhook abonniert alle Ereignisse oder eine gewählte Liste.
Es sind die Ereignisse des Servers selbst, zum Beispiel:

| Bereich | Ereignisse |
| --- | --- |
| Bibliothek | `library.scan.started`, `library.scan.completed`, `media.item.added`, `media.item.updated`, `media.item.removed` |
| Wiedergabe | `playback.started`, `playback.stopped`, `media.marked.watched`, `transcode.started`, `transcode.failed` |
| Konten | `user.created`, `user.login.succeeded`, `user.login.failed`, `user.profile.created` |
| Anfragen | `media.request.created`, `media.request.answered` |
| Server | `backup.created`, `backup.failed`, `plugin.installed`, `plugin.error`, `server.config.changed` |

**Zustellung.** Jedes Ereignis ist ein JSON-`POST`. Zwei Header kommen mit:

- `X-Evey-Delivery`: eine Kennung, die über die Wiederholungen einer Zustellung
  gleich bleibt, sodass Ihr Empfänger ein Duplikat ignorieren kann;
- `X-Evey-Signature`: `sha256=`, gefolgt vom HMAC-SHA256 des Inhalts, berechnet
  mit dem Geheimnis des Webhooks. Prüfen Sie ihn mit demselben Geheimnis, um zu
  wissen, dass die Anfrage von Ihrem Server kommt. Ohne Geheimnis beweist die
  Signatur nichts.

Eine fehlgeschlagene Zustellung wird nach 1, 5 und 30 Sekunden wiederholt. Jeder
Versuch wird im Zustellungsprotokoll des Webhooks festgehalten.

**Nur öffentliche Adressen.** Eine Webhook-URL muss `http` oder `https` sein und
auf eine öffentliche Adresse zeigen. Private und lokale Adressen (`192.168.x.x`,
`10.x.x.x`, `localhost`, ein Containername, der zu einer solchen aufgelöst wird)
werden abgelehnt, beim Speichern des Webhooks und erneut, wenn der Server sich
verbindet. Das schützt den Server davor, genutzt zu werden, um Ihr internes
Netzwerk zu erreichen; ein Empfänger in Ihrem lokalen Netzwerk braucht eine
öffentliche Adresse oder ein Relais.

## API-Schlüssel

Ein API-Schlüssel lässt ein Skript die Server-API ohne Passwort aufrufen.
Schlüssel werden unter **Admin → API-Schlüssel** von einem Administrator
angelegt und gehören diesem Administrator.

- **Der Schlüssel wird einmal gezeigt**, bei der Erstellung. Speichern Sie ihn
  dann; er kann nicht erneut angezeigt, nur widerrufen werden.
- **Er handelt als sein Besitzer**, mit Administratorrechten, im Rahmen seiner
  Berechtigungen.
- **Berechtigungen** (Scopes) begrenzen, was der Schlüssel tun kann:

| Berechtigung | Erlaubt |
| --- | --- |
| `media:read` | Metadaten der Medien lesen |
| `media:write` | Medien ändern und Scans starten |
| `playback:read` | Wiedergabezustand und Verlauf lesen |
| `playback:write` | Wiedergabe und Fortschritt steuern |
| `users:read` | Benutzer lesen |
| `users:write` | Benutzer ändern |
| `admin:read` | Aktivität und Statistiken der Verwaltung lesen |
| `admin:write` | Einstellungen der Verwaltung ändern |

**Ein ohne Berechtigung erstellter Schlüssel hat vollen Zugriff**, wie sein Besitzer:
Das ist das alte Verhalten, für bestehende Schlüssel beibehalten. Wählen Sie die
Berechtigungen, die ein Skript braucht.

Der Schlüssel gehört in den Header `Authorization`, mit oder ohne `Bearer`:

```bash
curl -H "Authorization: Bearer evey_..." https://media.example.com/api/v1/...
```

Ein Schlüssel funktioniert sofort nicht mehr, wenn er widerrufen oder sein
Besitzer deaktiviert wird. Die Endpunkte selbst stehen in der
[Server-API](api/README.md).
