---
source_sha: f4ecae392bc0b50e2c1ef8c70f50db7121732b5c6e7c194891cf416bffb92680
reviewed: false
---
# Berechtigungen und Slots

Diese Seite ist für Plugin-Autoren: was ein Manifest angeben darf, was bei der
Installation eines Plugins geprüft wird und welche Regeln zur Laufzeit gelten.
Wie der Server ein Plugin einsperrt und was diese Einschränkung nicht verhindern
kann, beschreibt [Sicherheitsmodell und Plugin-Sandbox](../SECURITY.md).

## Was bei der Installation geprüft wird

Ein Plugin wird bei der Installation abgelehnt, wenn:

- `kind` fehlt oder weder `plugin` noch `theme` ist;
- einem `kind: plugin` `language` oder `entrypoint` fehlt, oder ein
  `kind: theme` sie oder `permissions` angibt;
- eine Berechtigung oder Capability unbekannt ist;
- ein Frontend-Modul einen Slot außerhalb der [Slot-Liste](#slot-whitelist)
  anspricht;
- ein Eintrag in `external_fetch` gegen die [Abrufregeln](#external-fetch-rules)
  verstößt;
- das ZIP die Grenzen beim Entpacken überschreitet: mehr als 5.000 Dateien, eine
  Datei über 100 MB, mehr als 500 MB entpackt, ein Kompressionsverhältnis über
  100:1, ein Pfad, der das Archiv verlässt, oder ein symbolischer Link;
- die statische Analyse ein kritisches Muster findet (Shell-Ausführung,
  Verschleierung, dynamischer Import, Ausbruch aus der Sandbox, Rechtewechsel,
  Mining...). Kompilierte Programme und eingebettete Archive werden abgelehnt,
  außer das Manifest gibt `native-binary` an.

Die Analyse liest auch Kommentare: Eine Zeile `# pip install foo` in einem
Kommentar wird wie echter Code gemeldet. Entfernen Sie solche Zeilen vor der
Veröffentlichung. Die vollständige Liste der Kategorien steht unter
[Sicherheitsmodell und Plugin-Sandbox](../SECURITY.md#static-analysis).

Vor der Installation sieht der Administrator die angegebenen Berechtigungen,
Slots und Hosts sowie die Funde der Analyse.

## Unter welchen Bedingungen das Plugin läuft

Was auch immer das Manifest angibt:

- das Plugin sieht sein eigenes Verzeichnis, nicht die Daten des Servers;
- es hat keinen direkten Netzwerkzugang: Ein Plugin mit der Berechtigung
  `network` erreicht die Hosts seiner Liste `external_fetch` über den Aufruf
  `Fetch` des Hosts;
- ohne `subprocess` kann es keine Kindprozesse starten (Threads funktionieren);
- es läuft unter Ressourcengrenzen, die der Administrator anpassen kann:
  standardmäßig 256 MiB Speicher, 64 Prozesse und Threads, 100 MiB Festplatte für
  sein Verzeichnis und eine niedrigere CPU-Priorität.

Signieren ist während der Entwicklung optional. Wie man ein Plugin signiert und
welche Signaturrichtlinie ein Server durchsetzen kann:
[Ein Plugin schreiben](dev-setup.md).

## Berechtigungskatalog

Eine Berechtigung wird unter `permissions` angegeben; jeder andere Wert führt zur Ablehnung der Installation. Webhooks empfangen (`webhook.receiver`) und Metadaten liefern (`metadata.provider`) sind Capabilities, angegeben unter `capabilities`: siehe [Plugin-API-Referenz](api-reference.md#capabilities).

| Berechtigung | Was sie gewährt |
|---|---|
| `network` | Das Plugin darf ausgehende Netzwerkverbindungen herstellen. **Muss** mit einer nicht leeren Liste `external_fetch` kombiniert werden - ohne sie wird die Installation abgelehnt. |
| `subprocess` | Das Plugin darf Kindprozesse starten. Ohne sie greifen zwei Schichten: Der LD_PRELOAD-seccomp-Shim blockiert `execve`/`execveat` für interpretierte Plugins, und das `-deny-fork` des Launchers blockiert die Prozesserzeugung selbst (`fork`/`vfork`/`clone` ohne `CLONE_THREAD`) im Kernel. Threads funktionieren in beiden Fällen. (Das Kernel-seccomp des Launchers verweigert `execve` nicht - er muss das Plugin ausführen -, aber jedes Kind erbt den Kernelfilter + die Landlock-Domäne.) |
| `native-binary` | Das Plugin darf `.so`/`.dll`/`.exe`/`.wasm`/`.jar` oder eingebettete Archive mitliefern. Der Administrator sieht den undurchsichtigen Code in der Installationsvorschau und muss ihn annehmen. |
| `filesystem` | Angegebene Absicht, auf das Dateisystem außerhalb des eigenen Verzeichnisses zuzugreifen. Wird gespeichert und dem Administrator gezeigt; die Sandbox gewährt keinen Dateizugriff außerhalb des Plugin-Verzeichnisses, was auch immer angegeben ist. |
| `media.read` | Das Plugin darf die RPCs `MediaRead` aufrufen: Metadaten von Mediendateien und Stream-Zugriff lesen. |
| `media.write` | Das Plugin darf die RPCs `MediaWrite` aufrufen: Medieneinträge anlegen und ändern. |
| `api.library` | Das Plugin darf RPCs der Bibliotheksverwaltung aufrufen: Bibliotheken auflisten und durchsuchen, Scans auslösen. |
| `api.user` | Das Plugin darf benutzerbezogene RPCs aufrufen (eigenes Profil, Vorlieben lesen). |
| `api.metadata` | Das Plugin darf Metadaten-RPCs aufrufen: suchen und Anreicherungsdaten schreiben. |
| `notification.sender` | Das Plugin darf Benachrichtigungsereignisse veröffentlichen (Web-Push, E-Mail, Webhook). |
| `public` | Die Oberfläche des Plugins darf eine sichere Projektion der Medien lesen (ID, Titel, Typ, Jahr, Zusammenfassung, Poster), nie Pfade oder Streams. Siehe [Plugin-API-Referenz](api-reference.md#public). |
| `prefs` | Die Oberfläche des Plugins darf die Vorlieben des aktuellen Benutzers im eigenen Namensraum des Plugins lesen und schreiben. Siehe [Plugin-API-Referenz](api-reference.md#prefs). |
| `hardware.gpu` | **Berechtigung mit dem höchsten Risiko - nur für Administratoren, Stufe der offiziellen Plugins.** Gewährt den DRM-Render-Knoten des Hosts (`-render-dev`, das echte `renderD1xx`, nie ein Card-Knoten, nie `/dev/dxg`/`/dev/nvidia*`/`/dev/kfd`): eine Angriffsfläche im Kerneltreiber und eine GPU-Speichergrenze, die mit dem Transkodierer GETEILT wird (prozessübergreifendes Speicherleck auf AMD-, Apple- und Qualcomm-GPUs möglich; eine hängende GPU kann die Transkodierung blockieren). Ohne diese Berechtigung sind GPU-Geräteknoten ganz aus der Landlock-Geräteliste ausgeschlossen (unabhängig von den Dateirechten verweigert). Mit ihr fügt der Launcher eine ausdrückliche Landlock-Regel `IOCTL_DEV` nur für diese eine Datei hinzu (ab ABI v5); auf älteren Landlock-ABIs regelt Landlock ioctls gar nicht, sodass Lese- und Schreibzugriff allein bereits das Einreichen von GPU-Befehlen erlauben. Der Installer zeigt in der Vorschau eine deutliche Warnung CRITICAL; nur vertrauenswürdigen, offiziellen Plugins gewähren. |

## Regeln für externe Abrufe

Angegeben in `external_fetch: [...]` im Manifest und bei der Installation von
`validateExternalFetch` (`internal/adapters/driven/pluginrunner/manifest.go`)
geprüft. Alles oder nichts: Ein ungültiger Eintrag lehnt die ganze Installation
ab.

Ein Eintrag ist ein **reiner Hostname**. Ein Schemapräfix wird geduldet und
verworfen - nur der Hostname bleibt.

| Eintrag | Gültig | Grund |
|---|---|---|
| `api.tmdb.org` | Ja | Genauer Hostname, die normale Form |
| `https://api.tmdb.org` | Ja | Schema geparst und verworfen, Hostname behalten |
| `*.tmdb.org` | Nein | Platzhalter werden abgelehnt - listen Sie jede benötigte Subdomain auf |
| `*` | Nein | Gleiche Regel |
| `192.168.1.1`, `::1` | Nein | IP-Literale werden abgelehnt, beide Familien |

Zur Laufzeit fügt der Broker des Hosts eigene Grenzen hinzu, die das Manifest
nicht erweitern kann: nur `http` und `https`, nur `GET` und `HEAD`, der Zielhost
ohne Beachtung der Groß- und Kleinschreibung mit Ihrer Liste abgeglichen, und
private oder reservierte Adressen nach der DNS-Auflösung abgelehnt -
Weiterleitungen werden gegen beide Regeln erneut geprüft.

> Der Abgleich des Brokers versteht auch eine Form `*.example.com`, aber der
> Installer lehnt einen solchen Eintrag ab, ein solcher Eintrag erreicht ihn also nie.
> Schreiben Sie ausdrückliche Hostnamen.

## Slot-Liste

Plugins dürfen Oberfläche nur in ausdrücklich erlaubten Slots einfügen. Kritische Pfade fehlen absichtlich.

| Slot | Anmeldung erforderlich | Hinweise |
|---|---|---|
| `home.section` | Ja | Hauptinhaltsbereich der Seite |
| `home.hero` | Ja | Bannerbereich |
| `library.sidebar` | Ja | Seitenleiste der Bibliotheksansicht |
| `media.detail.sidebar` | Ja | Seitenleiste der Mediendetails |
| `player.overlay` | Ja | Überlagerung im Player |
| `settings.user.appearance` | Ja | Abschnitt Darstellung in den Benutzereinstellungen |
| `sidebar.bottom` | Ja | Unterer Bereich der Navigationsleiste |
| `admin.nav` | Ja | Ein Navigationseintrag im Admin-Bereich, der die eigene abgeschottete Seite des Plugins öffnet - fügt nichts in bestehende Admin-Bildschirme ein |

**Nie erlaubt:** `admin.*` (außer `admin.nav`), `login.*`, `setup.*`, `profile.security.*` - diese Pfade sind für Authentifizierung und Rechte kritisch, und ein Einfügen von Plugins dort ist ein Einfallstor für Rechteausweitung oder Phishing.

Auch der Inhalt von `admin.nav` ist wie jeder andere Slot abgeschottet: Er wird in einem `<iframe sandbox="allow-scripts">` (ohne `allow-same-origin`) mit der CSP `default-src 'none'` gezeichnet, er ist also Inhalt von Dritten, der im Admin-Bereich angezeigt wird, keine dem Plugin gewährte Administratorfähigkeit.

Quelle: `internal/domain/plugin/slots.go`.

Frontend-Module laufen in einem abgeschotteten `<iframe>` ohne Zugriff auf die
Host-Seite. Die Brücke begrenzt, was sie tun können: `evey.toast` höchstens 3
Meldungen pro Sekunde, 200 Zeichen, kein HTML; `evey.navigate` akzeptiert nur
`/home`, `/library`, `/media`, `/player`, `/settings` und `/search`;
`evey.resize` wird auf 50-800 px begrenzt.

## Ablauf eines Updates

1. Der Administrator lädt ein neues ZIP hoch → `PreviewPluginUpdate` führt die statische Analyse und die Cosign-Prüfung aus und liefert die Unterschiede bei Berechtigungen, Slots und `external_fetch` gegenüber der installierten Version.
2. Sind die Unterschiede nicht leer (neue Berechtigungen, neue Slots, geänderte externe Hosts), muss der Administrator ausdrücklich zustimmen, bevor das Update angewendet wird (`requires_ack = true`).
3. Die Prüfsumme der Quelle wird bei der Vorschau erfasst und bei der Anwendung geprüft (gegen TOCTOU: Das analysierte ZIP ist das installierte ZIP).
4. Ist die Cosign-Richtlinie `enforce`, blockiert eine fehlerhafte Signatur beim Anwenden das Update, auch wenn die Vorschau bestanden wurde.
