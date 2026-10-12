---
source_sha: 1c1f263382d37fa80c540dd5129e11b0ecffa890475ab9305d76db4d94571396
reviewed: true
---
# Benutzer, Profile und Kindersicherung

Zwei Wörter sind auf dieser Seite wichtig:

- Ein **Konto** ist das, womit man sich anmeldet: ein Benutzername und ein
  Passwort.
- Ein **Profil** ist eine Person, die zuschaut. Jedes Profil hat seinen
  eigenen Verlauf, seine Fortsetzungspunkte, Favoriten, Sprachen und seine
  eigene Startseite.

Ein Konto kann mehrere Profile haben. Die meisten Haushalte teilen sich ein
Konto, mit einem Profil pro Person. Sie können aber auch jedem Erwachsenen ein
eigenes Konto geben.

Diese Seite beginnt mit den häufigen Aufgaben: jemanden hinzufügen, ein Kind
hinzufügen. Die feineren Einstellungen (Grenzen pro Konto, Nutzungszeiten,
Anfragen, die Startseite, Download-Links) folgen danach.

## Konten und Profile

Ein Konto hat einen Benutzernamen und ein Passwort, oder es meldet sich über
Single Sign-on an (siehe [OIDC](../oidc.md)). Es ist entweder ein
Administrator- oder ein normales Konto. Jedes neue Konto beginnt mit einem
Profil, das wie sein Benutzername heißt.

## Ein Konto anlegen

<figure class="evey-shot" markdown="span">
  ![Verwaltung → Benutzer auf dem Demo-Server](../images/admin-users.webp){ loading=lazy }
  <figcaption><strong>Verwaltung → Benutzer</strong>: jedes Konto mit seinen Profilen, seiner Rolle und seinem Zustand (Oberfläche auf Englisch).</figcaption>
</figure>

Gehen Sie zu **Verwaltung → Benutzer**. Es gibt zwei Wege.

**Einen Einladungslink schicken**, damit die Person ihr Passwort selbst wählt:

1. Drücken Sie auf **Einladungslink erstellen**.
2. Auf Wunsch können Sie ihn begrenzen: **Ablaufdatum (optional)**,
   **Max. Verwendungen (leer = unbegrenzt)**, **Zugängliche Bibliotheken**
   (die Bibliotheken, die das neue Konto sieht) und die Zahl der Profile, die
   es anlegen darf. Setzen Sie das Häkchen bei **Als Administrator anlegen**
   nur für jemanden, der den Server mit Ihnen verwalten wird.
3. Drücken Sie unter **Einladungslinks** auf **Link kopieren** und schicken
   Sie den Link.

Der Link wird aus der Adresse in Ihrem Browser gebildet. Öffnen Sie den
Verwaltungsbereich über die Adresse, die auch die anderen benutzen werden (zum
Beispiel `http://192.168.1.20:9091`), nicht über `localhost`. **Widerrufen**
macht einen Link unbrauchbar.

**Oder legen Sie das Konto selbst an**: Drücken Sie auf **Benutzer
erstellen**, wählen Sie **Benutzername** und **Passwort** und geben Sie beides
der Person. Setzen Sie für einen Administrator das Häkchen bei
**Administrator**. Sie können auch gleich die Bibliotheken, die Zahl der
Profile und die Zahl der Geräte festlegen (siehe
[Was ein Administrator pro Konto einschränken kann](#what-an-administrator-can-restrict-per-account)).

In der Zeile eines Kontos:

- **Deaktivieren** sperrt seine Anmeldungen, ohne etwas zu löschen.
  **Aktivieren** erlaubt wieder Anmeldungen.
- **Löschen** entfernt das Konto und seine Profile.

## Profile, Kinderprofile und die PIN

**Ein Profil hinzufügen**: Gehen Sie in diesem Konto zu **Einstellungen →
Profile → Profil erstellen**. Ein Administrator kann auch jedem Konto ein
Profil hinzufügen, über **Benutzer bearbeiten → Neues Profil**. Ein Profil hat
einen **Name**, eine **Sprache** und eine **Region**.

**Ein Kinderprofil hinzufügen**:

1. Legen Sie das Profil an und setzen Sie das Häkchen bei **Kinderprofil**.
2. Wählen Sie eine **Max. Altersfreigabe**. Wenn Sie **Standard (12+)**
   lassen, liegt die Grenze bei 12.
3. Geben Sie jedem anderen Profil des Kontos eine **PIN (4 Ziffern)**. Der
   Server verlangt das, sobald es ein Kinderprofil gibt, und speichert kein
   Profil ohne PIN.

Was das Kind dann sieht:

- Das Kinderprofil öffnet sich ohne PIN.
- Wer zu einem anderen Profil wechselt, muss die PIN dieses Profils eingeben.
- Titel über der Altersgrenze erscheinen nie (siehe
  [Altersgrenzen](#age-limits)).

??? info "Einzelheiten zur PIN und wer Profile verwalten kann"

    - Wenn Sie zum ersten Mal eine PIN festlegen, wird das Passwort des
      Kontos abgefragt. Wenn Sie sie ändern, wird die aktuelle PIN abgefragt.
      **Einstellungen → Kindersicherung** legt die PIN des gerade benutzten
      Profils fest.
    - Profile und Einstellungen der Kindersicherung lassen sich nur von einem
      Profil aus verwalten, das kein Kinderprofil ist.
    - Das Konto eines Administrators kann kein Kinderprofil haben: Die Grenze
      würde einen Teil der eigenen Bibliothek des Administrators verbergen.

## Altersgrenzen

Der Server wendet die Altersgrenze an, nicht die App. Ein Titel über der
Grenze wird nicht nur auf dem Bildschirm versteckt: Er wird nie an das
Kinderprofil geschickt. Das gilt beim Stöbern, bei der Suche, bei der
Wiedergabe und bei Downloads.

- **Welche Freigaben gelten.** Das Alter wird aus dem Freigabesystem der
  **Region** des Profils gelesen (**Einstellungen → Profile → Bearbeiten**).
  Bei **Automatisch** ergibt sich die Region aus der Sprache des Profils.
  Hilft das nicht weiter, wird das britische System verwendet.
- **Ein Titel ohne brauchbare Freigabe** wird im Kinderprofil versteckt
  statt gezeigt.
- **Im Zweifel nein.** Kann der Server die Einstellungen eines Profils nicht
  lesen (zum Beispiel wegen eines Datenbankfehlers), verweigert er den Inhalt,
  statt ihn zu erlauben.
- **Auch Downloads.** Ein auf ein Handy heruntergeladener Film behält seine
  Grenze auch offline: Ein Kinderprofil kann einen Download, den ein anderes
  Profil gemacht hat, nicht öffnen, wenn er über seiner Altersgrenze liegt.

Für Administratoren gelten keine Profilgrenzen.

## Was ein Administrator pro Konto einschränken kann

Diese Einstellungen finden Sie unter **Benutzer erstellen** und **Benutzer
bearbeiten** sowie in der Zeile des Kontos unter **Verwaltung → Benutzer**.

| Einstellung | Was sie bewirkt |
| --- | --- |
| **Bibliothekszugriff** (die Schaltfläche **Bibliotheken** in der Zeile) | Die Bibliotheken, die das Konto sieht. Die anderen tauchen für dieses Konto nirgends auf: nicht in der Suche, nicht auf seiner Startseite. Ein Administrator sieht immer alles. |
| **Maximale Bitrate** (die Schaltfläche **Bitrate**) | Begrenzt die Videoqualität, die das Konto bekommt, von 1 bis 40 Mbit/s (0 = keine Grenze). Ein Titel darüber wird auf diesen Wert heruntergerechnet. |
| **Max. Profile** | Wie viele Profile das Konto anlegen darf (0 = keine Grenze). |
| **Max. Geräte pro Profil** | Wie viele Videos jedes Profil gleichzeitig abspielen darf, über alle seine Geräte (0 = keine Grenze). Ein weiteres wird beim Start abgelehnt. |

**Anmelden als** zeigt den Server genau so, wie dieser Benutzer ihn sieht.
Nutzen Sie das, um eine Einschränkung zu prüfen oder ein Problem selbst zu
sehen. Ein Banner bleibt auf dem Bildschirm, und **Beenden** bringt Sie zurück
zu Ihrem eigenen Konto.

## Nutzungszeiten

Sie können ein Kinderprofil nur zu bestimmten Zeiten schauen lassen.

1. Drücken Sie unter **Einstellungen → Profile** auf das Uhrsymbol neben dem
   Kinderprofil.
2. Schalten Sie **Auf Nutzungszeiten beschränken** ein. Evey schlägt
   werktags 18:00 bis 22:00 Uhr vor, und samstags und sonntags 9:00 bis
   12:00 Uhr und 14:00 bis 21:00 Uhr.
3. Passen Sie die Zeiten an, entweder mit **Wochentage / Wochenende** (ein
   Block für Montag bis Freitag, einer für das Wochenende) oder mit **Tag für
   Tag**.

Ein Tag kann mehrere Zeitspannen haben. Eine Zeitspanne darf über Mitternacht
gehen (20:00 bis 0:30 Uhr). Ein Tag ohne Zeitspanne ist gesperrt.

Was passiert:

- **Außerhalb der Zeiten** ist das Profil überall gesperrt: Stöbern, Filme,
  Musik, Downloads. Jede App zeigt einen Sperrbildschirm, der sagt, wann es
  wieder freigegeben ist.
- **Zehn Minuten vor Schluss** zeigt der Player eine Warnung. Zur
  Schlusszeit hört die Wiedergabe auf.
- **Um das Kind trotzdem schauen zu lassen**, geben Sie auf dem
  Sperrbildschirm die **Eltern-PIN** ein: die PIN eines Profils desselben
  Kontos, das kein Kinderprofil ist. Sie öffnet das Kinderprofil bis zur
  nächsten regulären Öffnung. Die eigene PIN des Kindes funktioniert dort
  nicht. **Profil wechseln** führt zurück zur Profilliste.

??? info "Zeitzone und Sonderfälle"

    - Der Server wendet die Zeiten in seiner eigenen Zeitzone an:
      **Verwaltung → Serverkonfiguration → Sprache & Region → Zeitzone des
      Servers** (wenn leer, die Variable `TZ`, dann Europe/Paris). Die Uhr
      auf einem Tablet umzustellen ändert nichts.
    - Der Wechsel von **Tag für Tag** zurück zu **Wochentage / Wochenende**
      wird abgelehnt, wenn sich die Tage eines Blocks unterscheiden. So geht
      nichts aus Versehen verloren.
    - Ein Zeitplan, den der Server nicht lesen kann, lässt das Profil
      gesperrt.
    - Ein heruntergeladener Film, der ohne Verbindung abgespielt wird,
      erreicht den Server nie. Für ihn gelten die Zeiten also nicht.

## Anfragen

Man kann einen Film, eine Serie oder einen Künstler anfragen, den Sie noch
nicht haben. Diese Anfragen erscheinen unter **Verwaltung → Medienanfragen**.
Wählen Sie bei jeder **Genehmigen**, **Ablehnen** oder **Später**, und fügen
Sie auf Wunsch eine Notiz hinzu. Die Person wird über Ihre Antwort
benachrichtigt.

Um festzulegen, wer anfragen darf, öffnen Sie den Reiter **Zugriff**:
**Anfragen standardmäßig für alle Benutzer erlauben**, dann **Sonderregelungen
pro Benutzer** für Ausnahmen.

## Die Startseite

Die Startseite besteht aus Reihen, die untereinander stehen. Eine Reihe, die nichts zu zeigen
hat, wird ausgeblendet.

| Reihe | Was sie zeigt |
| --- | --- |
| **Wiedergabe fortsetzen** | Was das Profil angefangen und nicht beendet hat, das Neueste zuerst. Ein Titel gilt ab 90 % als gesehen. |
| **Als Nächstes** | Für jede Serie, die das Profil angefangen hat, die nächste noch nicht gesehene Folge. |
| **Sammlungen** | Die Sammlungen, mit einem Link zur vollständigen Liste. |
| **Für Sie empfohlen** | Filme und Serien aus den Genres, die das Profil am meisten schaut, die beste Übereinstimmung zuerst, dann nach Bewertung. Ein Genre zählt, wenn es in mindestens 3 der letzten 200 Wiedergaben des Profils vorkommt. |
| **Am beliebtesten** | Die Filme und Serien, die in den letzten 90 Tagen auf dem Server am häufigsten abgespielt wurden. |
| Eine Reihe pro Bibliothek | Die neuesten Zugänge dieser Bibliothek, das Neueste zuerst. |

**Für Sie empfohlen** und **Am beliebtesten** lassen weg, was das Profil schon
zu Ende geschaut hat. Eine Serie gilt als zu Ende geschaut, sobald jede Folge
gesehen ist. Plugins können eigene Reihen hinzufügen.

**Wer über die Reihen entscheidet:**

- Sie legen die Vorgabe für jedes Profil unter **Verwaltung →
  Serverkonfiguration → Startseiten-Feeds** fest: welche Reihen erscheinen, in
  welcher Reihenfolge und wie viele Einträge jede zeigt.
- Jedes Profil kann dann seine eigene Startseite mit **Anpassen** einrichten
  (auf der Startseite der Web- und Handy-Apps, in der oberen Leiste der
  TV-App): eine Reihe zeigen oder ausblenden, sie verschieben oder
  **Zurücksetzen**. Das wird auf dem Server gespeichert und gilt für das Profil
  auf jedem Gerät.
- Ein Profil kann eine Reihe zeigen, die Sie ausgeblendet haben, außer der
  Reihe eines Plugins: Dort gilt Ihre Entscheidung.

## Öffentliche Download-Links

Auf der Seite eines Films oder einer Folge kann ein Administrator einen
**Öffentlicher Download-Link** erstellen (Web- und Android-Apps). Jeder, der
den Link hat, kann die Originaldatei herunterladen, ohne Konto. Der Link wird
für Sie kopiert, damit Sie ihn einfügen können.

- Nach 48 Stunden funktioniert er nicht mehr.
- Ist der Fernzugriff ausgeschaltet, funktioniert er nur in Ihrem Heimnetz.
- Wer den Link hat, kann die Datei herunterladen, bis er abläuft: Teilen Sie
  ihn so vorsichtig wie die Datei selbst.

??? info "Über die API des Servers"

    Nur Administratoren können Links erstellen, auflisten und widerrufen. Die
    Apps erstellen sie nur; Auflisten und Widerrufen geht über die API
    (`GET /api/v1/admin/share-links`, `DELETE /api/v1/share-links/{id}`).
    Über die API kann ein Link bis zu 72 Stunden gelten und auf eine Zahl von
    Downloads begrenzt werden (`POST /api/v1/media/{id}/share`).

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
first profile named after the username: internal/app/user/user_service.go, access_service.go - RegisterWithInvite (NewProfile with u.Username)
invite link built from the admin's browser address: frontend/apps/web/src/features/admin/InviteLinkRow.tsx - inviteUrl (window.location.origin)
requester notified of the answer: internal/app/mediarequest/service.go - Answer ; i18n notifications push.request_answered
labels: i18n auth.profile.schedule.enable, modeWeek, modeDaily ; auth.profile.locked.pinLabel, switchProfile ; media.shareLink ; admin.users.admin, libraries, bitrate, inviteIsAdmin, revoke
-->
