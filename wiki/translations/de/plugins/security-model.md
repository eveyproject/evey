---
source_sha: e10872a44bc7a4594b78350969813e3dffa0a11bd28a8acc97dcfb617e5b019b
reviewed: true
---
# Berechtigungen und Slots { #permissions-and-slots }

Für Plugin-Autoren: was ein Manifest angeben darf, was der Installer prüft
und welche Regeln für ein Plugin zur Laufzeit gelten. Der Katalog unten ist
die Liste, aus der Sie wählen. Wie der Server ein Plugin isoliert und was
diese Isolierung nicht verhindern kann, steht in
[Plugin-Sandbox und Bedrohungsmodell](../SECURITY.md).

## Was bei der Installation geprüft wird { #what-is-checked-at-install }

Ein Plugin wird bei der Installation abgelehnt, wenn:

- `kind` fehlt oder weder `plugin` noch `theme` ist;
- einem `kind: plugin` die Angabe `language` oder `entrypoint` fehlt, oder
  ein `kind: theme` sie angibt oder `permissions` oder `capabilities`
  angibt;
- eine Berechtigung oder eine Capability unbekannt ist;
- ein Frontend-Modul einen Slot außerhalb der [Liste der Slots](#slot-whitelist)
  anspricht;
- ein Eintrag in `external_fetch` gegen die
  [Regeln für Abrufe](#external-fetch-rules) verstößt oder `network` ohne
  jeden Eintrag angegeben ist;
- das ZIP gegen die [Grenzen beim Entpacken](../SECURITY.md#zip-extraction)
  verstößt;
- die statische Analyse ein kritisches Muster findet oder eine Warnung, die
  der Installer wie ein solches behandelt (unsichere Deserialisierung,
  Verschleierung). Kompilierte Binärdateien, eingebettete Archive und nativer
  Code werden abgelehnt, außer das Manifest gibt `native-binary` an.

Kommentare werden vor der Analyse entfernt, eine auskommentierte Zeile wird
also nicht gemeldet. Die Kategorien stehen in
[Plugin-Sandbox und Bedrohungsmodell](../SECURITY.md#static-analysis).

Vor der Installation sieht der Administrator die angegebenen
Berechtigungen und Fähigkeiten und was die Analyse gefunden hat; Slots, Hosts
und Signatur zeigen sich beim Update und im Sicherheits-Dashboard. Nichts, was
ein Plugin angibt, wird stillschweigend gewährt.

## Unter welchen Bedingungen das Plugin läuft { #what-the-plugin-runs-under }

Egal, was das Manifest angibt:

- das Plugin sieht sein eigenes Verzeichnis, nicht die Daten des Servers;
- es hat keinen direkten Netzzugang: Ein Plugin mit der Berechtigung
  `network` erreicht die Hosts seiner Liste `external_fetch` über den Aufruf
  `Fetch` des Hosts, und sonst nichts;
- ohne `subprocess` kann es keine Kindprozesse starten (Threads
  funktionieren);
- es läuft unter Ressourcenlimits, die der Administrator anpassen kann:
  standardmäßig 256 MiB Arbeitsspeicher, 64 Prozesse und Threads, 100 MiB
  Speicherplatz für sein Verzeichnis und eine niedrigere CPU-Priorität.

Signieren ist während der Entwicklung freiwillig. Wie Sie ein Plugin
signieren und welche Signaturregel ein Server durchsetzen kann:
[Ein Plugin schreiben](dev-setup.md#7-sign-your-plugin).

## Katalog der Berechtigungen { #permissions-catalog }

Eine Berechtigung wird unter `permissions` angegeben. Jeder andere Wert
führt zur Ablehnung der Installation. Webhooks empfangen (`webhook.receiver`)
und Metadaten liefern (`metadata.provider`) sind Capabilities und werden
unter `capabilities` angegeben: siehe
[Plugin-API-Referenz](api-reference.md#capabilities).

| Berechtigung | Was sie gewährt |
|---|---|
| `network` | Das Backend darf `Fetch` aufrufen. Nur zusammen mit einer nicht leeren Liste `external_fetch`; ohne sie wird die Installation abgelehnt. Der Prozess selbst hat weiterhin keinen Socket. |
| `subprocess` | Das Backend darf Kindprozesse starten. Ohne diese Berechtigung lehnt der Kernel-Filter des Launchers `fork`, `vfork` und `clone` ohne `CLONE_THREAD` ab, und die seccomp-Zwischenschicht lehnt `execve` für interpretierte Plugins ab. Threads funktionieren in beiden Fällen. Der Administrator sieht eine eigene Warnung. |
| `native-binary` | Das Paket darf `.so`, `.dll`, `.exe`, `.wasm`, `.jar`, eingebettete Archive oder nativen Code enthalten, den die Analyse nicht lesen kann. Der Administrator sieht Code, den die Analyse nicht prüfen kann, und muss ihn akzeptieren. Pflicht für ein `go`-Plugin. |
| `filesystem` | Angekündigte Absicht, das Dateisystem über das Verzeichnis des Plugins hinaus zu nutzen. Wird festgehalten und dem Administrator angezeigt; die Sandbox gewährt keinen Dateizugriff außerhalb des Plugin-Verzeichnisses, egal was angegeben ist. |
| `media.read` | `GetMediaItem` und `ListMediaItems` auf dem Host: der vollständige Datensatz des Mediums. |
| `media.write` | Für die Aufrufe zum Schreiben von Metadaten reserviert, heute nicht bedient. |
| `api.library` | Dieselben Leseaufrufe für Medien wie `media.read`. |
| `api.user` | Für `GetUserPreferences` reserviert, heute nicht bedient. |
| `api.metadata` | Für `UpdateMetadata` reserviert, heute nicht bedient. |
| `notification.sender` | Für `SendNotification` reserviert, heute nicht bedient. |
| `public` | Die Oberfläche darf eine sichere Projektion der Medien lesen (id, Titel, Typ, Jahr, Inhaltsangabe, Poster), nie Pfade oder Streams. Siehe [`evey.media.*`](api-reference.md#eveymedia-permission-public). |
| `prefs` | Die Oberfläche darf die Einstellungen des aktuellen Benutzers im eigenen Namensraum des Plugins lesen und schreiben. Siehe [`evey.prefs.*`](api-reference.md#eveyprefs-permission-prefs). |
| `hardware.gpu` | Der Prozess bekommt den DRM-Render-Knoten des Hosts (den echten `renderD1xx`, nie einen Card-Knoten). Das ist eine Angriffsfläche auf den Kernel-Treiber und eine GPU-Speichergrenze, die er mit dem Transcoder teilt: Eine hängende GPU kann die Transkodierung blockieren. Ohne diese Berechtigung sind GPU-Knoten ganz aus der Landlock-Freigabeliste ausgeschlossen. Mit ihr fügt der Launcher eine `IOCTL_DEV`-Regel hinzu, die nur für diese eine Datei gilt (Landlock ABI 5 oder neuer; bei älteren Kerneln erlaubt schon der Lese- und Schreibzugriff, Befehle abzuschicken). Das Sicherheits-Dashboard zeigt eine kritische Warnung, sobald das Plugin installiert ist. Gewähren Sie sie nur Plugins, die Sie auch ohne Sandbox laufen lassen würden. |

## Regeln für externe Abrufe { #external-fetch-rules }

Angegeben in `external_fetch: [...]` und bei der Installation geprüft
(`validateExternalFetch`). Alles oder nichts: Ein ungültiger Eintrag lehnt
die ganze Installation ab.

Ein Eintrag ist ein **Hostname**. Ein vorangestelltes Schema und ein
abschließender Pfad werden beim Einlesen verworfen; nur der Hostname bleibt.
Hosts müssen exakt passen und ein Platzhalter `*.example.com` wird bei der Installation abgelehnt.
Ein Eintrag ohne Port erlaubt nur die Ports 80 und 443; `host:port` erlaubt nur diesen Port.

| Eintrag | Gültig | Grund |
|---|---|---|
| `api.tmdb.org` | Ja | Genauer Hostname, die normale Form |
| `https://api.tmdb.org` | Ja | Schema und Pfad beim Einlesen verworfen, Hostname bleibt |
| `api.example.com:8443` | Ja | Dieser Host nur auf Port 8443 |
| `*.tmdb.org` | Nein | Platzhalter werden bei der Installation abgelehnt; jeden Host einzeln auflisten |
| `*` | Nein | Ein einzelner Platzhalter wird abgelehnt |
| `192.168.1.1`, `::1` | Nein | IP-Adressen werden abgelehnt, beide Familien |

Zur Laufzeit setzt der Vermittler des Hosts eigene Grenzen, die das
Manifest nicht erweitern kann: nur `http` und `https`, nur `GET` und `HEAD`,
der Zielhost wird ohne Beachtung von Groß- und Kleinschreibung mit Ihrer
Liste verglichen, der Zielport mit dem Eintrag (80 oder 443 ohne angegebenen
Port), private oder reservierte Adressen werden nach der DNS-Auflösung abgelehnt,
ein Inhalt ist auf 8 MiB begrenzt und eine Anfrage auf 20 Sekunden.
Weiterleitungen werden erneut gegen die Liste, die Portregel und die
Adressregel geprüft.

Dieselbe Hostliste ist das `connect-src` des Frames des Plugins, jeweils mit
`https://` davor: Ein Widget darf diese Hosts direkt aufrufen, nur über
`https`.

## Liste der erlaubten Slots { #slot-whitelist }

Plugins dürfen Oberfläche nur in ausdrücklich erlaubte Slots einfügen.
Kritische Bereiche fehlen absichtlich.

| Slot | Anmeldung nötig | Hinweise |
|---|---|---|
| `home.section` | Ja | Inhaltsbereich der Startseite |
| `home.hero` | Ja | Bereich des großen Banners |
| `library.sidebar` | Ja | Seitenleiste der Bibliotheksansicht |
| `media.detail.sidebar` | Ja | Seitenleiste der Detailseite eines Mediums |
| `player.overlay` | Ja | Überlagerung im Player |
| `settings.user.appearance` | Ja | Bereich Darstellung in den Benutzereinstellungen |
| `sidebar.bottom` | Ja | Unterer Bereich der Navigationsleiste |
| `admin.nav` | Ja | Ein Navigationseintrag im Verwaltungsbereich, der die eigene Sandbox-Seite des Plugins öffnet; er fügt nichts in bestehende Verwaltungsbildschirme ein |

**Nie erlaubt:** `admin.*` (außer `admin.nav`), `login.*`, `setup.*`,
`profile.security.*`. Diese Bereiche sind für Anmeldung oder Rechte kritisch,
und ein Einfügen dort würde eine Rechteausweitung oder Phishing ermöglichen.

Inhalt in `admin.nav` ist isoliert wie in jedem anderen Slot: Er wird in
einem `<iframe sandbox="allow-scripts">` ohne `allow-same-origin`
dargestellt, in einer Seite mit der Richtlinie `default-src 'none'`. Er ist
also fremder Inhalt, der im Verwaltungsbereich angezeigt wird, keine
Verwaltungsfähigkeit, die dem Plugin gewährt wird.

Die Brücke begrenzt, was ein Frame von der Seite verlangen darf:
`evey.toast` höchstens 3 Nachrichten pro Sekunde, 200 Zeichen, kein HTML;
`evey.navigate` nur `/home`, `/library`, `/media`, `/player`, `/settings`
und `/search`; `evey.resize` auf 50 bis 800 px begrenzt. Der Frame erhält
keinen Token: Der Host stellt für die Aufrufe `public` und `prefs` einen
Token für fünf Minuten aus, begrenzt auf (Plugin, Benutzer), und der Server
lehnt standardmäßig jede Methode ab, die dieser Token nicht nennt.

## Ablauf eines Updates { #update-workflow }

1. Der Administrator lädt ein neues ZIP hoch. `PreviewPluginUpdate` führt
   die statische Analyse und die Signaturprüfung aus und liefert den
   Unterschied bei Berechtigungen, Capabilities, Slots und `external_fetch`
   gegenüber der installierten Version.
2. Ist der Unterschied nicht leer, muss der Administrator ihn bestätigen,
   bevor das Update angewendet wird (`requires_ack = true`; über die
   Upload-Route `accept_permission_changes=true`).
3. Der Hash des Pakets wird bei der Vorschau festgehalten und beim Anwenden
   geprüft: Das analysierte ZIP ist genau das ZIP, das installiert wird.
4. Der Signaturschlüssel wird auf Kontinuität geprüft: Ein Update, das mit
   einem anderen Schlüssel als die installierte Version signiert oder nicht
   mehr signiert ist, wird abgelehnt, bis der Administrator den neuen
   Herausgeber akzeptiert. Mit `EVEY_PLUGINS_COSIGN_POLICY=enforce` blockiert
   eine nicht vertraute oder ungültige Signatur beim Anwenden das Update,
   auch wenn die Vorschau bestanden hat.

<!-- sources
manifest validation: internal/adapters/driven/pluginrunner/manifest.go - validateManifest, validateExternalFetch ; internal/domain/plugin/model.go - Validate ; capabilities.go - KnownPermissions, KnownCapabilities
escalated warnings, native-binary: internal/app/plugin/install.go - checkEscalatedWarnings, validateStagedPlugin
comments stripped before analysis: docs/SECURITY.md (Static analysis) ; internal/adapters/driven/pluginrunner/analyzer_normalize.go
subprocess, deny-fork, deny-inet, render-dev: cmd/evey-plugin-launcher/main.go ; i18n admin.plugins.subprocessWarning
resource defaults: internal/config/config.go - Plugins
permission per RPC: internal/adapters/driving/grpcapi/plugin_interceptor.go ; served or not: internal/adapters/driving/pluginhost/host_server.go
broker: internal/adapters/driving/pluginhost/fetch.go - Fetch, hostAllowed, maxFetchBytes, fetchTimeout
frame connect-src: frontend/packages/client/src/plugins/hostProtocol.ts - buildConnectSrc ; internal/adapters/driving/http/plugin_frame_handler.go - pluginFramePolicy
slots: internal/domain/plugin/slots.go - AllowedSlots
bridge limits: frontend/packages/client/src/plugins/hostProtocol.ts ; token: internal/app/plugin/token_service.go - PluginTokenTTL ; docs/adr/0012-plugin-token-deny-by-default.md
update workflow: internal/app/plugin/update.go - ApplyUpdate ; internal/app/plugin/install.go - resolveExistingPlugin ; proto/evey/v1/admin_plugin.proto - PreviewPluginUpdate, ApplyPluginUpdate
-->
