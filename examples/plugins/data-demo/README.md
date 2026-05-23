# data-demo — plugin data store + grantable capabilities

A minimal example of the storage and capability features a plugin can use.

## Per-plugin data store (no permission)

`main.js` persists a `boot_count` across restarts via the HostService data RPCs:

```js
const r = await getData("boot_count");            // GetData → { value, found }
const next = (r?.found ? parseInt(r.value, 10) : 0) + 1;
await setData("boot_count", String(next));        // SetData(key, value)
// also available: DeleteData(key), ListDataKeys()
```

The plugin id is taken from the authenticated host socket, so a plugin can only
ever touch **its own** namespace — never another plugin's. Rows cascade-delete
when the plugin is uninstalled, so nothing is left behind.

## Grantable capabilities (admin consents at install)

This manifest requests two capabilities, both shown in the install consent diff:

- **`public`** — the iframe may read non-sensitive media (title/poster/year via
  `GetPublicMediaItem` / `ListPublicMediaItems`), never file paths.
- **`prefs`** — the iframe may read/write the **current user's** preferences in
  this plugin's namespace (`GetPref` / `SetPref` / `DeletePref` / `ListPrefs`).

## What you can never request

`admin.*`, `auth.*` (login/logout/session), `system.*`, and generic
`user.write` / `library.write` are hard-locked: stripped from any token and
rejected at the API boundary even if the manifest declares them. A scoped plugin
token is deny-by-default on the server API — it only reaches what the capabilities
above unlock.

## Frontend slot

The `admin.nav` slot adds one entry to the admin panel opening this plugin's own
sandboxed page (`dist/admin.js`) — it does not inject into existing admin screens.

## Build & install

```sh
make plugin-zips                 # zips every example into a .zip
# then upload examples/plugins/data-demo.zip via /admin/plugins (or the API)
```
