---
source_sha: 4a1aad8cb9228c4a82e7b9a0a3cbe3ee43a07f46b3a7109eb678dd7331ba9fd2
reviewed: false
---
# OIDC / SSO einrichten

Evey unterstützt Single Sign-on über OpenID Connect: Keycloak, Authentik,
Authelia, Google oder jeden anderen OIDC-Anbieter. Die Anmeldeseite der Web- und
Android-App zeigt dann **Mit SSO anmelden** neben dem Passwortformular.

## 1. Evey beim Anbieter registrieren

Legen Sie beim Anbieter einen Client (oder eine Anwendung) mit dem Grant-Typ
Authorization Code, den Scopes `openid email profile`, einem Client-Secret und
den beiden Adressen an, die die Karte **Single Sign-on (SSO)** unter **Diese
Adressen beim Anbieter eintragen** zeigt:

| Feld | Wert |
|-------|-------|
| Weiterleitungs-URI | `https://<Ihre Adresse>/api/v1/auth/oidc/callback` |
| Weiterleitungs-URI nach der Abmeldung | `https://<Ihre Adresse>/login` |

Beide werden aus der öffentlichen Adresse des Servers gebildet (**Admin →
Fernzugriff**): Legen Sie diese zuerst fest, wenn Evey unter einer anderen
Adresse erreicht wird.

## 2. Evey konfigurieren

**Admin → Serverkonfiguration → Single Sign-on (SSO)**:

| Feld | Wirkung |
| --- | --- |
| **Single Sign-on aktivieren** | Zeigt die SSO-Schaltfläche auf den Anmeldeseiten. |
| **Issuer-URL** | Die Adresse des Anbieters, ohne `/.well-known/openid-configuration`. |
| **Client-ID**, **Client-Secret** | Der oben registrierte Client. Das Secret bleibt auf dem Server und wird nie wieder angezeigt; die Punkte stehen lassen behält es. |
| **Zugelassene E-Mail-Domains** | Kommagetrennt. Sind sie gesetzt, werden nur diese Adressen angenommen, und der Anbieter muss die Adresse bestätigen. |
| **Beschriftung der Schaltfläche** | Optional, ersetzt „Mit SSO anmelden". |
| **Mit bestehenden Konten über bestätigte E-Mail verknüpfen** | Standardmäßig an. Siehe unten. |
| **Konten für neue SSO-Benutzer anlegen** | Aus, außer wenn E-Mail-Domains zugelassen sind. Siehe unten. |
| **Bibliothekszugriff neuer SSO-Konten** | **Alle Bibliotheken** oder **Nur die ausgewählten Bibliotheken**. |

**Verbindung testen** prüft, ob der Issuer antwortet. **SSO-Einstellungen
speichern** gilt sofort, ohne Neustart. Die Zeile **Status** zeigt, ob der
Anbieter erreichbar ist, und andernfalls den letzten Fehler.

Die Variablen `EVEY_OIDC_*` unter [Konten und Anmeldung](environment/accounts.md)
sind der Rückfall: Die Karte sagt, wenn ihre Werte von dort kommen, und
Speichern übernimmt.

Der Server wartet nie auf den Anbieter: Er startet auch, wenn der Anbieter nicht
läuft, die SSO-Schaltfläche bleibt verborgen, solange der Anbieter nicht
erreichbar ist, und die Anmeldung per Passwort funktioniert weiter. Der Anbieter
wird beim nächsten Anmeldeversuch erneut kontaktiert, höchstens alle 30
Sekunden.

## Wer sich anmelden kann

1. Ein Konto, das sich schon mit diesem Anbieter angemeldet hat, meldet sich
   wieder an, sofern es nicht deaktiviert ist.
2. Sonst wird bei eingeschaltetem **Mit bestehenden Konten über bestätigte
   E-Mail verknüpfen** ein bestehendes Konto mit derselben E-Mail mit dem
   Anbieter verknüpft und angemeldet, aber nur, wenn der Anbieter die Adresse
   bestätigt. Das Konto behält sein Passwort.
3. Sonst wird bei eingeschaltetem **Konten für neue SSO-Benutzer anlegen** ein
   neues Konto angelegt, mit einem Profil und dem in der Karte gewählten
   Bibliothekszugriff. Sein Benutzername ist der `preferred_username` des
   Anbieters, sonst der Teil der E-Mail vor `@`; ein bereits vergebener Name
   erhält ein kurzes Suffix.

**Konten für neue SSO-Benutzer anlegen** ist standardmäßig aus, wenn keine
E-Mail-Domain zugelassen ist: Bei einem öffentlichen Anbieter wie Google bekäme
sonst jeder mit einem Konto dort eines auf Ihrem Server. Ist es aus, können sich
nur bestehende Konten per SSO anmelden.

Wird eine Anmeldung abgewiesen, nennt die Anmeldeseite den Grund: Die E-Mail-
Domain ist nicht zugelassen, kein Konto passt und das Anlegen ist aus, das Konto
ist deaktiviert, das Konto existiert, konnte aber nicht verknüpft werden, oder
der Anbieter ist nicht verfügbar.

## Abmelden

Wenn der Anbieter einen Abmelde-Endpunkt veröffentlicht, beendet die Abmeldung
bei Evey nach einer SSO-Anmeldung auch die Sitzung beim Anbieter und kehrt dann
zur Anmeldeseite von Evey zurück. Die Karte zeigt, ob der Anbieter das
unterstützt.
