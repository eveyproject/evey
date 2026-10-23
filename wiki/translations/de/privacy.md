---
source_sha: e527433cfc41c2cba1cfcb773095634c496390297f79ac7e7b368fa36d27d047
reviewed: true
---
# Datenschutzerklärung { #privacy-policy }

Zuletzt aktualisiert: 2026-10-04.

Diese Erklärung gilt für die Evey-Apps für Android und Android TV.

Kurz gesagt: Das Evey-Projekt betreibt keinen Dienst, der Ihre Daten
empfängt. Es gibt kein Konto bei uns, keine Nutzungsstatistik, keine Werbung
und keine Absturzberichte an uns. Die Apps kommunizieren mit dem Evey-Server, den
Sie oder die Person, die Sie eingeladen hat, installiert haben und betreiben.

## Was die Apps senden, und an wen { #what-the-apps-send-and-to-whom }

**An Ihren Evey-Server, und an nichts anderes, das wir betreiben.** Die Apps
senden, was ein Mediaplayer zum Funktionieren braucht:

- Ihren Benutzernamen und Ihr Passwort, wenn Sie sich anmelden (oder den Code
  der Schnellverbindung), danach ein Sitzungstoken, einen Schlüssel, der Sie
  angemeldet hält;
- was Sie durchstöbern, suchen, abspielen, bewerten, als Favorit markieren
  oder anfragen, und wo Sie in der Wiedergabe stehen, damit Sie auf einem
  anderen Gerät weiterschauen können;
- den Namen des Geräts (so wie Android ihn meldet), damit Sie Ihre Geräte in
  **Abspielen auf** und in der Sitzungsliste des Servers auseinanderhalten
  können;
- technische Wiedergabeberichte: welche Audio- und Videoformate das Gerät
  abspielen kann und welche Fehler beim Abspielen aufgetreten sind, damit der
  Server einen Stream wählen kann, den das Gerät abspielen kann;
- ein Bild, nur wenn Sie eines als Profilbild wählen, und Dateien, die Sie als
  Administrator selbst auswählen (einen externen Untertitel, ein Plugin, eine
  Sicherung).

Wer diese Daten lesen kann, entscheidet die Person, die diesen Server
betreibt. Das Evey-Projekt hat keinen Zugriff darauf.

**An Dritte, nur in diesen Fällen:**

- **Poster und Album-Cover.** Manche Bilder in Medienanfragen und in der
  Musik kommen direkt von ihrer öffentlichen Quelle (The Movie Database, das
  Cover Art Archive, IMDb). Das Gerät lädt sie wie ein Webbrowser, wodurch
  diese Seite seine IP-Adresse sieht.
- **Streamen.** Das Streamen auf einen Chromecast läuft über Google Cast, das
  zu den Google-Play-Diensten auf Ihrem Handy gehört. Der Chromecast lädt die
  Evey-Empfängerseite von `cast.eveyproject.org`, die bei GitHub Pages liegt.
  Der Film oder die Musik selbst kommt direkt von Ihrem Server.
- **Sprachsuche auf Android TV.** Die Mikrofontaste übergibt an die
  Spracherkennung des Fernsehers (meist die von Google). Die App bekommt den
  erkannten Text, nie die Aufnahme.
- **Links, die Sie öffnen.** Ein Trailer, eine Seite von The Movie Database
  oder IMDb oder die Play-Store-Seite für ein Update öffnet sich in der
  passenden App oder im Browser, nach den Regeln dieses Dienstes.
- **Plugins.** Plugins installiert der Administrator des Servers. Ein Plugin
  kann Inhalte eines externen Dienstes anzeigen (zum Beispiel ein
  Wetter-Plugin).

## Was auf dem Gerät bleibt { #what-stays-on-the-device }

Die Serveradresse, Ihr Sitzungstoken und, wenn Sie „Angemeldet bleiben“ ankreuzen, Ihr Benutzername und Ihr Passwort (im verschlüsselten Speicher von Android, dem Android-Keystore), Ihre Einstellungen und die Filme und die Musik, die Sie zum Schauen
oder Hören ohne Internet herunterladen. Wenn Sie die App entfernen, wird all
das gelöscht. Das Entsperren per Fingerabdruck oder Gesicht, wenn Sie es
einschalten, übernimmt Android: Die App erfährt nur, ob es erfolgreich war.

## Berechtigungen { #permissions }

| Berechtigung | Wozu |
| --- | --- |
| Internet, WLAN-Status | Ihren Server erreichen, Ihr Heimnetz von unterwegs unterscheiden. |
| Kamera | Den QR-Code der Schnellverbindung scannen. Nichts wird aufgenommen oder gesendet. |
| Benachrichtigungen, Vordergrunddienst | Den Fortschritt von Wiedergabe und Downloads zeigen, bei ausgeschaltetem Bildschirm weiterspielen. |
| Biometrie | Optionale App-Sperre. |
| Audioeinstellungen ändern | Die Tonwiedergabe (Lautstärkeverwaltung) während eines Films anpassen. |
| Wachhalten (Wake Lock) | Verhindern, dass das Gerät während der Wiedergabe in den Ruhezustand geht. |
| Vibration | Haptische Rückmeldung in der App. |
| Externen Speicher lesen und schreiben (nur Android 12 und älter) | Ihre Offline-Downloads auf diesen Android-Versionen speichern und lesen. |

Die Handy-App hat keine Berechtigung für das Mikrofon.

## Diese Website { #this-website }

`eveyproject.org` liegt bei GitHub Pages, das die IP-Adresse der Besucher
sieht. Die Seite liefert ihre Schriften und Skripte selbst aus, der Browser
kontaktiert also keinen anderen Dienst. Sie setzt keine eigenen Cookies und
erhebt keine Besucherstatistik.

## Dienste von Drittanbietern { #third-party-services }

Dieses Produkt nutzt die TMDB-API, wird aber von TMDB weder unterstützt noch zertifiziert. Artwork und Metadaten stammen von [TMDB](https://www.themoviedb.org/).

## Kinder { #children }

Die Apps richten sich nicht an Kinder. Kinderprofile legt die Person an, die
den Server betreibt, und sie verwaltet diese Daten.

## Kontakt { #contact }

Fragen zu dieser Erklärung: [contact@eveyproject.org](mailto:contact@eveyproject.org).
Für Daten auf einem Evey-Server wenden Sie sich an die Person, die diesen
Server betreibt.

<!-- sources
no tracker: frontend/apps/mobile/package.json, frontend/apps/tv/package.json
permissions: frontend/apps/mobile/android/app/src/main/AndroidManifest.xml
device name: frontend/apps/mobile/src/hooks/useDeviceRegistration.ts
playback reports: frontend/packages/client/src/native/useAudioCapsBeacon.ts
outside posters: internal/app/mediarequest/service.go - trustedPosterHosts
cover art: frontend/packages/client/src/lib/artwork.ts - coverArtUri
cast receiver: docs/adr/0025-server-driven-cast.md
voice search: frontend/packages/voice-search/android/src/main/java/com/evey/voicesearch/EveyVoiceSearchModule.kt
store page: frontend/apps/tv/src/lib/appUpdate.ts - openStorePage
-->
