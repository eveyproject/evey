# Webhooks and API keys

Two ways to connect Evey to other tools without writing a plugin: webhooks
send Evey's events to a URL of yours, API keys let your scripts call Evey.

## Webhooks

**Admin → Webhooks → Create Webhook**: a name, a URL, the events to send, and
an optional secret. **Test** sends a ping and reports the answer.

<figure class="evey-shot" markdown="span">
  ![A webhook in Admin → Webhooks](images/admin-webhooks.webp){ loading=lazy }
  <figcaption>A webhook sending the start and the end of each playback.</figcaption>
</figure>

**Events.** A webhook subscribes to all events or to a chosen list. The events
are those of the server itself, for example:

| Area | Events |
| --- | --- |
| Library | `library.scan.started`, `library.scan.completed`, `media.item.added`, `media.item.updated`, `media.item.removed` |
| Playback | `playback.started`, `playback.stopped`, `media.marked.watched`, `transcode.started`, `transcode.failed` |
| Accounts | `user.created`, `user.login.succeeded`, `user.login.failed`, `user.profile.created` |
| Requests | `media.request.created`, `media.request.answered` |
| Server | `backup.created`, `backup.failed`, `plugin.installed`, `plugin.error`, `server.config.changed` |

**Delivery.** Each event is a JSON `POST`. Two headers come with it:

- `X-Evey-Delivery`: an identifier that stays the same across the retries of
  one delivery, so your receiver can ignore a duplicate;
- `X-Evey-Signature`: `sha256=` followed by the HMAC-SHA256 of the body,
  computed with the webhook's secret. Verify it with the same secret to know
  the request comes from your server. Without a secret, the signature proves
  nothing.

A delivery that fails is retried after 1, 5 and 30 seconds. Each attempt is
recorded in the webhook's delivery log.

**Only public addresses.** A webhook URL must be `http` or `https` and point
to a public address. Private and local addresses (`192.168.x.x`, `10.x.x.x`,
`localhost`, a container name that resolves to one) are refused, when the
webhook is saved and again when the server connects. This protects the server
against being used to reach your internal network; a receiver on your local
network needs a public address or a relay.

## API keys

An API key lets a script call the server API without a password. Keys are
created in **Admin → API Keys**, by an administrator, and belong to that
administrator.

- **The key is shown once**, when it is created. Store it then; it cannot be
  displayed again, only revoked.
- **It acts as its owner**, with administrator rights, within its scopes.
- **Scopes** restrict what the key can do:

| Scope | Allows |
| --- | --- |
| `media:read` | Reading media metadata |
| `media:write` | Modifying media and starting scans |
| `playback:read` | Reading playback state and history |
| `playback:write` | Controlling playback and progress |
| `users:read` | Reading users |
| `users:write` | Modifying users |
| `admin:read` | Reading administration activity and statistics |
| `admin:write` | Modifying administration settings |

**A key created without any scope has full access**, like its owner: this is
the legacy behaviour, kept for existing keys. Select the scopes a script needs.

The key goes in the `Authorization` header, with or without `Bearer`:

```bash
curl -H "Authorization: Bearer evey_..." https://media.example.com/api/v1/...
```

A key stops working at once when it is revoked, or when its owner is disabled.
The endpoints themselves are listed in the [Server API](api/README.md).

<!-- sources
webhook delivery, headers, retries, signature: internal/app/webhook/service.go - attemptDeliver, sign, defaultRetryDelays
all events: internal/app/webhook/service.go - handleAny ; internal/domain/webhook/model.go - MatchesEvent ; EventName() in internal/domain
public addresses only: internal/pkg/urlvalidator/validator.go - ValidateWebhookURL ; internal/pkg/ssrf/dialer.go ; internal/adapters/driven/webhooksender/sender.go
api keys: internal/adapters/driving/grpcapi/interceptors/auth.go - authenticateAPIKey ; internal/pkg/auth/rbac.go (scopes) ; i18n auth.apiKeys.* ; frontend/apps/web/src/routes/_auth/admin/api-keys.lazy.tsx (only UI)
-->
