---
source_sha: dffa9823329d7253d7991f9037012a372bdeeb2765f9c4c68f0d73f04efc4dbc
reviewed: false
---
# Gestaltungsprinzipien

Die folgenden Prinzipien erklären, warum sich Evey so verhält, wie es sich
verhält. Jedes ist eine Vorgabe, der der Code folgt, mit den Seiten und
Entscheidungsprotokollen, in denen sie sichtbar wird. Wenn zwei davon in
Konflikt geraten, gewinnt das weiter oben stehende.

## 1. Zugang wird verweigert, wenn er sich nicht prüfen lässt

Wo eine Entscheidung Zugang gewährt, führt ein Fehler zur restriktivsten
Antwort. Ein Profil, dessen Kindersicherung sich nicht lesen lässt, erhält
keinen Inhalt; ein Plugin, dessen Code sich nicht analysieren lässt, wird nicht
installiert; der Fernzugriff folgt einer ausdrücklichen Wahl, sobald eine
getroffen wurde, und sagt laut, wenn er alle abweist. Fernzugriff und
Authentifizierung sind getrennt: Das Erste ist eine Entscheidung der
Bereitstellung, das Zweite schützt die Bibliothek.

Siehe [Benutzer, Profile und Kindersicherung](administration/users.md),
[Fernzugriff und HTTPS](administration/remote-access.md),
[ADR-0026](adr/0026-remote-access-inferred-from-a-published-url.md).

## 2. Code von Dritten wird eingesperrt, nicht für vertrauenswürdig gehalten

Ein Plugin erhält die Berechtigungen, die sein Manifest angibt, und nichts
sonst, wird vor der Installation analysiert, läuft in einer Sandbox ohne
direkten Netzwerkzugang und hält ein Token, das standardmäßig verweigert wird.
Was die Sandbox noch nicht verhindern kann, wird aufgeschrieben statt
unterstellt.

Siehe [Sicherheitsmodell und Plugin-Sandbox](SECURITY.md),
[ADR-0012](adr/0012-plugin-token-deny-by-default.md),
[ADR-0014](adr/0014-landlock-plugin-sandbox.md).

## 3. Das Original wird ausgeliefert, wann immer es geht

Das bevorzugte Ergebnis ist die Datei, wie sie ist, dann dieselben Spuren in
einem anderen Container, dann eine Transkodierung nur der Spuren, die es
brauchen. Qualität ist nichts, was der Zuschauer vor dem Abspielen wählen muss:
Der Server behält die Qualität der Quelle, außer eine echte Einschränkung (das
Gerät, die Verbindung, eine Grenze) verhindert es.

Siehe [Wie Evey die Wiedergabeart wählt](playback/how-playback-is-decided.md).

## 4. Entscheidungen beruhen auf Bewiesenem, nicht auf Namen

Die Fähigkeiten eines Players kommen aus dem, was sein Gerät meldet, nicht aus
einer Liste bekannter Geräte. Eine Grafikkarte wird für das genutzt, was sie bei
der Messung bestanden hat, nicht für das, was ihr Modell können sollte. Lässt
sich eine Fähigkeit nicht beweisen, nimmt der Server die vorsichtige Antwort:
Ein unbekannter Cast-Empfänger erhält die Fähigkeiten des ältesten, und
Pixelformate, die Player noch nicht melden, folgen einer festen
Kompatibilitätsregel. Zu viel anzunehmen ergibt einen schwarzen Bildschirm oder
falsche Farben; zu wenig anzunehmen kostet eine Transkodierung.

Siehe [Streamen auf Chromecast](playback/casting.md), [Hardwarebeschleunigung](administration/hardware-acceleration.md),
[ADR-0029](adr/0029-transcoding-routes-are-measured-not-assumed.md).

## 5. Eine Wiedergabe, eine Entscheidung

Eine Wiedergabe wird einmal entschieden, beim Start, und jedes Manifest und
jedes Segment der Sitzung wird aus dieser Entscheidung ausgeliefert. Der Player
erhält nie Segmente aus zwei Kodierungen, und was Stream-Info meldet, wird aus
demselben Plan abgeleitet, den der Encoder ausführt, von Tests in beide
Richtungen geprüft.

Siehe [ADR-0030](adr/0030-one-plan-one-decision.md),
[ADR-0024](adr/0024-playback-plan-resolver.md).

## 6. Der Server bemisst sich an der Maschine

Der Server misst, wie viel er transkodieren kann, und lässt nicht mehr zu: Eine
kleine Maschine spielt weniger transkodierte Streams ab, statt alle schlecht.
Die Wiedergabe kommt zuerst; Hintergrundarbeit pausiert, während jemand schaut.
Dasselbe Image läuft auf einem NAS und auf einem Server mit mehreren
Grafikkarten.

Siehe [Transkodierung und Serverkapazität](playback/transcoding.md),
[Hintergrundaufgaben und Serverlast](operations/background-work.md).

## 7. Wenige Einstellungen, jede an einem Ort

Was existieren muss, bevor der Server startet (Speicher, Geheimnisse, die
öffentliche Adresse), steht in der Compose-Datei; alles, was sich später ändern
kann, liegt im Admin-Bereich, in der Datenbank gespeichert, und gilt ohne
Neustart, wo nichts einen verlangt. Erkennung und Messung ersetzen Einstellungen,
wo immer sie können: Eine Installation ist eine Compose-Datei, Bibliotheken und
Benutzer.

Siehe [Installation](installation.md), [Konfigurationsreferenz](configuration.md),
[ADR-0028](adr/0028-install-is-one-compose-file.md).

## Entscheidungsprotokolle

Strukturelle Entscheidungen werden als Architecture Decision Records
festgehalten, mit dem Kontext, den abgewogenen Optionen und den Folgen. Sie
liegen beim Code: [docs/adr](adr/README.md).
