---
source_sha: 00f9d5811af70059e3ff6fc49a0defb49e3801f0005d12d5b22033634e960f33
reviewed: false
---
# OIDC / SSO einrichten

Evey unterstützt Single Sign-on über OpenID Connect: Keycloak, Authentik,
Authelia, Google oder jeden anderen OIDC-Anbieter. Die Anmeldeseite der Web- und
Android-App zeigt dann **Mit SSO anmelden** neben dem Passwortformular.

## 1. Evey beim Anbieter registrieren

Legen Sie beim Anbieter einen Client (oder eine Anwendung) an:

| Feld | Wert |
|-------|-------|
| Client ID | `evey` oder ein beliebiger Name |
| Client-Secret | eines erzeugen (Pflicht) |
| Weiterleitungs-URI | `https://<Ihre Adresse>/api/v1/auth/oidc/callback` |
| Weiterleitungs-URI nach der Abmeldung | `https://<Ihre Adresse>/login` |
| Grant-Typ | Authorization Code |
| Scopes | `openid email profile` |

## 2. Evey konfigurieren

```env title=".env"
EVEY_OIDC_ENABLED=true
EVEY_OIDC_ISSUER=https://auth.example.com/realms/main
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=your-client-secret
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=example.com,company.org
```

`EVEY_OIDC_ISSUER` ist die Basis-URL des Anbieters, ohne
`/.well-known/openid-configuration`. Die vollständige Liste, einschließlich
`EVEY_OIDC_REDIRECT_URL`, steht unter [Konten und Anmeldung](environment/accounts.md).

## 3. Neu starten

```bash
docker compose up -d
```

Der Server kontaktiert den Anbieter beim Start. Ist der Aussteller nicht
erreichbar oder das Client-Secret leer, **startet der Server nicht**: Die Logs
nennen die Ursache.

## Wer sich anmelden kann

Die erste SSO-Anmeldung legt ein Evey-Konto an, ohne Passwort und mit einem
Profil. Sein Benutzername ist der `preferred_username` des Anbieters, sonst der
Teil der E-Mail vor `@`. Das Konto ist ein normaler Benutzer und sieht wie
jedes Konto ohne Einschränkungen alle Bibliotheken: Schränken Sie es danach
unter **Admin → Benutzer** ein (siehe [Benutzer](administration/users.md)).

**Begrenzen Sie Registrierungen mit `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS`.** Leer
bekommt jeder, der ein Konto beim Anbieter hat, eines auf Ihrem Server, was bei
einem öffentlichen Anbieter wie Google jeden bedeutet. Mit Domains werden nur
diese Adressen angenommen, und der Anbieter muss die Adresse als verifiziert
kennzeichnen.

Ein Konto wird am Anbieter und seiner Benutzerkennung erkannt, nicht an der
E-Mail. Wer bereits ein lokales Konto mit derselben E-Mail oder demselben
Benutzernamen hat, kann kein SSO-Konto anlegen: Die Anmeldung scheitert mit
„SSO-Anmeldung fehlgeschlagen". Ändern Sie beim Anbieter die Adresse oder den
Benutzernamen oder entfernen Sie zuerst das lokale Konto.

Ein deaktiviertes Konto wird bei der Anmeldung wie üblich abgewiesen.

## Abmelden

Wenn der Anbieter einen Abmelde-Endpunkt veröffentlicht, beendet die Abmeldung
bei Evey auch die Sitzung beim Anbieter und kehrt dann zur Anmeldeseite von
Evey zurück.
