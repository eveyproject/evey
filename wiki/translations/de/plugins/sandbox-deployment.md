---
source_sha: a6f8003c3c1502fd56c9686aaf41877fd1dc643a86c7f3eb3cb6910f60615fd6
reviewed: false
---
# Plugin-Sandbox - welche Einrichtung für Ihren Server

Evey führt Plugins von Dritten in einer Sandbox aus. **Für fast alle lautet die
Antwort: Standardeinstellungen lassen.** Der Server erkennt Ihre Maschine und
wählt automatisch die stärkste mögliche Isolation. Lesen Sie nur weiter, wenn Sie
ein **altes NAS** betreiben, Evey **ohne Docker** betreiben oder **noch weiter
absichern** wollen.

Das vollständige technische Modell steht in [SECURITY.md](../SECURITY.md). Diese
Seite ist die allgemeinverständliche Entscheidungshilfe.

---

## Was Sie schützt (in einfachen Worten)

Ein installiertes Plugin wird eingesperrt. Der Käfig, je nach Maschine:

- **Begrenzt, welche Dateien es anfassen kann** - es sieht seinen eigenen Ordner
  und die Systembibliotheken, die es zum Laufen braucht, sonst nichts (nicht Ihre
  Medien, nicht Ihre Geheimnisse, nicht andere Plugins).
- **Blockiert gefährliche Systemaktionen** - andere Prozesse debuggen,
  Kernelmodule laden, die Uhr stellen, Datenträger einbinden usw. werden vom
  Kernel verweigert.
- **Kappt den direkten Netzwerkzugang**, immer. Ein Plugin, das Netzwerk verlangt
  hat (und das Sie genehmigt haben), erreicht nur die Hosts, die es angegeben
  hat, über den Server.
- **Begrenzt CPU, Speicher, Festplatte und Prozessanzahl**, damit ein
  entgleistes Plugin den Server nicht lahmlegen kann.
- **Hält Ihre Geheimnisse unlesbar** - den Schlüssel, der Anmeldungen signiert,
  und die Sicherungen.

Dazu kommt: Plugins können **nur Administratoren installieren**, sie werden auf
verdächtigen Code **gescannt**, und Sie **genehmigen ihre Berechtigungen**, bevor
sie laufen.

**Prüfen, was tatsächlich gilt:** Admin → Plugins → Sicherheits-Dashboard zeigt
die *Host-Sandbox* - den Modus, zu dem `auto` aufgelöst wurde (`landlock`,
`bubblewrap` oder `basic`), mit den einzelnen Prüfergebnissen. Zeigt es
**basic**, bietet Ihr Host keine Isolation pro Dateisystempfad (Plugins laufen
nur mit UID-Herabstufung und seccomp); die Prüfungen sagen Ihnen warum, und die
Abschnitte unten, wie Sie es beheben.

---

## Ihre Situation wählen

| Ihr Server | Was Sie ohne Zutun bekommen | Etwas zu tun? |
|---|---|---|
| **Modernes Linux in Docker** (Kernel ≥ 5.13 - im Grunde alles ab ~2021) - *der Normalfall* | **Stärkste Isolation** (Landlock), automatisch. | **Nichts.** Standards lassen. |
| **Altes NAS in Docker** (z. B. ältere Synology, Kernel < 5.13) | Gute Isolation, **ohne die Dateiwände pro Ordner**. Geheimnisse, Netzwerk und Systemaktionen bleiben verriegelt. | **Nichts.** Die UID-Trennung (unten) schaltet sich in diesem Fall selbst ein. |
| **Beliebiges Linux, ohne Docker** | **Stärkste Isolation**, automatisch (Landlock auf einem modernen Kernel, sonst bubblewrap-Namespaces). | Folgen Sie der [Anleitung ohne Container](../deployment-bare-metal.md). |

Woran Sie erkennen, was Sie haben: Öffnen Sie **Admin → Plugins** - das Abzeichen
des Sicherheitsbereichs zeigt den tatsächlich geltenden Sandbox-Modus: `landlock`
oder `bubblewrap` heißt stark, `basic` heißt der Fall „altes NAS".

---

## Der eine Regler, den Sie anfassen könnten: die UID-Trennung

**Was sie tut, einfach gesagt:** Sie lässt ein Plugin unter einem **anderen
Benutzerkonto** als Evey selbst laufen, sodass es die geheimen Dateien von Evey
physisch nicht lesen kann - selbst wenn nichts anderes es aufhielte.

**Wann sie zählt:** nur auf einer **alten Maschine ohne die Dateiwände pro
Ordner** (also im Fall `basic`). Auf modernen Maschinen halten die Dateiwände die
Geheimnisse bereits fern, dort ist die UID-Trennung nur eine redundante
zusätzliche Schicht.

**Die Einstellung** - `EVEY_PLUGINS_UID_SEPARATION`:

| Wert | Bedeutung | Nutzen, wenn |
|---|---|---|
| `auto` *(Standard)* | Schaltet sich **nur ein, wenn sie wirklich gebraucht wird** (der Fall `basic`). Aus, wo sie redundant ist. | **So lassen** - sie tut auf jeder Maschine das Richtige. |
| `on` | **Immer an**, auch auf modernen Maschinen - eine zweite, unabhängige Mauer um die Geheimnisse, zusätzlich zu den Dateiwänden. | Sie wollen doppelte Absicherung und nehmen die kleinen Kosten unten in Kauf. |
| `off` | **Nie.** | Ein bestimmtes Plugin verhält sich darunter falsch (selten). |

**Der Haken:** Die UID-Trennung ist **nicht mit der Docker-Härtungsoption
`no-new-privileges` vereinbar** - wählen Sie eines von beiden. Kann Ihre
Einrichtung sie auf einer alten Maschine (`basic`) nicht gewähren, **verweigert
Evey den Start des Plugins**, statt es neben Ihren Geheimnissen laufen zu lassen;
auf einer modernen Maschine protokolliert es eine Warnung und führt das Plugin
normal aus (Sie bekommen nur die zusätzliche Mauer nicht). Das offizielle Image
bringt schon alles Nötige mit; Sie setzen nur diese Variable.

**Kleine Kosten von `on`:** Das Plugin läuft unter einem eigenen Konto, sodass
ein Plugin, das Arbeitsdateien *in seinen eigenen Ordner* schreibt, stolpern
könnte. Tut eines das, setzen Sie `off`. (Deshalb ist der Standard `auto` - er
geht dieses Risiko nur dort ein, wo es einen echten Nutzen gibt.)

---

## Die Risiken, in einfachen Worten

Was ein **bösartiges** Plugin tun könnte und was nicht - nach Szenario:

**Modernes Linux (Docker oder ohne Container):**
Ein schlechtes Plugin ist vollständig eingesperrt. Es **kann** Ihre Geheimnisse
**nicht** lesen, die Dateien anderer Plugins **nicht** sehen oder anfassen,
**keine** beliebigen Netzwerkaufrufe machen und das System **nicht** manipulieren.
Der einzige realistische Ausweg ist ein schwerer Fehler im Linux-**Kernel** - das
gilt für *jede* Sandbox, die keine vollständige virtuelle Maschine ist.

**Altes NAS in Docker, ohne Wände pro Ordner (`basic`):**
- ✅ Ihre **Geheimnisse bleiben geschützt** (Anmeldeschlüssel, Sicherungen), durch
  das eigene Benutzerkonto, das `auto` hier einschaltet.
- ✅ Das Plugin hat **kein direktes Netzwerk**; ein erlaubtes erreicht nur die
  Hosts, die es angegeben hat.
- ✅ **Gefährliche Systemaktionen bleiben** im Kernel **blockiert**.
- ⚠️ Ohne Wände pro Ordner **könnte** ein schlechtes Plugin aber **gewöhnliche,
  nicht geheime Dateien lesen**, **laufende Prozesse auflisten** und - weil sich
  Plugins ein Konto teilen - möglicherweise **die Dateien eines anderen Plugins
  lesen oder stören**.
- **Unterm Strich:** Installieren Sie auf so einem Host nur Plugins, denen Sie
  vertrauen, oder wechseln Sie auf einen modernen Kernel für die vollen
  Dateiwände.

---

## Konfigurationsbeispiele (zum Kopieren)

Diese gehören in Ihre `.env` (Docker Compose) oder als `EVEY_*`-Umgebungsvariablen.

**1. Normaler moderner Server - empfohlen (nichts tun):**
```bash
# Die Standards passen. Optional ausdrücklich setzen:
EVEY_PLUGINS_SANDBOX_MODE=auto          # wählt das stärkste verfügbare
# EVEY_PLUGINS_UID_SEPARATION=auto      # (Standard) nur wo nötig an
```

**2. Altes NAS:** nichts zu setzen, `auto` führt Plugins dort bereits unter dem
eigenen Konto aus. Halten Sie `no-new-privileges` aus dem `security_opt` Ihrer
Compose-Datei heraus, es würde das verhindern:
```bash
EVEY_PLUGINS_SANDBOX_MODE=auto
EVEY_PLUGINS_UID_SEPARATION=auto        # (Standard) im Fall basic an
```

**3. Doppelte Absicherung auf einem modernen Server (beide Mauern stapeln):**
```bash
EVEY_PLUGINS_SANDBOX_MODE=auto          # → landlock
EVEY_PLUGINS_UID_SEPARATION=on          # die unabhängige UID-Mauer obendrauf
```

**4. Einen bestimmten Modus festlegen (fortgeschritten):**
```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock      # Plugins nicht starten, wenn Landlock fehlt (fail-closed)
```

**5. Ein Plugin bricht unter der UID-Trennung:**
```bash
EVEY_PLUGINS_UID_SEPARATION=off
```

**6. Ressourcengrenzen (alle später unter Admin → Plugins änderbar):**
```bash
EVEY_PLUGINS_MEMORY_MB=256              # max. RAM pro Plugin
EVEY_PLUGINS_PIDS_MAX=64                # max. Prozesse/Threads
EVEY_PLUGINS_DISK_MAX_MB=100            # max. Größe des Installationsordners (0 = aus)
EVEY_PLUGINS_CPU_NICE=10                # Scheduling-Priorität 0-19 (höher = zurückhaltender)
```

---

## Den Sandbox-Modus im laufenden Betrieb ändern

Sie können den Modus auf der Karte **Ressourcenlimits** unter **Admin → Plugins**
ändern. Laufende Plugins werden **automatisch neu gestartet**, damit der neue
Modus sofort gilt (kein manuelles Deaktivieren/Aktivieren). Das
Sicherheitsabzeichen zeigt den tatsächlich geltenden Modus. Kann ein Plugin unter
dem neuen Modus nicht laufen (z. B. wenn Sie `bubblewrap` in Docker gewählt
haben, wo es Rechte braucht), erscheint es im Dashboard als **fehlerhaft** -
schalten Sie einfach zurück.

---

## Prüfen, dass es funktioniert

```bash
# der Launcher trägt die Capability für die UID-Trennung:
docker exec evey-server getcap /evey-plugin-launcher        # → cap_setuid=ep

# unter welchem Benutzer ein laufendes Plugin läuft (evey = mit dem Server geteilt, evey-plugin = getrennt):
docker exec evey-server ps -o user,pid,args | grep -v grep | grep -iE "node|python"

# der tatsächlich geltende Sandbox-Modus: das Sicherheitsabzeichen auf der Seite Admin → Plugins.
```

---

## Siehe auch

- [SECURITY.md](../SECURITY.md) - das vollständige Sicherheitsmodell der Plugins
  (jede Schicht, die Tabelle pro Modus, bekannte Grenzen).
- [deployment-bare-metal.md](../deployment-bare-metal.md) - Betrieb ohne Docker.
- [installation.md](../installation.md) - die Anleitung zum Selbsthosten mit Docker.
