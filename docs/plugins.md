# Plugins

This page is for the administrator: how to install a plugin or a theme, what
you accept when you do, and what the server lets it touch on your host.
Everything happens in **Admin → Plugins**; the pages for plugin authors are
listed [at the end](#writing-a-plugin).

A plugin adds to Evey what it does not do by itself: a section on the home
page, an overlay in the player, a metadata source, a theme. Each plugin is a
separate process that talks to the server through a local socket, confined by
a sandbox and limited to the permissions its manifest declares. Its interface,
when it has one, runs in an isolated frame of the page.

<div class="evey-shot-pair" markdown>

<figure class="evey-shot" markdown="span">
  ![The home page without plugins](images/home-no-plugin.webp){ loading=lazy }
  <figcaption>Without plugins.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![The home page with the weather plugin](images/home.webp){ loading=lazy }
  <figcaption>With the example weather plugin: a section at the top, drawn in its own frame.</figcaption>
</figure>

</div>

## Install a plugin

1. In **Admin → Plugins**, open **Install Plugin** and choose the **Plugin
   file (.zip)**. If the author ships a signature, add it in **Signature file
   (.zip.sig, optional)**. The **From URL** tab installs from a link instead;
   append `#sha256:<hash>` to the URL to have the download checked.
2. **Preview & Install** analyses the package without installing it. The
   preview shows the **Permissions** and **Capabilities** the plugin asks for,
   the result of the **Static Analysis** and, when a signature was given, the
   **Signature** verdict with the publisher. The slots and the hosts are shown
   at update and on the **Security Dashboard**.
3. Read that list as the contract: those are the only things the plugin will
   be able to do. Confirm, and the plugin starts.

<figure class="evey-shot" markdown="span">
  ![Admin → Plugins on the demo server](images/admin-plugins.webp){ loading=lazy }
  <figcaption><strong>Admin → Plugins</strong>: the runtimes available, the resource limits, and one card per plugin.</figcaption>
</figure>

A package whose analysis finds a critical pattern is refused, whatever you
answer. Warnings are shown and left to you. The plugin's language must be
among the **Available Runtimes** of the server: the default image ships
Python, Node.js, PHP, Ruby and Lua; the `-nodejs` image keeps Python and
Node.js and the `-lean` image keeps Python only.

!!! tip "Signed plugins"
    A plugin author can sign the ZIP with [cosign](https://docs.sigstore.dev/cosign/installation/);
    the author's public key travels inside the package. Evey checks the
    signature on its own (no `cosign` binary) and tells you **Signed by
    <publisher>** when the key is in your **Trusted publishers** list,
    **Signed, unknown key** with its fingerprint otherwise (one click trusts
    it), **Unsigned** or **Invalid**. An update that changes key, or drops the
    signature, waits for your explicit acceptance. The policy decides the
    rest:

    ```sh
    EVEY_PLUGINS_COSIGN_POLICY=warn      # off | warn (default) | enforce
    ```

    With `warn`, the verdict is shown and nothing is blocked; with `enforce`,
    only trusted publishers install. Authors: see
    [Sign your plugin](plugins/dev-setup.md#7-sign-your-plugin).

## Update a plugin

On the plugin's card, **Update from source…** takes the new ZIP (a path or a
URL) and shows the same preview. An update must carry a higher version than
the installed one. When it asks for new permissions, capabilities, slots or
hosts, the preview says **This update requests new permissions** and lists the
difference: nothing is applied until you choose **Approve and install**. The
package analysed is the package installed: its hash is checked again when the
update is applied.

## Manage the installed plugins

Each card shows the plugin's state (**Enabled** or **Disabled**) and offers:

| Action | What it does |
| --- | --- |
| **Enabled** / **Disabled** | Starts or stops the plugin's process. A disabled plugin keeps its files and its configuration. |
| **Config** | The settings the plugin declares (an API key, a city, a unit...). Secret fields are shown masked and are never sent back to the browser. |
| **Logs** | The last lines the plugin wrote through the host. |
| **Metadata** | On a metadata plugin only: the fields it replaces and the fetch of the existing library ([below](#metadata-plugins)). |
| **Uninstall** | Removes the process, the files and the plugin's stored data. |

**Resource limits**, at the top of the page, caps every plugin process. The
values in the panel are the defaults; the environment variable sets them when
the panel has never been saved.

| Field | Default | Environment variable |
| --- | --- | --- |
| **Max memory (MiB)** | 256 | `EVEY_PLUGINS_MEMORY_MB` |
| **Max processes** | 64 | `EVEY_PLUGINS_PIDS_MAX` |
| **Max disk (MiB)** | 100 | `EVEY_PLUGINS_DISK_MAX_MB` |
| **CPU nice (0-19)** | 10 | `EVEY_PLUGINS_CPU_NICE` |
| **Sandbox mode** | `auto` | `EVEY_PLUGINS_SANDBOX_MODE` |

Memory, processes and disk apply at the next sweep of the monitor, which stops
a plugin that exceeds them; the CPU priority and the sandbox mode apply to
plugins started afterwards. `0` restores the default for memory and processes; only the disk limit treats
`0` as off.

The **Security Dashboard** keeps, per plugin, the findings of the analysis, the
signature result and the last error. Its **Host sandbox** line shows the
sandbox mode actually in force on this machine: if it reads `basic`, the host
offers no per-path filesystem isolation, and
[Plugin sandbox setup](plugins/sandbox-deployment.md) explains what to change.

Plugins are enabled by default. `EVEY_PLUGINS_ENABLED=false` turns the whole
subsystem off.

## Metadata plugins

A metadata plugin (capability `metadata.provider`) gives its own text for your
movies and series, from a source other than TMDB. It does not replace TMDB:
TMDB still identifies each title and gives its cast, its artwork, its ratings,
its collections and its translations. The plugin's text is kept beside TMDB's,
and shown in its place field by field.

What a viewer reads, from the weakest to the strongest:

1. TMDB's text, in the viewer's language when TMDB has it;
2. the plugin's text in that language, for the fields the plugin is allowed to
   replace;
3. your own edit of the title (**Edit metadata** on its page), which always
   wins.

**Metadata** on the plugin's card chooses the fields it replaces among
**Title**, **Overview** and **Tagline**. Until you choose, a plugin replaces
the overview and the tagline, never the title. A change applies at once to
what the plugin already gave; nothing is fetched again.

The plugin is asked about every movie and series as it is matched or
refreshed. **Fetch for the whole library**, on the same panel, asks it about
the titles already there, one after the other, and shows how far it has gone
and, at the end, how many it found and how many got no answer. A refresh of
the title from TMDB never overwrites the plugin's text; disabling the plugin
hides it, uninstalling it deletes it.

Seasons and episodes keep TMDB's text, and search still matches TMDB's titles.

## What a plugin can do on your host

Whatever its manifest says, a plugin:

- sees its own directory, not the server's data nor the media files;
- has no direct network access: a plugin with the `network` permission reaches
  only the hosts it declared, through the server, with `GET` and `HEAD`;
- cannot start child processes unless it declares `subprocess`;
- runs under the resource limits above and, when the host allows it, under a
  separate user.

Its interface is confined to the slots the manifest declares: a section or the
hero area of the home page, a sidebar panel in a library or on a detail page,
an overlay of the player, the appearance settings, the bottom of the
navigation sidebar, or one entry in the admin menu that opens the plugin's own
page. A plugin can never draw into the sign-in, setup, security or existing
admin screens. In its frame, it has no access to the page, your cookies or
your session: what it reads from the server goes through a short-lived token
limited to the two permissions it may ask for (`public` for a safe projection
of media titles and posters, `prefs` for the current user's preferences in its
own namespace).

The permissions a plugin can ask for are listed in
[Permissions and slots](plugins/security-model.md). How the sandbox works,
and what it still cannot prevent, is in
[Plugin sandbox and threat model](SECURITY.md).

!!! warning "The server checks where a plugin talks, not what it says"
    Once a host is on a plugin's list, the server cannot inspect what the
    plugin sends to it. Review a third-party plugin before installing it, as
    you would any software, and treat its host list as the list of parties
    that may receive your library's titles.

## Themes

A theme is a package of `kind: theme` with no process: a stylesheet, and
optionally a script confined to a hidden frame. It is installed from the same
**Install Plugin** dialog; the stylesheet is sanitized at install. Each user
then picks it under **Settings → Appearance → Theme**. What a theme may
contain is described in [Themes](themes.md).

## Writing a plugin

| Page | Contents |
| --- | --- |
| [Writing a plugin](plugins/dev-setup.md) | A first plugin step by step, the install loop, signing, distribution. Start here. |
| [Plugin API reference](plugins/api-reference.md) | The manifest, the backend calls and data store, the `evey.*` bridge of the interface. |
| [Permissions and slots](plugins/security-model.md) | What a manifest may declare, what is checked at install, the external fetch rules, the slots. |
| [Themes](themes.md) | Restyling the interface with CSS and a sandboxed script. |
| [Webhooks and API keys](webhooks-api-keys.md) | Integrating Evey with other tools without writing a plugin. |

<!-- sources
install dialog, from URL with #sha256: i18n admin.plugins.installPlugin, pluginFileZip, sigFileOptional, fromUrl, installFromUrlHint, previewInstall, staticAnalysis ; internal/app/plugin/install.go - downloadIfURL
critical findings refuse, warnings: internal/app/plugin/install.go - validateStagedPlugin, checkEscalatedWarnings ; i18n admin.plugins.blockedByAnalysis, installAnyway
runtimes: internal/adapters/driven/pluginrunner/runtime_availability.go - runtimeBinaries ; i18n admin.plugins.availableRuntimes
cosign policy: internal/config/config.go - CosignPolicy, CosignPublicKey ; internal/adapters/driven/pluginrunner/cosign.go
update, version higher, escalation, hash re-checked: internal/app/plugin/install.go - resolveExistingPlugin ; internal/app/plugin/update.go - ApplyUpdate ; internal/app/plugin/permissions.go - DiffPermissions ; i18n admin.plugin.update, admin.plugins.permissionEscalation.*
card actions: i18n admin.plugin.enabled, disabled, config, logs, uninstall, metadata.title ;
metadata plugins: internal/app/metadata/enricher.go - enrichPluginTexts, takesPluginTexts ; internal/app/library/query_service.go - GetMediaItemTranslations, pluginTextsFor ; internal/domain/library/plugin_text.go - OverlayPluginTexts ; internal/adapters/driving/grpcapi/media_item_overlay.go - overlayItems (admin edits after) ; internal/domain/plugin/metadata_fields.go - DefaultMetadataFields ; internal/adapters/driven/postgres/plugin_text_repo.go - DisplayedForLocale (enabled plugins only) ; migration 000182 (ON DELETE CASCADE) ; internal/app/metadata/plugin_text_refresh.go ; i18n admin.plugin.metadata.* ; search: internal/app/library/search_reindex.go (TMDB translations only)
 secrets masked: internal/domain/plugin/model.go - IsSecretConfigField ; internal/adapters/driving/grpcapi/admin_plugin.go
resource limits defaults and env: internal/config/config.go - Plugins (MemoryMB, PIDsMax, DiskMaxMB, CPUNice, SandboxMode) ; i18n admin.plugins.resourceLimits.*
security dashboard, host sandbox: i18n admin.plugins.security.* ; internal/app/plugin/security_dashboard_service.go
plugins enabled flag: internal/config/config.go - PluginsEnabled
fetch broker GET/HEAD: internal/adapters/driving/pluginhost/fetch.go - Fetch
subprocess: docs/plugins/security-model.md (Permissions catalog)
slots: internal/domain/plugin/slots.go - AllowedSlots
scoped token, public/prefs: docs/adr/0012-plugin-token-deny-by-default.md ; internal/pkg/auth/plugin_jwt.go - AllowedPluginScopes
theme selection: frontend/apps/web/src/features/user/ThemeSettings.tsx ; i18n common.appearance, common.theme
-->
