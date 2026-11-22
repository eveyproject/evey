---
source_sha: bc4e8000ca3b6f13e4b5d57a81bf41b5b314d18f1578b3eb3a7b6737c5d1b091
reviewed: true
---
# App-Updates von der Instanz { #app-updates-served-by-the-instance }

Die Apps für Telefon und TV laden ihr JavaScript von der Evey-Instanz, mit der
sie verbunden sind. Eine Korrektur am JavaScript der App erreicht ein Gerät
dann ohne Store-Veröffentlichung, sobald sein Server aktualisiert wird. Native
Änderungen brauchen weiterhin eine.

Eine App bewegt sich nur vorwärts: Sie nimmt ein Release, das neuer ist als
das, was sie ausführt, und behält gegenüber einem älteren Server ihren eigenen
Code.

## Wie die Teile zusammenspielen { #how-it-fits-together }

- **Das Image bringt die Bundles mit.** `make client-updates` baut ein
  signiertes Bundle pro App nach `client-updates/`, und das Image kopiert
  dieses Verzeichnis nach `/app/client-updates` (`EVEY_CLIENT_UPDATES_DIR`).
  Ein Image, das ohne diesen Schritt gebaut wurde, liefert kein Update aus,
  und die Apps behalten den Code, mit dem sie gebaut wurden.
- **Der Server gibt nur Bytes heraus.**
  `GET /api/v1/client-updates/{app}/manifest` und
  `.../assets/{platform}/{runtime}/{name}` sprechen das Expo-Updates-Protokoll
  (`internal/adapters/driving/http/client_update_handler.go`). Die Instanz
  hält keinen Schlüssel, baut kein Manifest und öffnet nur Dateien innerhalb
  dieses Verzeichnisses.
- **Die Apps prüfen.** Jedes Bundle ist vom Projekt signiert und wird mit dem
  Zertifikat des Schlüssels gesendet, der es signiert hat; die Apps prüfen
  dieses Zertifikat gegen die in sie einkompilierte Wurzel,
  `frontend/client-updates/certificate.pem`. Eine Instanz kann nur
  veröffentlichten Code verteilen; alles andere wird auf dem Gerät abgelehnt.
- **Die Apps gehen nie zurück.** Siehe „Release-Untergrenze“ weiter unten.
- **Die Apps folgen dem aktiven Server.** Sobald ein Server gewählt ist,
  richtet die App expo-updates auf ihn aus und lädt sein Bundle im Hintergrund
  herunter (`frontend/packages/client/src/native/clientUpdates.ts`). Das
  Bundle wird beim nächsten Start der App aktiv. Das Telefon bietet einen Neustart an,
  wenn es eine andere App-Version ist als die laufende, nie über dem Player; der
  Fernseher nie, ein Neuladen im laufenden Betrieb ist für eine kleine Box zu schwer.
  Ist kein Server gewählt, fragt die App niemanden.

## Version der nativen Laufzeit { #native-runtime-version }

Ein Bundle läuft nur auf dem nativen Build, für den es gemacht wurde. Jede App
deklariert eine `runtimeVersion` in ihrer `app.json` (`mobile-1`, `tv-1`), und
ein Bundle wird unter diesem Namen gespeichert und angefragt.

**Erhöhen Sie `runtimeVersion` in derselben Änderung wie alles Native**: ein
neues oder aktualisiertes natives Modul, ein Upgrade des Expo SDK oder von
React Native, ein Config-Plugin, das Android-Manifest oder der in
`offline-audio` eingebundene Go-Code. Die Telefon-App hält ihr Android-Projekt
in git, der Wert steht deshalb auch in
`frontend/apps/mobile/android/app/src/main/res/values/strings.xml`.

Eine Instanz hat kein Bundle für eine Laufzeit, mit der sie nicht gebaut
wurde: Ein älterer oder neuerer Store-Build behält einfach seinen eigenen
Code.

## Release-Untergrenze { #release-floor }

Eine Signatur sagt, wer ein Release veröffentlicht hat, nicht, dass es aktuell
ist. Ohne weitere Vorkehrung könnte jeder Server, oder jeder, der auf dem Weg
einer unverschlüsselten HTTP-Verbindung sitzt, einer App ein altes signiertes Release
geben und einen seither behobenen Fehler zurückbringen.

- Ein Release ist durch den Commit datiert, aus dem es gebaut wurde
  (`createdAt` im signierten Manifest, Committer-Zeit).
- Jedes Gerät führt eine Untergrenze: das neuere aus dem Commit, aus dem sein
  Store-Build gemacht wurde (von Gradle festgehalten), und dem neuesten
  ausgelieferten Release, das auf ihm angezeigt wurde. Die Untergrenze steigt
  nur.
- Die App lädt nichts unterhalb der Untergrenze herunter und startet das
  neueste Release, das sie auf dieser Höhe oder darüber hält, gleich von
  welchem Server es kam. Ein Server, der nichts oder etwas Älteres ausliefert,
  lässt die App bei dem, was sie hat.
- Der in die App eingebaute Code darf immer laufen. Er startet, wenn nichts
  Gespeichertes in Frage kommt. Auch er ist durch seinen Commit datiert, nicht
  durch den Zeitpunkt, zu dem die App kompiliert wurde, sodass das Release
  desselben Commits, und jedes späteren, ihm vorgezogen wird.
- Das JavaScript führt dieselbe Untergrenze auf seiner Seite und weigert sich,
  darunter zu laufen.

Was daraus folgt:

- Gegenüber einem älteren Server führt eine App neueres JavaScript aus, als
  dieser Server mitgebracht hat, so wie es ein Store-Build tut. Die Server-API
  muss mit neueren Clients kompatibel bleiben.
- Wenn der Wächter ein Release verwirft, das bereits angezeigt wurde, liegt
  das davor meist unter der Untergrenze: Die App führt ihren eingebauten Code
  aus, bis die Instanz ein neueres Release veröffentlicht.
- Ein Build, dessen Untergrenze nicht gelesen werden kann, nimmt gar kein
  Update an.
- Zwei Builds eines Commits sind zwei Releases mit fast demselben Datum. Die
  Apps speichern ein Release pro Datum: Fallen beide Daten zusammen, wird das
  zweite nicht gespeichert, und die App behält, was sie ausführt. Bauen Sie
  einmal pro Release.

Nichts Unsigniertes steuert das: Die Apps ignorieren Manifest-Filter, vom
Server definierte Header, den Extensions-Teil und Direktiven, und senden den
Servern weder eine Installationskennung noch den Text eines früheren Absturzes.

## Wenn ein Update sich falsch verhält { #when-an-update-misbehaves }

- Ein Bundle, das vor seiner ersten Anzeige abstürzt, wird von expo-updates
  verworfen, das auf das vorherige oder auf den in die App eingebauten Code
  zurückfällt.
- Ein Bundle, das angezeigt wird und dann bei jedem Start stirbt, fängt der
  eigene Wächter der App ab (`clientUpdateBootGuard`): Nach drei Starts in
  Folge, die endeten, ohne sich zu stabilisieren, verlangt der nächste
  denselben Rückfall.
- Ein verworfenes Bundle wird nicht erneut versucht. Die Instanz muss ein
  anderes veröffentlichen.
- Wenn das verworfene Bundle nichts zum Starten übrig lässt (das davor liegt
  unter der Untergrenze), startet die App von selbst neu mit ihrem
  eingebauten Code.

## Der expo-updates-Patch { #the-expo-updates-patch }

`frontend/patches/expo-updates@*.patch` enthält, was der Entwurf braucht und
wofür expo-updates keine Einstellung hat:

- Asset-Adressen in einem Manifest dürfen relativ sein, aufgelöst gegen die
  Adresse des Manifests, sodass ein signiertes Manifest auf jeder Instanz
  gültig ist;
- die Rückfall-Anforderung des Wächters verwirft ein Update auch dann, wenn es
  zuvor erfolgreich gestartet ist;
- wenn dadurch nichts zum Starten bleibt, wird die App neu gestartet, statt
  abzustürzen;
- die Release-Untergrenze und das Starten nach Release statt nach
  Serveradresse;
- der in die App eingebaute Code ist durch seinen Commit datiert;
- ein bereits gespeichertes Release wird übernommen und nicht erneut
  heruntergeladen, wenn eine andere Adresse es ausliefert (dieselbe Instanz
  aus dem lokalen Netz und von außen);
- jedes Asset muss seinen Hash im signierten Manifest tragen;
- Zertifikate werden zum späteren der beiden Zeitpunkte geprüft, Geräteuhr
  oder Untergrenze, und
  ein Release kann nicht nach dem Ablauf seines Signaturzertifikats datiert
  sein;
- die oben genannten unsignierten Eingaben und identifizierenden Header werden
  entfernt.

Erstellen Sie den Patch neu, wenn expo-updates aktualisiert wird; die
Abhängigkeit ist auf die exakte gepatchte Version festgelegt.

## Was ein Build haben muss { #what-a-build-must-have }

`frontend/scripts/check-expo-updates-patch.mjs` prüft für beide Apps: dass der
Patch in den Quellen steckt, die Android kompiliert, dass das Zertifikat und
seine Metadaten konfiguriert sind (ohne Zertifikat prüft expo-updates nichts),
dass das Zertifikat eine Wurzel ist und die Apps über sie prüfen, dass der
Build seine Release-Untergrenze festhält, dass keine Einstellung erweitert,
was die Apps annehmen, und dass die Version der nativen Laufzeit zu `app.json`
passt. Es läuft vor jedem Gradle-Build der Apps und im Gate auf jedem Branch,
der die Abhängigkeiten, ihre Patches, das Zertifikat oder die native Seite
einer App berührt.

## Schlüssel { #keys }

Zwei Schlüssel, erzeugt von `frontend/scripts/client-update-certificate.sh`:

- **Die Wurzel** (`root <dir>`), einmalig. Ihr Zertifikat ist
  `frontend/client-updates/certificate.pem`, einkompiliert in jeden
  Store-Build; ihr Schlüssel ist mit einer Passphrase verschlüsselt und bleibt
  offline, mit einer Sicherung. Er kommt nie auf eine Build-Maschine. Ihn zu
  verlieren oder ersetzen zu müssen bedeutet einen neuen Store-Build beider
  Apps, bevor wieder ein Update ausgeliefert werden kann.
- **Das Signaturzertifikat** (`leaf <dir> [days]`), standardmäßig ein Jahr,
  von der Wurzel ausgestellt. Sein Schlüssel ist das, was ein Release-Build
  hält.

Erneuern Sie das Signaturzertifikat, bevor es abläuft: Das
Veröffentlichungswerkzeug lehnt eines mit weniger als 30 verbleibenden Tagen
ab, und Geräte lehnen ein Release ab, sobald sein Zertifikat abgelaufen ist,
sodass ein Image sein Bundle ein Jahr nach der Ausstellung seines Zertifikats
nicht mehr herausgibt (Geräte behalten, was sie bereits halten). Es gibt keinen
Widerruf: Ein durchgesickerter Signaturschlüssel gilt, bis sein Zertifikat
abläuft, und nur für Releases, die davor datiert sind. Die Wurzel zu ersetzen
ist der einzige Weg, früher einen Schnitt zu machen.

## Ein Release veröffentlichen { #releasing }

1. `EVEY_CLIENT_UPDATE_KEY_FILE=<signing key> EVEY_CLIENT_UPDATE_CERT_FILE=<signing certificate> make client-updates`
   einmal, auf dem Release-Commit mit sauberem Arbeitsbaum, vor dem Bau des
   Images. Der Hermes-Bytecode ist nicht reproduzierbar: Zwei Läufe ergeben
   zwei verschiedene Updates, also muss jede Architektur eines Releases
   dieselbe Ausgabe mitbringen. Das Werkzeug weigert sich, über ein neueres
   Release zu veröffentlichen.
2. Bauen Sie das Image. Der Bau scheitert an einem Bundle, das unsigniert ist,
   aus einem anderen Commit gebaut wurde oder für eine Laufzeit bestimmt ist,
   die die Apps nicht mehr deklarieren.

Die CI tut das nur für Release-Tags (`v*.*.*`), in zwei Jobs
(`.github/workflows/ci.yml`):

- einer baut die Bundles unsigniert, mit allen Abhängigkeiten der Apps und
  ohne Schlüssel;
- einer signiert sie, mit dem Schlüssel und nichts als einem Checkout:
  `frontend/scripts/sign-client-update.mjs` lädt keine Abhängigkeit und lehnt
  ein Bundle ab, das nicht das des getaggten Commits ist, für die Laufzeiten,
  die die Apps bei diesem Commit deklarieren, datiert durch diesen Commit, mit
  Dateien, die dem entsprechen, was das Manifest sagt.

Der Image-Bau jeder Architektur nimmt dann dieselbe signierte Ausgabe. Ein
Tag, dessen Bundles nicht signiert werden können, scheitert, es wird nicht
ohne sie ausgeliefert.

Was die Signatur nicht abdeckt: Der Bau-Job führt die Abhängigkeiten der Apps
aus, und eine von ihnen könnte das JavaScript verändern, bevor es signiert
wird. Der Signatur-Job prüft die Form, den Commit und das Datum eines Bundles,
nicht, was sein Bytecode tut. Die Verteidigung sind das eingefrorene Lockfile
und die Durchsicht jeder Anhebung einer Abhängigkeit.

Images, die aus einem Branch oder von Hand gebaut werden, bringen kein Bundle
mit. Ein für einen Branch signiertes Bundle würde von jeder veröffentlichten
App angenommen, von jedem Server, und würde ihre Untergrenze über die
Releases anheben.

Der Signatur-Job läuft in der GitHub-Umgebung `client-update-signing`, die die
beiden Secrets `EVEY_CLIENT_UPDATE_SIGNING_KEY` und
`EVEY_CLIENT_UPDATE_SIGNING_CERT` hält (beides PEM-Dateien, vollständig) und
nur `v*.*.*`-Tags hereinlässt: Ein Branch, der den Workflow ändert, kann sie
nicht lesen. Ersetzen Sie beide, wenn das Signaturzertifikat erneuert wird.

Kein privater Schlüssel gelangt je in das Repository oder in ein Image, und
Schlüssel werden immer nur als Dateipfad übergeben.

<!-- sources
routes and what the server serves: internal/adapters/driving/http/client_update_handler.go ; internal/adapters/driving/http/client_update_handler_test.go ; internal/config/config.go - EVEY_CLIENT_UPDATES_DIR
following the active server, restart prompt, launch guard: frontend/packages/client/src/native/clientUpdates.ts - followClientUpdateSource, clientUpdateBootGuard, clientUpdateMarkSettled ; frontend/packages/client/src/native/clientUpdates.test.ts ; i18n common.clientUpdate.*
floor, launch by release, certificates, dropped inputs: frontend/patches - the expo-updates patch
build checks: frontend/scripts/check-expo-updates-patch.mjs ; Dockerfile
keys and publication: frontend/scripts/client-update-certificate.sh ; frontend/scripts/build-client-update.mjs ; frontend/scripts/sign-client-update.mjs ; .github/workflows/ci.yml (client-updates-build, client-updates-sign)
-->
