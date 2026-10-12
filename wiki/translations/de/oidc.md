---
source_sha: 5f2dd5bf7c21906e243c0cece2ecff83e306d69d5b99a193541b9e574db7a4c2
reviewed: true
---
# Single Sign-on (OIDC)

Lassen Sie die Leute sich über den Identitätsanbieter anmelden, den Sie schon
betreiben (Keycloak, Authentik, Authelia, Google oder jeder andere
OpenID-Connect-Anbieter): Die Anmeldeseite der Web- und Android-Apps zeigt
dann neben dem Passwortformular **Mit SSO anmelden**. Für den Administrator
sind es zwei Schritte: Evey beim Anbieter eintragen, dann eine Karte im
Verwaltungsbereich ausfüllen.

## 1. Evey beim Anbieter eintragen

Legen Sie beim Anbieter einen Client (oder eine Anwendung) an, mit dem Ablauf
Authorization Code, den Scopes `openid email profile`, einem Client-Secret und
den beiden Adressen, die die Karte **Single Sign-on (SSO)** unter **Diese
Adressen beim Anbieter eintragen** zeigt:

| Feld | Wert |
| --- | --- |
| **Weiterleitungs-URI** | `https://<your address>/api/v1/auth/oidc/callback` |
| **Weiterleitungs-URI nach der Abmeldung** | `https://<your address>/login` |

Beide werden aus der öffentlichen Adresse des Servers gebildet (**Verwaltung →
Fernzugriff**): Tragen Sie diese zuerst ein, wenn die Leute Evey unter einer
anderen Adresse erreichen.

## 2. Evey einrichten

**Verwaltung → Serverkonfiguration → Single Sign-on (SSO)**:

| Feld | Wirkung |
| --- | --- |
| **Single Sign-on aktivieren** | Zeigt die SSO-Schaltfläche auf den Anmeldebildschirmen. |
| **Issuer-URL** | Die Adresse des Anbieters; Evey liest darunter `/.well-known/openid-configuration`. |
| **Client-ID**, **Client-Secret** | Der oben eingetragene Client. Das Secret bleibt auf dem Server und wird nie wieder angezeigt; lassen Sie die Punkte stehen, bleibt es erhalten. |
| **Zugelassene E-Mail-Domains** | Durch Kommas getrennt. Ist das Feld ausgefüllt, werden nur diese Adressen angenommen, und der Anbieter muss die Adresse bestätigen. |
| **Beschriftung der Schaltfläche** | Freiwillig, ersetzt „Mit SSO anmelden“. |
| **Mit bestehenden Konten über bestätigte E-Mail verknüpfen** | Standardmäßig an. Siehe [Wer sich anmelden kann](#who-can-sign-in). |
| **Konten für neue SSO-Benutzer anlegen** | Aus, außer es sind E-Mail-Domains zugelassen. Siehe [Wer sich anmelden kann](#who-can-sign-in). |
| **Bibliothekszugriff neuer SSO-Konten** | **Alle Bibliotheken** oder **Nur die ausgewählten Bibliotheken**. |

**Verbindung testen** prüft, ob der Issuer antwortet. **SSO-Einstellungen
speichern** gilt sofort, ohne Neustart. Die Zeile **Status** sagt, ob der
Anbieter erreichbar ist, und nennt sonst den letzten Fehler.

Die Variablen `EVEY_OIDC_*` aus
[Konten und Anmeldung](environment/accounts.md#single-sign-on-oidc) sind der
Rückfall: Die Karte sagt, wenn ihre Werte von dort kommen, und sobald Sie die
Karte speichern, gilt die Karte.

!!! note "Der Anbieter liegt nie auf dem kritischen Pfad des Servers"
    Der Server startet auch, wenn der Anbieter ausgefallen ist. Die
    SSO-Schaltfläche bleibt verborgen, solange der Anbieter nicht erreichbar
    ist, und die Anmeldung mit Passwort funktioniert weiter. Der Anbieter wird
    beim nächsten Anmeldeversuch wieder kontaktiert, höchstens alle
    30 Sekunden.

## Wer sich anmelden kann

1. Ein Konto, das sich schon einmal über diesen Anbieter angemeldet hat,
   meldet sich wieder an, außer es ist deaktiviert.
2. Sonst, wenn **Mit bestehenden Konten über bestätigte E-Mail verknüpfen**
   an ist: Ein bestehendes Konto mit derselben E-Mail wird mit dem Anbieter
   verknüpft und angemeldet, aber nur, wenn der Anbieter die Adresse
   bestätigt. Das Konto behält sein Passwort. Administratorkonten werden nie
   auf diese Weise verknüpft.
3. Sonst, wenn **Konten für neue SSO-Benutzer anlegen** an ist: Ein neues
   Konto wird angelegt, mit einem Profil und dem in der Karte gewählten
   Bibliothekszugriff. Sein Benutzername ist der `preferred_username` des
   Anbieters, sonst der Teil der E-Mail vor dem `@`; ein schon vergebener Name
   bekommt eine kurze Endung.

!!! warning "Warum das Anlegen von Konten standardmäßig aus ist"
    Bei einem öffentlichen Anbieter wie Google könnte sonst jeder, der dort
    ein Konto hat, auch eines auf Ihrem Server bekommen. Das Anlegen ist nur
    dann standardmäßig an, wenn **Zugelassene E-Mail-Domains** einschränkt,
    wer infrage kommt. Ist es aus, können sich nur bestehende Konten über SSO
    anmelden.

Wird eine Anmeldung abgelehnt, sagt die Anmeldeseite, warum: Die
E-Mail-Domain ist nicht zugelassen, kein Konto passt und die Registrierung ist
aus, das Konto ist deaktiviert, das Konto existiert, konnte aber nicht
verknüpft werden, oder der Anbieter ist nicht verfügbar.

## Abmelden

Veröffentlicht der Anbieter einen Endpunkt zum Abmelden, beendet das Abmelden
bei Evey nach einer SSO-Anmeldung auch die Sitzung beim Anbieter und führt
dann zurück zur Anmeldeseite von Evey. Die Karte sagt, ob der Anbieter das
unterstützt.

<!-- sources
settings and precedence (panel keys oidc.* win over EVEY_OIDC_*): internal/app/user/oidc_settings.go - ResolveOIDCConfig, AllowSignup default from AllowedEmailDomains
lazy discovery, 30 s backoff, never at boot: internal/app/user/oidc_service.go - oidcRetryBackoff ; cmd/evey/wire.go
account resolution (identity, verified-email link, sign-up, suffix, default libraries, admin never linked): oidc_service.go findOrCreateUser ; tests oidc_policy_test.go
admin routes GET/PUT /api/v1/admin/sso, POST /api/v1/admin/sso/test: internal/adapters/driving/http/sso_admin_handler.go
login error reasons: internal/adapters/driving/http/auth_handler.go (oidc_error=<reason>) ; i18n auth.login.ssoError.*
logout endSessionUrl: internal/adapters/driving/http/server.go
mobile flow: internal/adapters/driving/http/oidc_app_flow.go ; frontend/apps/mobile/app/oidc-callback.tsx
UI: frontend/apps/web/src/features/admin/serverConfig/SsoSettingsCard.tsx ; i18n admin.sso.* ; auth.login.signInWithSSO
-->
