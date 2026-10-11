# Apps and devices

Every app talks to your own server. You give it the server's address once,
sign in, and choose a profile; resume points, watch history and favourites
are the server's, so they follow you from one device to the other. This page
is for the person installing the apps on the household's phones and TVs: the
address to type is the first section.

| App | Where it comes from | Signing in |
| --- | --- | --- |
| **Web** | Served by Evey itself, at the server's address. Nothing to install. | Username and password, or single sign-on if configured. |
| **Android** (phone, tablet) | The Android app. | Server address, then username and password. |
| **Android TV** | The Android TV app. | Server address, then **Quick Connect** (below) or username and password. |
| **Chromecast** | No app: cast from the web app in Chrome or from the Android app. | Uses the session of the device that casts. |

The administration panel is part of the web app, for administrator accounts.

## The server address to enter

The Android and Android TV apps open on a screen titled **Connect to Server**
(**Connect to your server** on the TV):
one field (**Server URL**, placeholder `https://your-server.com`), a
**Connect** button and a **Scan local network** link. As long as no server is
saved, a line above the field explains what is asked: **Evey runs on a server
someone set up for you. Enter its web address - the same one you open in a
browser.**, followed by **Where do I find my server's address?**, which leads
to this page (in French when the phone or TV is in French, in English
otherwise). The phone opens it in the browser; the TV writes its address on
screen, to open on another device.

Type the **same address you open in a browser on the same network**, with the
scheme and the port:

```text
http://192.168.1.20:9091
```

<div class="evey-shot-pair" markdown>

<figure class="evey-shot" markdown="span">
  ![The Android app asking for the server address](../images/app-android-connect.webp){ loading=lazy data-gallery="connect" }
  <figcaption>The Android app: the address of the server at home, with http:// and the port.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![The Android TV app asking for the server address](../images/app-tv-connect.webp){ loading=lazy data-gallery="connect" }
  <figcaption>The Android TV app: the same address, typed with the remote or found by the scan.</figcaption>
</figure>

</div>

| Where the device is | What to type |
| --- | --- |
| At home, same Wi-Fi as the server | `http://` + the IP of the machine running Evey + `:9091`, for example `http://192.168.1.20:9091`. Its name works too (`http://nas:9091`) when your router resolves names. |
| Away from home | The public HTTPS address set in **Admin → Remote Access → External URL**, for example `https://evey.example.org`, without the port (the reverse proxy listens on 443). See [Remote access and HTTPS](../administration/remote-access.md). |

Where the pieces come from:

- **The IP** is the one of the computer or NAS running Docker on your home
  network: in your router's list of devices, or on the machine itself (`ip a`
  on Linux, `ipconfig` on Windows, `ifconfig` on macOS). It is never the
  phone's own IP.
- **The port** is the left-hand number of the `ports:` line of your compose
  file (`"9091:9091"`, see [Install](../installation.md#the-compose-file)).
  With the file from the installation page it is `9091`.
- **The scheme** is `http://` at home. Plain HTTP on the home network is the
  normal setup; `https://` is for the public address only.

Nothing comes after the port: no `/api`, no path. A trailing `/` is removed.
Once you press **Connect**, the app contacts the server, keeps the address,
and moves on to the sign-in screen.

### Scan local network: what it finds and what it does not

**Scan local network** tries `http://` on every address of four common home
subnets (`192.168.1.x`, `192.168.0.x`, `10.0.0.x` and `172.16.0.x`, hosts 1
to 254) on ports `9091`, `8096` and `8920`, fifty at a time with a 2-second
timeout each, so a full sweep takes up to a couple of minutes. Every server
found is listed with its name (the **Server name** chosen at setup), its
address and its version; tapping one only fills the field, you still press
**Connect**. When nothing answers, the screen says **No server found on this
network - enter the address below**.

The scan does not find a server:

- on another subnet (`192.168.2.x`, `10.1.x.x`...) or on another port;
- when the phone is on mobile data, on a guest Wi-Fi, or on a different
  network from the server;
- reachable only over HTTPS.

Type the address in those cases. The apps do not use mDNS or Bonjour and do
not read the phone's own network range: the four subnets above are fixed.

### When the connection fails

Every failure is a dialog titled **Connection failed**. What the message
means and what to do:

| Message (Android) | Cause | What to do |
| --- | --- | --- |
| **Start the address with http:// or https://.** | No scheme. | Type `http://` before the IP. |
| **This connection is not encrypted** | `http://` towards an address outside the home network. The password and the video would cross the internet in the clear. | **Cancel** and use the `https://` public address, or **Connect anyway** if you know what you are doing. |
| **No response from ... within 8s.** | Nothing answers at that address. | Check the server is running (`docker compose ps`), the IP and the port, and that the phone is on the same Wi-Fi. |
| **Couldn't reach ...** | Host or port wrong, `http`/`https` mismatch, or a different network. | Same checks; try `http://` instead of `https://` at home, and the other way round for a public address. |
| **TLS/certificate problem reaching ...** | A self-signed certificate. | Use `http://` at home, or a certificate issued by a public authority for the public address. |
| **... answered "HTTP 404 ..." instead of a healthy Evey server.** | Something answers, but not Evey: a path after the port, or another service on that port. | Remove everything after the port and check the port number. |
| **... answered, but it is not an Evey server.** | A captive portal, the ISP's page or another service answered with a page of its own. | Check the address; on a public Wi-Fi, sign in to the portal first. |
| **Remote access disabled** | The server is reached from outside the home while its remote access is off. | An administrator turns it on in **Admin → Remote Access**: [Remote access and HTTPS](../administration/remote-access.md). |

On Android TV the dialogs are shorter: **Server returned 404** for an answer
that is not Evey's, and the raw network error when nothing answers; the causes
and the checks are the same. The TV does not warn about `http://` towards a
public address, and the scheme is required there too: a bare
`192.168.1.20:9091` fails.

When an app that already knows a server cannot reach it at launch, it comes
back to this screen with the saved server listed; the TV adds **Your server
did not answer. It may be asleep or off the network.** Choose the server
again once it is up.

### Changing or adding a server later

Both apps keep a list of servers. The connect screen shows it above the field,
under **Your servers**, with the one in use marked **Current** (Android) or
**Active** (Android TV), and **Add another server** underneath. Tapping a
saved server reconnects to it without typing the address again. Each server
keeps its own sign-in.

- **Android.** Each saved server on the connect screen has a delete button, so
  an address that no longer works can be removed before signing in; a
  confirmation follows, and its sign-in is cleared. The sign-in screen has
  **Change server**. Once signed in, **Settings → Servers** opens **Manage
  Servers**: tap a server to switch to it, **Rename** it, or **Delete** it.
  In both places the only saved server cannot be deleted.
- **Android TV.** **Settings → Change server** signs the television out and
  returns to the connect screen; the sign-in screen has the same button. On
  the connect screen, **Manage Servers** renames or deletes a saved server.

## Quick Connect: signing in a TV without typing a password

On the TV's sign-in screen, choose **Quick Connect** (marked **Recommended for
TV**). The screen shows a 6-character code, a QR code and how long the code
stays valid. Approve it from a device where you are already signed in:

- **Web**: **Settings → Quick Connect**, type the code in **Device Code** and
  press **Authorize Device**.
- **Android app**: **Settings → Quick Connect**, then **Scan QR Code** with
  the camera, or type the code and press **Authorize Device**.
- **Phone camera**: scanning the QR code opens the server's Quick Connect page
  in the browser. Signed in there, the code is already filled in; otherwise
  sign in first and type it.

The TV notices the approval within seconds and is signed in to the same
account. The Android app's sign-in screen offers Quick Connect in the same
way, for a phone you would rather not type a password on.

## What each app adds

- **Web.** Keyboard shortcuts in the player (`?` lists them), browser push
  notifications (see [Notifications](../notifications.md)), and the
  administration panel.
- **Android.**
  - **Downloads** for offline viewing. When the administrator has turned it
    on (`downloads.quality_choice`, see
    [Playback delivery and downloads](../environment/playback-delivery.md)),
    the app offers a smaller copy prepared by the server for a film too large
    for the phone. A download stays
    playable offline for 30 days without contacting the server, and only for
    profiles allowed to see it: a kids profile cannot open a film downloaded by
    another profile above its age limit.
  - **Casting** to a Chromecast.
- **Android TV.** An interface built for the remote.

**Play on**, in the web and Android players, sends what you are watching to
another Evey app signed in to the same account and currently open, and
playback continues there.

## Keeping the apps up to date

Each server release names the Android and Android TV builds released with it.
When the installed app is older, the Android app shows **Update available:**
followed by the version at the top of the screen (never over the player), and
the TV shows it in its top bar. Pressing it opens the app's Play Store page.

The Play Store updates an app on its own only when the device is idle and on
Wi-Fi, and can be turned off per app: a phone or a television can stay several
versions behind without anyone noticing, which is what this notice is for. A
server that predates it, or an app that is current, shows nothing. The web app
needs no update: it is served by the server itself.

## Which device plays what

Each app reports what its device can decode, and the server decides for each
title whether to send the file as it is or to adapt it. The same film can be
sent untouched to a TV and re-encoded for an older phone. The player's
**Stream info** panel shows the decision and its reasons: see
[How playback is decided](../playback/how-playback-is-decided.md). Casting has
rules of its own: see [Casting](../playback/casting.md).

<!-- sources
connect screen, checks and messages (Android): frontend/apps/mobile/app/server-connect.tsx - handleConnect, describeConnectError, handleSelectServer ; frontend/apps/mobile/src/lib/privateHost.ts - isPrivateHost, isCleartextToPublicHost, confirmInsecureServer ; frontend/packages/api/src/remoteAccessError.ts - remoteAccessDisabledMessage ; i18n mobile.serverConnect.*, auth.remoteAccess.*
connect screen (TV): frontend/apps/tv/app/server-connect.tsx - handleConnect (no scheme check, no cleartext warning, "Server returned N") ; unreachable banner: frontend/apps/tv/app/index.tsx
boot back to the screen: frontend/apps/mobile/app/index.tsx ; frontend/apps/tv/app/index.tsx
probe and identity: frontend/packages/client/src/native/server-config.ts - probeServerHealth ; frontend/packages/client/src/lib/serverIdentity.ts - parseEveyHealth ; internal/adapters/driving/http/health_handler.go (name, version)
scan: frontend/packages/client/src/lib/network-scan.ts - scanLocalNetwork (subnets, COMMON_PORTS, TIMEOUT_MS, BATCH) ; no mDNS anywhere in frontend/apps/mobile, frontend/apps/tv, frontend/packages/client
server list: @evey/store useServerListStore ; frontend/apps/mobile/app/(auth)/manage-servers.tsx ; frontend/apps/mobile/app/(auth)/(tabs)/settings.tsx - handleManageServers ; frontend/apps/mobile/app/login.tsx (Change server) ; frontend/apps/tv/app/(auth)/(tabs)/settings.tsx - handleChangeServer ; frontend/apps/tv/app/login.tsx ; i18n mobile.servers.*, mobile.settings.changeServer
port: docs/installation.md compose ports line ; cmd/evey/main.go - httpPort
quick connect: frontend/apps/tv/app/quick-connect.tsx (displayCode last 6 chars, qrValue /quickconnect?code=, polling) ; frontend/apps/tv/app/login.tsx (recommendedForTV) ; frontend/apps/web/src/routes/_auth/quickconnect.lazy.tsx ; frontend/apps/web/src/routes/_auth/profile/settings.lazy.tsx (quick-connect tab) ; frontend/apps/mobile/app/(auth)/(tabs)/settings.tsx (scanQr, authorize) ; frontend/apps/mobile/app/(auth)/qr-scan.tsx ; frontend/apps/mobile/app/quick-connect.tsx ; i18n auth.settings.quickConnect.*
offline window and profile check: frontend/apps/mobile/src/lib/offlineAccess.ts - OFFLINE_DOWNLOAD_TTL_MS
reduced copy (off by default): cmd/evey/http.go (downloads.quality_choice) ; i18n downloads.reduced.*
connect hint and help link, shown with no saved server: frontend/apps/mobile/app/server-connect.tsx - serverHelpUrl, WIKI_LOCALES ; frontend/apps/tv/app/server-connect.tsx - serverHelpUrl ; i18n mobile.serverConnect.hint, mobile.serverConnect.whereToFind, mobile.servers.yourServers
delete from the connect screen (Android): frontend/apps/mobile/app/server-connect.tsx - handleDeleteServer ; i18n mobile.servers.deleteConfirmTitle, mobile.servers.cantDeleteOnlyServer
update notice: internal/adapters/driving/http/health_handler.go (clients) ; scripts/client-release-ldflags.sh ; frontend/packages/client/src/native/appUpdate.ts - newerClientRelease ; frontend/apps/mobile/src/components/UpdateBanner.tsx (not on /player) ; frontend/apps/tv/src/components/TopHeader.tsx ; i18n mobile.appUpdate.available
play on: i18n player.playOn.* (used by frontend/apps/web and frontend/apps/mobile, not frontend/apps/tv)
-->
