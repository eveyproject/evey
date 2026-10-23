---
source_sha: 9c17a56bd4379a8c15fd8d637fe0c4914e9b9cf848503ddd72e806f574506caa
reviewed: true
---
# Plugin-Sandbox einrichten { #plugin-sandbox-setup }

Welche Sandbox-Einrichtung zu Ihrem Server passt, in einfachen Worten. Für
fast alle lautet die Antwort: Lassen Sie die Standardwerte. Der Server erkennt
die Maschine und wählt die stärkste Isolierung, die verfügbar ist. Lesen
Sie nur weiter, wenn Sie ein altes NAS haben, Evey ohne Docker betreiben oder
eine stärkere Isolierung wollen. Das vollständige technische Modell
steht unter [Sicherheitsmodell und Plugin-Sandbox](../SECURITY.md).

## Was Sie schützt { #what-protects-you }

Ein Plugin, das Sie installieren, läuft isoliert. Je nach Maschine:

- **begrenzt die Isolierung, welche Dateien es anfassen kann**: seinen eigenen
  Ordner und die Systembibliotheken, die es braucht, sonst nichts (nicht Ihre
  Medien, nicht Ihre Geheimnisse, nicht andere Plugins);
- **blockiert sie gefährliche Systemaktionen** im Kernel: andere Prozesse
  debuggen, Module laden, die Uhr verstellen, Laufwerke einbinden;
- **sperrt sie den direkten Netzwerkzugang**, immer: Ein Plugin, das Netzwerk
  verlangt hat und dem Sie es erlaubt haben, erreicht nur die Hosts, die es
  angegeben hat, und das über den Server;
- **begrenzt sie CPU, Arbeitsspeicher, Festplatte und Zahl der Prozesse**,
  damit ein Plugin, das zu viele Ressourcen verbraucht, den Server nicht lahmlegen kann;
- **hält sie Ihre Geheimnisse unlesbar**: den Schlüssel, der Anmeldungen
  signiert, und die Sicherungen.

Dazu kommt: Nur Administratoren können Plugins installieren, Plugins werden
auf verdächtigen Code geprüft, und Sie genehmigen ihre Berechtigungen, bevor
sie laufen.

**Prüfen Sie, was gilt:** **Verwaltung → Plugins → Sicherheits-Dashboard**
zeigt die Sandbox des Hosts, den Modus, den `auto` gewählt hat
(`landlock`, `bubblewrap` oder `basic`), mit den Prüfergebnissen
(**Landlock-Kernel**, **Sandbox-Launcher**, **Bubblewrap**). `basic` bedeutet,
dass der Host keine Dateibeschränkung pro Pfad bietet: Plugins
laufen nur mit einem Wechsel der UID und seccomp. Die Prüfungen sagen, warum,
und die Abschnitte unten, was zu ändern ist.

## Ihre Situation wählen { #pick-your-situation }

| Ihr Server | Was Sie ohne Zutun bekommen | Etwas zu tun? |
|---|---|---|
| **Modernes Linux in Docker** (Kernel 5.13 oder neuer, ungefähr alles ab 2021), der Normalfall | die stärkste Isolierung (Landlock), automatisch | nichts |
| **Altes NAS in Docker** (Kernel älter als 5.13) | gute Isolierung, aber ohne die Dateibeschränkung pro Ordner; Geheimnisse, Netzwerk und Systemaktionen bleiben gesperrt | nichts: Die UID-Trennung (unten) schaltet sich selbst ein |
| **Beliebiges Linux, ohne Docker** | die stärkste verfügbare Isolierung: Landlock auf einem modernen Kernel, sonst bubblewrap-Namespaces | folgen Sie [Ohne Container](../deployment-bare-metal.md) |

## Die einzige Einstellung, die Sie vielleicht ändern: die UID-Trennung { #the-one-setting-you-might-change-uid-separation }

Die UID-Trennung lässt ein Plugin unter einem anderen Benutzerkonto als den
Server laufen. So kann es die geheimen Dateien des Servers nicht lesen,
selbst wenn nichts anderes es verhindern würde. Das ist nur auf einer Maschine ohne
die Dateibeschränkung pro Ordner relevant (der Fall `basic`). Auf einer modernen Maschine
hält diese Beschränkung die Geheimnisse bereits unzugänglich, und die Trennung ist eine
zusätzliche, redundante Schicht.

`EVEY_PLUGINS_UID_SEPARATION`:

| Wert | Bedeutung | Verwenden, wenn |
|---|---|---|
| `auto` (Standard) | nur an, wenn sie gebraucht wird (der Fall `basic`), aus, wo sie redundant wäre | immer, außer einer der beiden Fälle unten trifft zu |
| `on` | immer an, ein zweiter, unabhängiger Schutz zusätzlich zur Dateibeschränkung | Sie beide Schutzmaßnahmen wollen und die kleinen Kosten unten in Kauf nehmen |
| `off` | nie | ein bestimmtes Plugin sich damit falsch verhält (selten) |

!!! warning
    Die UID-Trennung ist nicht mit der Docker-Option `no-new-privileges`
    vereinbar: Entscheiden Sie sich für eines von beiden. Kann der Wechsel
    auf einer alten Maschine (`basic`) nicht gewährt werden, weigert sich der
    Server, das Plugin zu starten, statt es neben Ihren Geheimnissen laufen
    zu lassen. Auf einer modernen Maschine schreibt er eine Warnung ins
    Protokoll und startet das Plugin ohne den zusätzlichen Schutz. Das Image
    bringt alles Nötige mit, Sie setzen nur die Variable.

Die kleinen Kosten von `on`: Das Plugin läuft unter einem eigenen Konto. Ein
Plugin, das Hilfsdateien in seinen eigenen Ordner schreibt, kann dabei
fehlschlagen. Passiert das, setzen Sie `off`. Deshalb ist der Standard `auto`: Er
geht dieses Risiko nur ein, wo es einen echten Nutzen gibt.

## Was ein bösartiges Plugin tun könnte { #what-a-malicious-plugin-could-do }

**Modernes Linux (Docker oder ohne Container):** Ein bösartiges Plugin ist
vollständig isoliert. Es kann Ihre Geheimnisse nicht lesen, die Dateien
anderer Plugins weder sehen noch anfassen, keine beliebigen
Netzwerkverbindungen aufbauen und das System nicht manipulieren. Der einzige
realistische Weg hinaus ist ein schwerer Kernel-Fehler. Das gilt für jede Sandbox,
die keine vollständige virtuelle Maschine ist.

**Altes NAS in Docker, ohne Dateibeschränkung pro Ordner (`basic`):**

- Ihre Geheimnisse bleiben geschützt (Anmeldeschlüssel, Sicherungen), durch
  das eigene Benutzerkonto, das `auto` hier einschaltet;
- das Plugin hat kein direktes Netzwerk; ein erlaubtes erreicht nur die
  Hosts, die es angegeben hat;
- gefährliche Systemaktionen bleiben im Kernel blockiert;
- aber ohne Dateibeschränkung pro Ordner könnte ein bösartiges Plugin gewöhnliche, nicht
  geheime Dateien lesen, laufende Prozesse auflisten und, weil sich die
  Plugins ein Konto teilen, die Dateien eines anderen Plugins lesen oder
  stören.

Installieren Sie auf einem solchen Host nur Plugins, denen Sie vertrauen, oder
wechseln Sie auf einen modernen Kernel, um die vollständige Dateibeschränkung zu erhalten.

## Beispiele für die Konfiguration { #configuration-examples }

Jede Zeile gehört unter `environment:` in der Compose-Datei, geschrieben als
`KEY: value` (zum Beispiel `EVEY_PLUGINS_UID_SEPARATION: "on"`).

```bash title="1. Moderner Server, empfohlen (nichts zu tun)"
EVEY_PLUGINS_SANDBOX_MODE=auto          # wählt die stärkste verfügbare Stufe (Standard)
# EVEY_PLUGINS_UID_SEPARATION=auto      # (Standard) nur dort an, wo nötig
```

```bash title="2. Altes NAS (nichts zu setzen, nur zur Verdeutlichung)"
# no-new-privileges nicht in die security_opt der Compose-Datei aufnehmen, es würde den Wechsel blockieren
EVEY_PLUGINS_SANDBOX_MODE=auto
EVEY_PLUGINS_UID_SEPARATION=auto        # (Standard) im Fall basic an
```

```bash title="3. Beide Schutzmaßnahmen auf einem modernen Server"
EVEY_PLUGINS_SANDBOX_MODE=auto          # ergibt landlock
EVEY_PLUGINS_UID_SEPARATION=on          # zusätzlich die unabhängige UID-Trennung
```

```bash title="4. Einen Modus festlegen (fortgeschritten)"
EVEY_PLUGINS_SANDBOX_MODE=landlock      # Plugins nicht starten, wenn Landlock nicht verfügbar ist
```

```bash title="5. Ein Plugin funktioniert mit UID-Trennung nicht"
EVEY_PLUGINS_UID_SEPARATION=off
```

```bash title="6. Ressourcengrenzen (später änderbar unter Verwaltung → Plugins → Ressourcenlimits)"
EVEY_PLUGINS_MEMORY_MB=256              # max. Arbeitsspeicher pro Plugin
EVEY_PLUGINS_PIDS_MAX=64                # max. Prozesse und Threads
EVEY_PLUGINS_DISK_MAX_MB=100            # max. Größe des Installationsordners (0 = aus)
EVEY_PLUGINS_CPU_NICE=10                # Planungspriorität 0-19 (höher = rücksichtsvoller)
```

## Den Sandbox-Modus im laufenden Betrieb ändern { #change-the-sandbox-mode-while-running }

Ändern Sie **Sandbox-Modus** in der Karte **Ressourcenlimits** unter
**Verwaltung → Plugins**. Laufende Plugins werden automatisch neu gestartet,
damit der neue Modus sofort gilt, und das **Sicherheits-Dashboard** zeigt den
geltenden Modus. Ein Plugin, das unter dem neuen Modus nicht laufen kann (zum
Beispiel `bubblewrap` in einem Container ohne Sonderrechte), erscheint als
fehlerhaft: Stellen Sie den Modus zurück.

## Prüfen, ob es funktioniert { #verify-it-is-working }

```bash
# der Launcher trägt die Capability für die UID-Trennung
docker exec evey-server getcap /evey-plugin-launcher        # cap_setuid=ep

# unter welchem Benutzer ein laufendes Plugin läuft (evey = mit dem Server geteilt,
# evey-plugin = getrennt); das Image hat ps, also funktioniert auch `docker exec evey-server ps`
docker top evey-server -o user,pid,args | grep -iE "node|python"
```

Der geltende Sandbox-Modus ist das Abzeichen unter **Verwaltung → Plugins →
Sicherheits-Dashboard**.

<!-- sources
auto resolution and probes: internal/adapters/driven/pluginrunner/runner_landlock.go - resolveAutoMode ; runner_sandbox.go - effectiveMode, SandboxStatus, uidSepApplies, WithUIDSeparation, applyBasicSandbox
defaults: internal/config/config.go (EVEY_PLUGINS_SANDBOX_MODE, EVEY_PLUGINS_UID_SEPARATION, EVEY_PLUGINS_MEMORY_MB, _PIDS_MAX, _DISK_MAX_MB, _CPU_NICE)
restart on mode change: internal/app/plugin/runtime_service.go - RestartRunningPlugins
image: Dockerfile (setcap cap_setuid+ep /evey-plugin-launcher, no procps) ; docker-compose.yml (no security_opt)
labels: i18n/en/admin.yaml - plugins.security.title, probeLandlock, probeLauncher, probeBubblewrap, plugins.resourceLimits.title, resourceLimits.sandboxMode
-->
