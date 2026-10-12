---
source_sha: be9fbbf14af7dccd420515b33353e89b5ac1154b0fec5dc22468319328e97a6c
reviewed: true
---
# Gestaltungsprinzipien

Sieben Vorgaben, an die sich der Code hält, jede mit den Seiten und
Entscheidungsprotokollen, in denen sie sichtbar wird. Lesen Sie diese Seite,
um zu verstehen, warum Evey sich so verhält, bevor Sie lesen, wie es gebaut
ist ([Architektur](ARCHITECTURE.md)) oder warum eine grundlegende Wahl
getroffen wurde ([Entscheidungsprotokolle](adr/README.md)). Wenn sich zwei
Prinzipien widersprechen, gewinnt das, das weiter oben in der Liste steht.

| Prinzip | Wo es sichtbar wird | Entschieden durch |
| --- | --- | --- |
| 1. Zugang wird verweigert, wenn er sich nicht prüfen lässt | Kindersicherung, Plugin-Installation, Fernzugriff | [ADR-0026](adr/0026-remote-access-inferred-from-a-published-url.md) |
| 2. Fremder Code wird eingesperrt, nicht vertraut | Plugin-Sandbox | [ADR-0012](adr/0012-plugin-token-deny-by-default.md), [ADR-0014](adr/0014-landlock-plugin-sandbox.md) |
| 3. Das Original wird geliefert, wann immer es geht | Wiedergabeentscheidung | [ADR-0030](adr/0030-one-plan-one-decision.md) |
| 4. Entscheidungen beruhen auf Nachweisen, nicht auf Namen | Streamen, Hardware-Beschleunigung | [ADR-0029](adr/0029-transcoding-routes-are-measured-not-assumed.md) |
| 5. Eine Wiedergabe, eine Entscheidung | Wiedergabeplan, Stream-Info | [ADR-0030](adr/0030-one-plan-one-decision.md), [ADR-0024](adr/0024-playback-plan-resolver.md) |
| 6. Der Server richtet sich nach der Maschine | Kalibrierung, Hintergrundarbeiten | - |
| 7. Wenige Einstellungen, jede an einem Ort | Installation, Verwaltungsbereich | [ADR-0028](adr/0028-install-is-one-compose-file.md) |

## 1. Zugang wird verweigert, wenn er sich nicht prüfen lässt

Wo eine Entscheidung Zugang gewährt, führt ein Fehler zur strengsten
Antwort. Ein Profil, dessen Einstellungen zur Kindersicherung sich nicht
lesen lassen, bekommt keine Inhalte. Ein Plugin, dessen Code sich nicht
analysieren lässt, wird nicht installiert. Der Fernzugriff folgt einer
ausdrücklichen Wahl, sobald eine getroffen wurde, und meldet deutlich, wenn
er alle abweist. Fernzugriff und Authentifizierung sind getrennt: Das eine
ist eine Frage der Bereitstellung, das andere schützt die Bibliothek.

Siehe [Benutzer, Profile und Kindersicherung](administration/users.md),
[Fernzugriff und HTTPS](administration/remote-access.md),
[ADR-0026](adr/0026-remote-access-inferred-from-a-published-url.md).

## 2. Fremder Code wird eingesperrt, nicht vertraut

Ein Plugin bekommt die Berechtigungen, die sein Manifest angibt, und sonst
nichts. Es wird vor der Installation analysiert, läuft in einer Sandbox ohne
direkten Netzzugang und hält einen Token, der standardmäßig abgelehnt wird.
Was die Sandbox trotzdem nicht verhindern kann, steht ausdrücklich
geschrieben, statt nur angedeutet zu werden.

Siehe [Plugin-Sandbox und Bedrohungsmodell](SECURITY.md),
[ADR-0012](adr/0012-plugin-token-deny-by-default.md),
[ADR-0014](adr/0014-landlock-plugin-sandbox.md).

## 3. Das Original wird geliefert, wann immer es geht

Am liebsten wird die Datei so geliefert, wie sie ist. Danach kommen
dieselben Spuren in einem anderen Container, danach eine Transkodierung nur
der Spuren, die es brauchen. Die Qualität muss der Zuschauer nicht vor dem
Abspielen wählen: Der Server behält die Qualität der Quelle, außer eine
echte Einschränkung (das Gerät, die Verbindung, ein Limit) verhindert es.

Siehe [Wie Evey die Wiedergabeart wählt](playback/how-playback-is-decided.md).

## 4. Entscheidungen beruhen auf Nachweisen, nicht auf Namen

Was ein Player kann, ergibt sich aus dem, was sein Gerät meldet, nicht aus
einer Liste bekannter Geräte. Eine Grafikkarte wird für das genutzt, was sie
bei der Messung bestanden hat, nicht für das, was ihr Modell können sollte.
Lässt sich eine Fähigkeit nicht nachweisen, nimmt der Server die vorsichtige
Antwort: Ein unbekannter Cast-Empfänger bekommt die Fähigkeiten des ältesten
Modells, und Pixelformate, die Player noch nicht melden, folgen einer festen
Kompatibilitätsregel. Wer zu viel annimmt, bekommt ein schwarzes Bild oder
falsche Farben. Wer zu wenig annimmt, bezahlt mit einer Transkodierung.

Siehe [Streamen auf Chromecast](playback/casting.md),
[Hardware-Beschleunigung](administration/hardware-acceleration.md),
[ADR-0029](adr/0029-transcoding-routes-are-measured-not-assumed.md).

## 5. Eine Wiedergabe, eine Entscheidung

Eine Wiedergabe wird einmal entschieden, wenn sie beginnt, und jedes
Manifest und jedes Segment der Sitzung wird nach dieser Entscheidung
ausgeliefert. Der Player bekommt nie Segmente aus zwei Kodierungen. Was
Stream-Info anzeigt, wird aus demselben Auslieferungsplan abgeleitet, den
auch die Wiedergabekette benutzt, und Tests prüfen das in beide Richtungen.

Siehe [Ein Video ausliefern - der HLS-Wiedergabeweg](playback-serving.md),
[ADR-0030](adr/0030-one-plan-one-decision.md),
[ADR-0024](adr/0024-playback-plan-resolver.md).

## 6. Der Server richtet sich nach der Maschine

Der Server misst, wie viel er transkodieren kann, und nimmt nicht mehr an:
Eine kleine Maschine spielt weniger transkodierte Streams ab, statt alle
schlecht abzuspielen. Die Wiedergabe hat Vorrang. Hintergrundarbeiten
pausieren, solange jemand schaut. Dasselbe Image läuft auf einem NAS und auf
einem Server mit mehreren Grafikkarten.

Siehe [Transkodierung und Serverkapazität](playback/transcoding.md),
[Hintergrundarbeiten und Serverlast](operations/background-work.md).

## 7. Wenige Einstellungen, jede an einem Ort

Was vor dem Serverstart da sein muss (Speicher, Geheimnisse, die öffentliche
Adresse), steht in der Compose-Datei. Alles, was sich später ändern kann,
liegt im Verwaltungsbereich, wird in der Datenbank gespeichert und wirkt
ohne Neustart, wo nichts einen verlangt. Erkennung und Messung ersetzen
Einstellungen, wo immer es geht: Eine Installation besteht aus einer
Compose-Datei, Bibliotheken und Benutzern.

Siehe [Installation](installation.md), [Konfigurationsreferenz](configuration.md),
[ADR-0028](adr/0028-install-is-one-compose-file.md).

## Entscheidungsprotokolle

Grundlegende Entscheidungen werden als Architektur-Entscheidungsprotokolle
festgehalten, mit dem Kontext, den abgewogenen Möglichkeiten und den Folgen.
Sie liegen beim Code: [docs/adr](adr/README.md).

<!-- sources
fail closed: internal/adapters/driving/http/parental.go - BlockedByParental ; internal/app/plugin/install.go
contained plugins: docs/SECURITY.md ; docs/adr/0012-plugin-token-deny-by-default.md ; docs/adr/0014-landlock-plugin-sandbox.md
original first, one decision: internal/app/transcoding/decision.go ; internal/app/transcoding/route_tonemap_report_test.go ; docs/adr/0030-one-plan-one-decision.md
proved capabilities: frontend/packages/media-caps/src/castUrl.ts - castProfileFor ; docs/adr/0029-transcoding-routes-are-measured-not-assumed.md
sizing: internal/app/transcoding/calibration_service.go ; i18n admin.serverConfig.cpuBudget.globalDesc
-->
