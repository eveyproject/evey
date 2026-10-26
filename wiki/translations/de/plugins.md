---
source_sha: c893d5af47331de964c0067be032f345a649fa683f5c57669f68661aa76f66e0
reviewed: true
---
# Plugins { #plugins }

Diese Seite richtet sich an Administratoren: wie Sie ein Plugin oder ein Theme
installieren, was Sie dabei akzeptieren und was der Server ihm auf Ihrem Host
erlaubt. Alles geschieht unter **Verwaltung → Plugins**. Die Seiten für
Plugin-Autoren sind [am Ende](#writing-a-plugin) aufgeführt.

Ein Plugin ergänzt Evey um etwas, das es selbst nicht kann: einen Abschnitt
auf der Startseite, eine Einblendung im Player, eine Metadatenquelle, ein
Theme. Jedes Plugin ist ein eigener Prozess, der über einen lokalen Socket mit
dem Server kommuniziert, durch eine Sandbox isoliert und auf die Berechtigungen
beschränkt, die sein Manifest angibt. Seine Oberfläche, falls es eine hat,
läuft in einem isolierten Rahmen der Seite.

<div class="evey-shot-pair" markdown>

<figure class="evey-shot" markdown="span">
  ![Die Startseite ohne Plugins](images/home-no-plugin.webp){ loading=lazy }
  <figcaption>Ohne Plugins.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Die Startseite mit dem Wetter-Plugin](images/home.webp){ loading=lazy }
  <figcaption>Mit dem Beispiel-Plugin für das Wetter: ein Abschnitt ganz oben, in seinem eigenen Rahmen dargestellt.</figcaption>
</figure>

</div>

## Ein Plugin installieren { #install-a-plugin }

1. Öffnen Sie unter **Verwaltung → Plugins** den Punkt **Plugin
   installieren** und wählen Sie die **Plugin-Datei (.zip)**. Liefert der
   Autor eine Signatur mit, fügen Sie sie unter **Signaturdatei (.zip.sig,
   optional)** hinzu. Der Reiter **Von URL** installiert stattdessen über
   einen Link. Hängen Sie `#sha256:<hash>` an die URL an, damit der Download
   geprüft wird.
2. **Vorschau & Installieren** analysiert das Paket, ohne es zu installieren.
   Die Vorschau zeigt die **Berechtigungen** und **Fähigkeiten**, die das
   Plugin anfordert, das Ergebnis der **Statischen Analyse** und, wenn eine
   Signatur mitgegeben wurde, das **Signatur**-Urteil mit dem Herausgeber. Die
   Slots und die Hosts werden beim Update und im **Sicherheits-Dashboard**
   angezeigt.
3. Diese Liste ist verbindlich: Das ist alles, was das Plugin tun
   können wird. Bestätigen Sie, und das Plugin startet.

<figure class="evey-shot" markdown="span">
  ![Verwaltung → Plugins auf dem Demo-Server](images/admin-plugins.webp){ loading=lazy }
  <figcaption><strong>Verwaltung → Plugins</strong>: die verfügbaren Laufzeiten, die Ressourcenlimits und eine Karte pro Plugin (Oberfläche auf Englisch).</figcaption>
</figure>

Ein Paket, in dem die Analyse ein kritisches Muster findet, wird abgelehnt,
egal was Sie antworten. Warnungen werden angezeigt, die Entscheidung liegt bei
Ihnen. Die Sprache des Plugins muss zu den **Verfügbaren Laufzeiten** des
Servers gehören: Das Standard-Image bringt Python, Node.js, PHP, Ruby und Lua mit, das Image
`-nodejs` behält Python und Node.js und das Image `-lean` nur Python.

!!! tip "Signierte Plugins"
    Ein Plugin-Autor kann das ZIP mit [cosign](https://docs.sigstore.dev/cosign/installation/)
    signieren; sein öffentlicher Schlüssel reist im Paket. Evey prüft die
    Signatur selbst (kein `cosign`-Programm) und zeigt **Signiert von
    <Herausgeber>**, wenn der Schlüssel in Ihrer Liste der **Vertrauenswürdigen
    Herausgeber** steht, sonst **Signiert, unbekannter Schlüssel** mit seinem
    Fingerabdruck (ein Klick vertraut ihm), **Nicht signiert** oder
    **Ungültig**. Ein Update, das den Schlüssel wechselt oder die Signatur
    weglässt, wartet auf Ihre ausdrückliche Zustimmung. Die Regel entscheidet
    den Rest:

    ```sh
    EVEY_PLUGINS_COSIGN_POLICY=warn      # off | warn (Standard) | enforce
    ```

    Mit `warn` wird das Urteil angezeigt und nichts blockiert; mit `enforce`
    werden nur vertrauenswürdige Herausgeber installiert. Für Autoren: siehe
    [Ihr Plugin signieren](plugins/dev-setup.md#7-sign-your-plugin).

## Ein Plugin aktualisieren { #update-a-plugin }

Auf der Karte des Plugins nimmt **Aus Quelle aktualisieren …** das neue ZIP
entgegen (einen Pfad oder eine URL) und zeigt dieselbe Vorschau. Ein Update
muss eine höhere Version haben als die installierte. Verlangt es neue
Berechtigungen, Fähigkeiten, Slots oder Hosts, zeigt die Vorschau **Diese
Aktualisierung beantragt neue Berechtigungen** und listet den Unterschied
auf: Nichts wird übernommen, bevor Sie **Genehmigen und installieren**
wählen. Das analysierte Paket ist das installierte Paket: Sein Hashwert wird
beim Übernehmen des Updates noch einmal geprüft.

## Die installierten Plugins verwalten { #manage-the-installed-plugins }

Jede Karte zeigt den Zustand des Plugins (**Aktiviert** oder
**Deaktiviert**) und bietet:

| Aktion | Was sie tut |
| --- | --- |
| **Aktiviert** / **Deaktiviert** | Startet oder stoppt den Prozess des Plugins. Ein deaktiviertes Plugin behält seine Dateien und seine Konfiguration. |
| **Konfiguration** | Die Einstellungen, die das Plugin angibt (ein API-Schlüssel, eine Stadt, eine Einheit...). Geheime Felder werden maskiert angezeigt und nie an den Browser zurückgeschickt. |
| **Protokolle** | Die letzten Zeilen, die das Plugin über den Host geschrieben hat. |
| **Metadaten** | Nur bei einem Metadaten-Plugin: die Felder, die es ersetzt, und der Abruf der vorhandenen Bibliothek ([weiter unten](#metadata-plugins)). |
| **Deinstallieren** | Entfernt den Prozess, die Dateien und die gespeicherten Daten des Plugins. |

**Ressourcenlimits** oben auf der Seite begrenzt jeden Plugin-Prozess. Die
Werte im Verwaltungsbereich sind die Standardwerte. Die Umgebungsvariable
legt sie fest, solange im Verwaltungsbereich noch nie gespeichert wurde.

| Feld | Standard | Umgebungsvariable |
| --- | --- | --- |
| **Max. Arbeitsspeicher (MiB)** | 256 | `EVEY_PLUGINS_MEMORY_MB` |
| **Max. Prozesse** | 64 | `EVEY_PLUGINS_PIDS_MAX` |
| **Max. Festplatte (MiB)** | 100 | `EVEY_PLUGINS_DISK_MAX_MB` |
| **CPU-Nice (0-19)** | 10 | `EVEY_PLUGINS_CPU_NICE` |
| **Sandbox-Modus** | `auto` | `EVEY_PLUGINS_SANDBOX_MODE` |

Arbeitsspeicher, Prozesse und Festplatte gelten beim nächsten Durchlauf der
Überwachung, die ein Plugin stoppt, das sie überschreitet. CPU-Priorität und
Sandbox-Modus gelten für danach gestartete Plugins. `0` stellt für
Arbeitsspeicher und Prozesse den Standardwert wieder her; nur die Festplattengrenze behandelt `0` als „aus“.

Das **Sicherheits-Dashboard** hält pro Plugin die Befunde der Analyse, das
Ergebnis der Signaturprüfung und den letzten Fehler fest. Seine Zeile
**Host-Sandbox** zeigt den Sandbox-Modus, der auf dieser Maschine wirklich
gilt: Steht dort `basic`, bietet der Host keine Beschränkung des Dateisystems
pro Pfad, und [Plugin-Sandbox einrichten](plugins/sandbox-deployment.md)
erklärt, was zu ändern ist.

Plugins sind standardmäßig eingeschaltet. `EVEY_PLUGINS_ENABLED=false`
schaltet das gesamte Plugin-System ab.

## Metadaten-Plugins { #metadata-plugins }

Ein Metadaten-Plugin (Fähigkeit `metadata.provider`) liefert eigenen Text für
Ihre Filme und Serien, aus einer anderen Quelle als TMDB. Es ersetzt TMDB
nicht: TMDB erkennt weiterhin jeden Titel und liefert Besetzung, Bilder,
Bewertungen, Sammlungen und Übersetzungen. Der Text des Plugins wird neben dem
von TMDB gespeichert und Feld für Feld an seiner Stelle angezeigt.

Was ein Zuschauer liest, vom schwächsten zum stärksten:

1. der Text von TMDB, in der Sprache des Zuschauers, wenn TMDB ihn hat;
2. der Text des Plugins in dieser Sprache, für die Felder, die das Plugin
   ersetzen darf;
3. Ihre eigene Korrektur des Titels (**Metadaten bearbeiten** auf seiner
   Seite), die immer Vorrang hat.

**Metadaten** auf der Karte des Plugins wählt die Felder, die es ersetzt,
unter **Titel**, **Beschreibung** und **Slogan**. Solange Sie nichts gewählt
haben, ersetzt ein Plugin die Beschreibung und den Slogan, nie den Titel. Eine
Änderung gilt sofort für das, was das Plugin bereits geliefert hat; nichts wird
erneut abgerufen.

Das Plugin wird zu jedem Film und jeder Serie befragt, wenn sie zugeordnet
oder aktualisiert werden. **Für die ganze Bibliothek abrufen** im selben Bereich
befragt es zu den bereits vorhandenen Titeln, einem nach dem anderen, zeigt
den Fortschritt und am Ende, wie viele es gefunden hat und wie viele ohne
Antwort blieben. Eine Aktualisierung des Titels von TMDB überschreibt den Text
des Plugins nie; das Plugin zu deaktivieren blendet ihn aus, es zu
deinstallieren löscht ihn.

Staffeln und Episoden behalten den Text von TMDB, und die Suche findet weiterhin
die Titel von TMDB.

## Was ein Plugin auf Ihrem Host tun kann { #what-a-plugin-can-do-on-your-host }

Egal, was sein Manifest angibt, ein Plugin:

- sieht sein eigenes Verzeichnis, weder die Daten des Servers noch die
  Mediendateien;
- hat keinen direkten Netzwerkzugang: Ein Plugin mit der Berechtigung
  `network` erreicht nur die Hosts, die es angegeben hat, über den Server, mit
  `GET` und `HEAD`;
- kann keine Kindprozesse starten, es sei denn, es gibt `subprocess` an;
- läuft unter den Ressourcenlimits oben und, wenn der Host es erlaubt, unter
  einem eigenen Benutzer.

Seine Oberfläche ist auf die Slots beschränkt, die das Manifest angibt: einen
Abschnitt oder den großen Bereich oben auf der Startseite, eine Seitenleiste
in einer Bibliothek oder auf einer Detailseite, eine Einblendung im Player,
die Darstellungseinstellungen, den unteren Teil der Navigationsleiste oder
einen Eintrag im Verwaltungsmenü, der die eigene Seite des Plugins öffnet.
Ein Plugin kann nie etwas in den Bildschirmen für Anmeldung, Einrichtung und
Sicherheit oder in den vorhandenen Verwaltungsbildschirmen anzeigen. In seinem
Rahmen hat es keinen Zugriff auf die Seite, Ihre Cookies oder Ihre Sitzung:
Was es vom Server liest, läuft über ein kurzlebiges Token, das auf die zwei
Berechtigungen beschränkt ist, die es verlangen darf (`public` für eine
sichere Auswahl aus Medientiteln und Postern, `prefs` für die Einstellungen
des aktuellen Benutzers in seinem eigenen Namensraum).

Die Berechtigungen, die ein Plugin verlangen kann, stehen unter
[Berechtigungen und Slots](plugins/security-model.md). Wie die Sandbox
funktioniert und was sie trotzdem nicht verhindern kann, steht unter
[Plugin-Sandbox und Bedrohungsmodell](SECURITY.md).

!!! warning "Der Server prüft, welche Hosts ein Plugin kontaktiert, nicht welche Daten es sendet"
    Steht ein Host einmal auf der Liste eines Plugins, kann der Server nicht
    prüfen, was das Plugin an ihn schickt. Prüfen Sie ein Plugin eines
    Drittanbieters vor der Installation, wie jede andere Software, und
    betrachten Sie seine Host-Liste als Liste derer, die die Titel Ihrer
    Bibliothek erhalten können.

## Themes { #themes }

Ein Theme ist ein Paket mit `kind: theme` ohne Prozess: ein Stylesheet und
optional ein Skript, das isoliert in einem versteckten Rahmen läuft. Es wird
über denselben Dialog **Plugin installieren** installiert; das Stylesheet wird
bei der Installation bereinigt. Jeder Benutzer wählt es dann unter
**Einstellungen → Darstellung → Design**. Was ein Theme enthalten darf, steht
unter [Themes](themes.md).

## Ein Plugin schreiben { #writing-a-plugin }

| Seite | Inhalt |
| --- | --- |
| [Ein Plugin schreiben](plugins/dev-setup.md) | Ein erstes Plugin Schritt für Schritt, der Installationsablauf, Signieren, Verteilung. Fangen Sie hier an. |
| [Plugin-API-Referenz](plugins/api-reference.md) | Das Manifest, die Backend-Aufrufe und der Datenspeicher, die `evey.*`-Brücke der Oberfläche. |
| [Berechtigungen und Slots](plugins/security-model.md) | Was ein Manifest angeben darf, was bei der Installation geprüft wird, die Regeln für externe Abrufe, die Slots. |
| [Themes](themes.md) | Die Oberfläche mit CSS und einem Skript in einer Sandbox umgestalten. |
| [Webhooks und API-Schlüssel](webhooks-api-keys.md) | Evey mit anderen Werkzeugen verbinden, ohne ein Plugin zu schreiben. |

<!-- sources
install dialog, from URL with #sha256: i18n admin.plugins.installPlugin, pluginFileZip, sigFileOptional, fromUrl, installFromUrlHint, previewInstall, staticAnalysis ; internal/app/plugin/install.go - downloadIfURL
critical findings refuse, warnings: internal/app/plugin/install.go - validateStagedPlugin, checkEscalatedWarnings ; i18n admin.plugins.blockedByAnalysis, installAnyway
runtimes: internal/adapters/driven/pluginrunner/runtime_availability.go - runtimeBinaries ; i18n admin.plugins.availableRuntimes
cosign policy: internal/config/config.go - CosignPolicy, CosignPublicKey ; internal/adapters/driven/pluginrunner/cosign.go
update, version higher, escalation, hash re-checked: internal/app/plugin/install.go - resolveExistingPlugin ; internal/app/plugin/update.go - ApplyUpdate ; internal/app/plugin/permissions.go - DiffPermissions ; i18n admin.plugin.update, admin.plugins.permissionEscalation.*
card actions: i18n admin.plugin.enabled, disabled, config, logs, uninstall ; secrets masked: internal/domain/plugin/model.go - IsSecretConfigField ; internal/adapters/driving/grpcapi/admin_plugin.go
resource limits defaults and env: internal/config/config.go - Plugins (MemoryMB, PIDsMax, DiskMaxMB, CPUNice, SandboxMode) ; i18n admin.plugins.resourceLimits.*
security dashboard, host sandbox: i18n admin.plugins.security.* ; internal/app/plugin/security_dashboard_service.go
plugins enabled flag: internal/config/config.go - PluginsEnabled
fetch broker GET/HEAD: internal/adapters/driving/pluginhost/fetch.go - Fetch
subprocess: docs/plugins/security-model.md (Permissions catalog)
slots: internal/domain/plugin/slots.go - AllowedSlots
scoped token, public/prefs: docs/adr/0012-plugin-token-deny-by-default.md ; internal/pkg/auth/plugin_jwt.go - AllowedPluginScopes
theme selection: frontend/apps/web/src/features/user/ThemeSettings.tsx ; i18n common.appearance, common.theme
-->
