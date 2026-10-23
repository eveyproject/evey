# First setup

Evey is running, and you opened its address in a browser, for example
`http://192.168.1.20:9091` (not sure which address:
[Step 3 of Install](../installation.md#the-address-to-open-next)).

Four steps remain before your household can watch:

1. [The setup wizard](#1-the-setup-wizard): about a minute.
2. [Add your libraries](#2-add-your-libraries): tell Evey where your media is.
3. [Invite your household](#3-invite-your-household): one account each, or
   one shared account with a profile per person.
4. [Connect the apps](#4-connect-the-apps): phones, tablets and TVs.

## 1. The setup wizard

Until the server has an administrator, its address always opens the wizard.
It shows **Step 1 of 4 - Language** at the top, and four dots that fill in
as you go.

### Step 1 of 4 - Language

The card says **Choose your server language**. Pick:

- **Language**: the default language of new profiles, and of the titles and
  summaries Evey fetches. The wizard switches to it at once.
- **Region**: filled in from the language. It decides which age ratings are
  shown. **Automatic** is available.

You can change both later in the admin panel. Press **Continue**.

### Step 2 of 4 - Welcome

Nothing to fill in. Press **Get Started**.

<div class="evey-shot-pair" markdown>

<figure class="evey-shot" markdown="span">
  ![Step 1 of the wizard: language and region](../images/setup-wizard-1-language.webp){ loading=lazy data-gallery="setup" }
  <figcaption>Step 1: the language of the server, and the region used for age ratings.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Step 2 of the wizard: welcome](../images/setup-wizard-2-welcome.webp){ loading=lazy data-gallery="setup" }
  <figcaption>Step 2: nothing to fill in.</figcaption>
</figure>

</div>

### Step 3 of 4 - Admin account

The card says **Create admin account**. This is your own account, the one
that manages the server.

| Field | What to type |
| --- | --- |
| **Username** | At least 3 characters. |
| **Password** | At least 8 characters. |
| **Confirm password** | The same password again. |
| **Server name** | Optional. A name such as "Living room". People see it on the sign-in page, in the browser tab, in the apps' list of servers and in e-mails. You can change it later in **Admin → Server Config → Server Name**. |

Press **Continue**. Evey creates the account and signs you in. The wizard
will not appear again.

### Step 4 of 4 - First library

The card says **Add your first library**. A library is one kind of media in
one or more folders, for example "Films".

- **Library name**: for example "Films".
- **Library type**: **Movies**, **Shows**, **Music** or **Photos**.
- **Paths**: pick a folder from the list. It shows the folders Evey can see,
  starting with `/media`. To type a path such as `/media/movies`, choose
  **Enter custom path...**.

Press **Create**: Evey starts reading the folder. Or press **Skip** and add
libraries later. Both take you to the home page.

<div class="evey-shot-pair" markdown>

<figure class="evey-shot" markdown="span">
  ![Step 3 of the wizard: the administrator account](../images/setup-wizard-3-admin.webp){ loading=lazy data-gallery="setup" }
  <figcaption>Step 3: the administrator account; the server name is optional.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Step 4 of the wizard: the first library](../images/setup-wizard-4-library.webp){ loading=lazy data-gallery="setup" }
  <figcaption>Step 4: a name, a type, and a folder as Evey sees it.</figcaption>
</figure>

</div>

??? note "If the wizard does not appear, or appears twice"

    Once an administrator exists, the address opens the sign-in page, and
    `/setup` sends you there too. If two people open the wizard at the same
    time, only the first one gets the administrator account: the second is
    sent to the sign-in page.

<figure class="evey-shot" markdown="span">
  ![The admin dashboard of the demo server](../images/admin-dashboard.webp){ loading=lazy }
  <figcaption>The admin panel once signed in: the dashboard, and every section in the side menu.</figcaption>
</figure>

## 2. Add your libraries

Go to **Admin → Libraries → Add Library**. Give a name, a type (Movies,
Shows, Music or Photos) and one or more folders.

The folders are named as Evey sees them, not as they are on your computer.
With the compose file from [Install](../installation.md#the-compose-file),
your media folder is `/media`. So a folder `/srv/media/films` on your
computer is `/media/films` in Evey.

Evey starts reading the folder by itself. Posters, summaries and cast arrive
in the background, so a large library fills in over a few minutes. New files
you add later appear on their own.

Your titles do not show up, or show the wrong poster? See
[Libraries and metadata](../administration/libraries.md): how to name files,
and how to fix a wrong match.

## 3. Invite your household

There are two ways to give someone an account, both in **Admin → Users**.

**Send an invitation link** (the person picks their own password):

1. Open the admin panel from the address the others will use, for example
   `http://192.168.1.20:9091`, not `http://localhost:9091`. The link is built
   from the address in your browser.
2. Press **Create Invite Link**. You can limit how long it works
   (**Expires at (optional)**), how many times it can be used
   (**Max uses (empty = unlimited)**) and which libraries the new account
   sees (**Accessible libraries**).
3. In **Invite Links**, press **Copy Link** and send it by message or e-mail.
4. The person opens the link, sees **You've been invited! Fill in your details
   below.**, picks a username and a password (at least 8 characters), and is
   signed in.

**Or create the account yourself**: press **Create User**, choose the
**Username** and the **Password**, and give them to the person.

Every account starts with one profile, named after the username. A household
can share one account, with one profile per person: add them from
**Settings → Profiles → Create Profile**. For a child, tick **Kids profile**
and choose a **Max age rating**. Every other profile of that account then
needs a **PIN (4 digits)**, so that the child cannot switch to it.

Then send each person three things: the address, their username and
password (or the invitation link), and the page written for them,
[Watching with Evey](watching.md).

More options (limits per account, opening hours, PINs, age limits):
[Users, profiles and parental control](../administration/users.md).

## 4. Connect the apps

- **In a browser**, there is nothing to install: it is the address you
  already opened.
- **On an Android phone, tablet or TV**, the app asks for the server address
  first. Type the **same address as in the browser**, with `http://` and the
  port, for example `http://192.168.1.20:9091`. Then sign in.
- **On a TV**, you can sign in with a code approved from your phone, instead
  of typing a password with the remote.

Step by step, and what each error message means:
[Connect the apps](apps.md#the-server-address-to-enter).

## What the server does by itself

You do not need to set any of this:

- **How each device plays a file.** Each device tells the server what it can
  play. For each title, the server then sends the file as it is, repackages
  it, or converts only what the device cannot play. See
  [How playback is decided](../playback/how-playback-is-decided.md).
- **How many people it can serve.** At its first quiet moment, the server
  measures how much video it can convert at once, with the processor and any
  graphics card, and never accepts more. This takes a few minutes. Meanwhile,
  a playback that needs a conversion is asked to wait. See
  [Transcoding and server capacity](../playback/transcoding.md).
- **Access from outside your home.** Off until you give the server a public
  address. See [Remote access and HTTPS](../administration/remote-access.md).
- **A safety copy of the database** before every update that changes it.
  Regular backups are one setting away:
  [Backups and updates](../backups.md).

## Before you share the address outside your home

1. Put the server behind HTTPS. Evey itself only speaks plain HTTP, which is
   fine at home but not over the internet.
   [Remote access and HTTPS](../administration/remote-access.md).
2. Set a backup schedule, and keep a copy of the `evey-data` folder. It holds
   the database and the key that protects every sign-in.
   [Backups and updates](../backups.md).

<!-- sources
wizard screens, order and buttons: frontend/apps/web/src/routes/setup.tsx (STEPS, StepDots, handlePickLanguage, validateAdmin, handleAdminSubmit, handleCreateLibrary) ; i18n auth.setup.* ; common.language, common.region.*, common.continue, common.back, common.skip, common.create
every address opens on the wizard until setupComplete: frontend/apps/web/src/routes/__root.tsx ; frontend/apps/web/src/routes/index.tsx ; /setup redirects to /login once complete, 409 on the admin step goes to /login: setup.tsx
folder drop-down: GET /api/v1/admin/fs/browse ; internal/adapters/driving/http/browse_handler.go - mediaBrowseGlob "/media*" ; i18n admin.libraries.paths, selectDirectory, enterCustomPath
library types: i18n auth.setup.libraryTypes (movies, shows, music, photos)
apps ask for the address: frontend/apps/mobile/app/server-connect.tsx ; frontend/apps/tv/app/server-connect.tsx
calibration at quiet moment, wait: internal/app/transcoding/calibration_service.go
remote access inference: docs/adr/0026-remote-access-inferred-from-a-published-url.md
pre-migration dump: internal/adapters/driven/postgres/migrate.go
server name in admin: frontend/apps/web/src/features/admin/serverConfig/ServerConfigSections.tsx ; i18n admin.nav.serverConfig, admin.serverSettings.serverName
invite link built from the admin's browser address: frontend/apps/web/src/features/admin/InviteLinkRow.tsx - inviteUrl (window.location.origin) ; i18n admin.users.createInviteLink, expiresAtLabel, maxUsesLabel, inviteLibraries, inviteLinks, copyLink, createUser
invite page, password 8 chars, signed in after: frontend/apps/web/src/routes/register.tsx - handleSubmit ; i18n auth.register.invitedMessage, passwordMin
first profile named after the username: internal/app/user/user_service.go, internal/app/user/access_service.go - RegisterWithInvite (NewProfile with u.Username)
kids profile forces a PIN on the others: frontend/apps/web/src/features/profile/CreateProfileDialog.tsx (pinRequired) ; i18n auth.profile.manage.createProfile, auth.profile.fields.isKids, maxAge, pin ; auth.settings.tabs.profiles
-->
