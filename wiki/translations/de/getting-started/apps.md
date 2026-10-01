---
source_sha: fb9873d46e2323723ae54ede6035d085a424d9e6c047c3fb833962b8358dd857
reviewed: false
---
# Apps und Geräte

Jede App spricht mit Ihrem eigenen Server. Sie geben ihr einmal die
Serveradresse, melden sich an und wählen ein Profil. Wiedergabepositionen,
Verlauf und Favoriten gehören dem Server und folgen Ihnen daher von einem Gerät
zum anderen.

| App | Woher sie kommt | Anmeldung |
| --- | --- | --- |
| **Web** | Von Evey selbst ausgeliefert, unter der Serveradresse. Nichts zu installieren. | Benutzername und Passwort, oder Single Sign-on, falls eingerichtet. |
| **Android** (Telefon, Tablet) | Die Android-App. | Serveradresse, dann Benutzername und Passwort. |
| **Android TV** | Die Android-TV-App. | Serveradresse, dann **Schnellverbindung** (unten) oder Benutzername und Passwort. |
| **Chromecast** | Keine App: Streamen aus der Web-App in Chrome oder aus der Android-App. | Nutzt die Sitzung des streamenden Geräts. |

Der Admin-Bereich ist Teil der Web-App, für Administratorkonten.

## Schnellverbindung: einen Fernseher ohne Passworteingabe anmelden

Wählen Sie auf dem Fernseher **Schnellverbindung**. Er zeigt einen Code und
einen QR-Code. Öffnen Sie auf einem Telefon oder Computer, auf dem Sie bereits
angemeldet sind, **Einstellungen → Schnellverbindung** und geben Sie den Code
ein oder scannen Sie den QR-Code mit der Android-App. Der Fernseher ist dann
mit demselben Konto angemeldet.

## Was jede App mitbringt

- **Web.** Tastenkürzel im Player (`?` listet sie auf), Push-Benachrichtigungen
  im Browser (siehe [Benachrichtigungen](../notifications.md)) und der
  Admin-Bereich.
- **Android.**
  - **Downloads** zum Offline-Schauen. Hat der Administrator es eingeschaltet
    (`downloads.quality_choice`, siehe
    [Auslieferung und Downloads](../environment/playback-delivery.md)), bietet
    die App für einen Film, der zu groß für das Telefon ist, eine kleinere, vom
    Server vorbereitete Kopie an. Ein Download bleibt 30 Tage lang offline
    abspielbar, ohne Kontakt zum Server, und nur für Profile, die ihn sehen
    dürfen: Ein Kinderprofil kann einen Film oberhalb seiner Altersgrenze nicht
    öffnen, auch wenn ein anderes Profil ihn heruntergeladen hat.
  - **Streamen** auf einen Chromecast.
- **Android TV.** Eine Oberfläche, die für die Fernbedienung gebaut ist.

**Abspielen auf** sendet in den Playern von Web und Android das, was Sie gerade
schauen, an eine andere Evey-App, die mit demselben Konto angemeldet und
gerade geöffnet ist, und die Wiedergabe geht dort weiter.

## Welches Gerät was abspielt

Jede App meldet, was ihr Gerät dekodieren kann, und der Server entscheidet für
jeden Titel, ob er die Datei so sendet, wie sie ist, oder sie anpasst.
Derselbe Film kann unverändert an einen Fernseher gehen und für ein älteres
Telefon neu kodiert werden. Das Panel **Stream-Info** des Players zeigt die
Entscheidung und ihre Gründe: siehe
[Wie Evey die Wiedergabeart wählt](../playback/how-playback-is-decided.md).
Für das Streamen auf Chromecast gelten eigene Regeln: siehe
[Streamen auf Chromecast](../playback/casting.md).
