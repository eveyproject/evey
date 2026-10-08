# Users, profiles and parental control

An **account** signs in; a **profile** watches. This page is for the
administrator who adds the household: create the accounts, let each one
create its profiles, mark the children's profiles as kids profiles and give
them an age limit. What an account can be restricted to, opening hours,
requests, public download links and the home page come after.

## Accounts and profiles

An account has a username and a password (or a single sign-on identity, see
[OIDC](../oidc.md)), and it is either an administrator or not. Each account
holds one or more profiles, each with its own watch history, resume points,
favourites, languages and home page. A household typically shares one
account, with a profile per person.

## Create an account

<figure class="evey-shot" markdown="span">
  ![Admin → Users on the demo server](../images/admin-users.webp){ loading=lazy }
  <figcaption><strong>Admin → Users</strong>: each account with its profiles, its role and its state.</figcaption>
</figure>

**Admin → Users** offers two ways:

- **Create User**: you choose the **Username** and the **Password**, tick
  **Admin** for an administrator, and can set the libraries, the number of
  profiles and the number of devices right there (see below).
- **Create Invite Link**: the person chooses their own. A link can have an
  expiry date (**Expires at**) and a maximum number of uses (**Max uses**),
  restrict the libraries the new account sees (**Accessible libraries**), cap
  the number of profiles it may create, and **Create as administrator**.
  **Invite Links** lists them; **Copy Link** copies one, **Revoke** ends it.

**Disable** on an account's row refuses its sign-ins without deleting
anything; **Enable** restores it. **Delete** removes the account and its
profiles.

## Profiles, kids profiles and the PIN

Each account creates its profiles in **Settings → Profiles → Create Profile**;
an administrator can also add one from **Edit User → New profile**. A profile
has a **Name**, a **Language** and a **Region**, and two settings that matter
for children:

- **Kids profile** marks the profile as a child's. It opens without a PIN,
  and leaving it for another profile asks for that profile's **PIN
  (4 digits)**. While a kids profile exists, every other profile of the
  account must have a PIN: the server refuses to save a profile without one.
  Setting a PIN the first time asks for the account password; changing it
  asks for the current PIN. **Settings → Parental** sets the PIN of the
  profile in use.
- **Max age rating** appears on a kids profile only: titles rated above it are
  never sent to the profile. Left on **Default (12+)**, the limit is 12.

Managing profiles and parental settings is only possible from a profile that
is not a kids profile. An administrator's account cannot have a kids profile:
the limit would gate the administrator's own library.

## Age limits

The age is read against the rating system of the profile's **Region**
(**Settings → Profiles → Edit**; **Automatic** derives it from the profile's
language, and falls back to the British system). A title whose rating that
system cannot place is hidden from the kids profile rather than shown.

The limit is applied by the server, not by the apps: a title above it is not
hidden in the interface, it is never sent. That holds for browsing and search,
for playback and for downloads. If the server cannot read a profile's settings
(a database error, for example), it refuses the content rather than allowing
it. Administrators are not subject to profile limits.

A film downloaded to a phone stays subject to the limit offline: a kids profile
cannot open a download made by another profile above its age limit.

## What an administrator can restrict per account

These settings live in **Create User** and **Edit User**, and on the account's
row in **Admin → Users**.

| Setting | Effect |
| --- | --- |
| **Library Access** (the **Libraries** button on the row) | The libraries the account sees. Everything else is absent from its searches, home page and API responses. An administrator always sees everything. |
| **Max Bitrate** (the **Bitrate** button) | A ceiling, from 1 to 40 Mbps, on the bitrate the account's players receive (0 = unlimited). A title above it is transcoded down to it. |
| **Max profiles** | How many profiles the account may create (0 = unlimited). |
| **Max devices per profile** | How many playbacks each profile may have running at once, across its devices (0 = unlimited). One more is refused when it starts. |

**Connect as** opens the server as that user sees it, to check a restriction
or reproduce a problem. A banner stays visible, and **Exit** returns to your
own account.

## Opening hours

A kids profile can be limited to **opening hours**: the clock icon next to it
in **Settings → Profiles** opens the editor. Nothing is set until you turn on
**Limit to opening hours**; the editor then suggests weekdays 18:00 to 22:00,
and Saturday and Sunday 9:00 to 12:00 and 14:00 to 21:00.

The hours are a list of time ranges, edited two ways: **Weekdays / weekend**
(one block for Monday to Friday, one for the weekend) or **Day by day**. A day
can have several ranges, a range can run past midnight (20:00 to 0:30), and a
day with no range is closed. Switching back to the week view is refused when
the days of a block differ, so nothing is lost silently.

The server enforces the hours, in its own time zone (**Admin → Server Config →
Language & Region → Server time zone**; without it, the `TZ` variable, then
Europe/Paris). Changing the clock of a tablet changes nothing. Outside the
hours the profile is refused everywhere - browsing, every segment of a film in
progress, music, downloads - and every app shows a lock screen saying when it
opens again. Ten minutes before closing, the player shows a warning; at the
time, playback stops.

On the lock screen, the **Parent PIN** (the PIN of a profile of the same
account that is not a kids profile) opens the kids profile until its next
opening; the child's own PIN, if it has one, does not. **Switch profile** leads
to the profile picker. A schedule that the server cannot read keeps the
profile closed rather than open.

A film downloaded to a phone and played without a connection does not reach the
server, so the hours do not apply to it.

## Requests

Users can search for a film, a series or an artist that is missing from the
libraries and ask for it. Requests arrive in **Admin → Media Requests**, where
an administrator chooses **Approve**, **Reject** or **Later**, with a note.
The **Access** tab sets who may request: **Allow requests by default for all
users**, with **Per-user overrides**.

## Public download links

On a film or episode page, an administrator can create a **Public download
link** (web and Android apps). It is a URL anyone can open, without an account,
to download the original file; it is copied to the clipboard.

- It expires after 48 hours. Through the server API it can last up to 72 hours
  and be limited to a number of downloads (`POST /api/v1/media/{id}/share`).
- Only administrators can create, list and revoke links. The apps only create
  them: listing and revoking go through the API (`GET /api/v1/admin/share-links`,
  `DELETE /api/v1/share-links/{id}`).
- With remote access off, the link only works from the local networks.
- Whoever holds the link can download the file until it expires: share it like
  the file itself.

## The home page

The home page is a stack of rows; a row with nothing to show is hidden.

| Row | What it shows |
| --- | --- |
| **Resume Playback** | What the profile started and has not finished, most recent first. A title counts as watched at 90 %. |
| **Up Next** | For each series the profile has watched an episode of, the next unwatched episode. |
| **Collections** | The collections, with a link to the full list. |
| **Recommended For You** | Films and series in the profile's usual genres, best match first, then by rating. A genre counts when it appears in at least 3 of the profile's last 200 playbacks. |
| **Most Popular** | The films and series played most on the server over the last 90 days. |
| One row per library | The latest additions to that library, newest first. |

Recommended For You and Most Popular leave out what the profile has already
finished; a series counts as finished once every episode is watched. Plugins can
add their own rows.

**Admin → Server Config → Home feeds** sets the default for every profile:
which rows appear, in what order, and how many items each shows. Each profile
then arranges its own home with **Customize** (on the home page of the web and
mobile apps, in the header of the TV app): show or hide a row, move it, or
**Reset to Defaults**. The choice is saved on the server and follows the profile
to every device. A profile can show a row the administrator has hidden, except
a plugin's row: there the administrator's choice is final.

<!-- sources
create user / edit user: frontend/apps/web/src/features/admin/CreateUserForm.tsx, EditUserModal.tsx (CreateProfileDialog), UserRow.tsx (edit, impersonate, libraries, bitrate, enable/disable, delete) ; i18n admin.users.createUser, username, password, admin, libraryAccessTitle, maxProfiles, maxDevicesPerProfile, impersonate, exitImpersonation ; common.enable, disable, delete
invites: frontend/apps/web/src/features/admin/CreateInviteForm.tsx, InviteLinkRow.tsx ; i18n admin.users.maxUsesLabel, expiresAtLabel, inviteLibraries, inviteMaxProfilesLabel, inviteIsAdmin, inviteLinks, copyLink, revoke ; internal/app/user/access_service.go (RegisterWithInvite, RevokeInviteLink)
bitrate: frontend/apps/web/src/features/admin/users-constants.ts BITRATE_OPTIONS (1-40 Mbps) ; internal/app/user/bitrate_limit.go
max devices per profile = concurrent active playback sessions of the profile: internal/app/playback/command_service.go (DeviceQuotaExceeded, ErrMaxDevicesPerProfile)
profiles: frontend/apps/web/src/features/profile/EditProfileDialog.tsx (isKids, maxAge kids-only, pin, region, locale), CreateProfileDialog.tsx, ProfileManagerList.tsx (Clock on kids profiles) ; i18n auth.profile.manage.createProfile, fields.*, settings.parental.*, settings.parental.defaultMaxAge ; errors.auth.pin_required_for_non_kids, parental_needs_non_kids
kids invariants: internal/app/user/user_service.go checkProfileInvariants (ErrKidsRequiresPIN, ErrAdminCannotBeKids, ErrMaxProfilesReached) ; service.go ErrPINRequired ; internal/domain/user/model.go EffectiveParental, DefaultKidsMaxAge = 12
rating region: internal/pkg/region/region.go Resolve (profile region, then locale, then GB ; server region passed empty) ; internal/adapters/driving/http/parental.go BlockedByParental (WorstCaseMinAge, worst < 0 blocked) ; grpcapi/library_handler_parental.go
server-side limit, fail closed, admins exempt: internal/adapters/driving/http/parental.go checkParental ; grpcapi/library_handler_parental.go
downloads: internal/adapters/driving/http/compat_download_handler.go (checkParental) ; frontend/apps/mobile/src/lib/offlineAccess.ts
library access: internal/app/user/access_service.go (GrantMany, RevokeAllLibraryAccess) ; UserRow hides the Libraries button for admins
requests: i18n admin.mediaRequests.* (approve, reject, later, tabAccess, accessDefaultLabel, accessOverridesTitle) ; requests.*
share links: internal/adapters/driving/grpcapi/interceptors/rbac.go (ShareService RoleAdmin) ; grpcapi/share_handler.go (48 h default) ; internal/app/media/share_service.go (max 72 h, maxViews) ; http/share_download_handler.go (no auth) ; cmd/evey/gateway.go - networkGateExempt (share not exempt) ; frontend/apps/web/src/features/library/ShareLinkButton.tsx, mobile media/[id].tsx (isAdmin) ; proto/evey/v1/share.proto
opening hours: internal/domain/user/schedule.go - AccessSchedule, DefaultKidsSchedule, Window, AccessAt, OverrideUntil ; internal/app/user/profile_schedule.go - Unlock (adult PIN only) ; internal/adapters/driving/http/profile_closed.go (httpAuthMiddleware, httpStreamAuthMiddleware) ; grpcapi/interceptors/profile_hours.go ; grpcapi/playback_closing_soon.go (closingWarnLead 10 min) ; internal/app/config/timezone.go - KeyServerTimezone ; postgres profile_repo.go - scheduleFromJSON (unreadable = closed) ; i18n auth.profile.schedule.*, auth.profile.locked.*, admin.serverConfig.timezone
home: internal/app/feed/* ; i18n admin.serverConfig.feedConfig.*, media.home.*
-->
