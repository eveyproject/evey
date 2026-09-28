---
source_sha: 0846a653f87edb54addf042a0876b46fd3ab9e42930e3f12e27706e59ce0bc97
---
# Sandbox für Erweiterungen - welches Setup für Ihren Server

Evey führt Erweiterungen von Dritten in einer Sandbox aus. **Für fast alle lautet
die Antwort: Lassen Sie die Voreinstellungen.** Der Server erkennt Ihren Rechner
und wählt automatisch die stärkste Isolation, die er bieten kann. Lesen Sie nur
weiter, wenn Sie ein **altes NAS** betreiben, Evey **ohne Docker** laufen lassen
oder **noch weiter abdichten** wollen.

Das vollständige technische Modell steht in [SECURITY.md](../SECURITY.md). Diese
Seite ist die Entscheidungshilfe in einfachen Worten.

---

## Was Sie schützt (in einfachen Worten)

Eine Erweiterung, die Sie installieren, ist eingesperrt. Je nach Rechner:

- **Begrenzt der Käfig die Dateien, die sie anfassen kann** - sie sieht ihren
  eigenen Ordner und die Systembibliotheken, die sie zum Laufen braucht, sonst
  nichts (nicht Ihre Medien, nicht Ihre Geheimnisse, nicht andere Erweiterungen).
- **Blockiert er gefährliche Systemaktionen** - andere Prozesse debuggen,
  Kernelmodule laden, die Uhr stellen, Datenträger einhängen usw. lehnt der
  Kernel ab.
- **Kappt er das Netzwerk**, sofern die Erweiterung es nicht ausdrücklich
  verlangt hat (und Sie diese Berechtigung genehmigt haben).
- **Deckelt er CPU, Speicher, Plattenplatz und Prozesszahl**, damit eine
  außer Kontrolle geratene Erweiterung den Server nicht lahmlegen kann.
- **Hält er Ihre Geheimnisse unlesbar** - den Schlüssel, der Anmeldungen
  signiert, und die Sicherungen.

Obendrein: Erweiterungen kann **nur der Administrator installieren**, sie werden
auf verdächtigen Code **gescannt**, und Sie **genehmigen ihre Berechtigungen**,
bevor sie laufen.

**Prüfen, was tatsächlich gilt:** Admin → Plugins → Security Dashboard zeigt die
*Host-Sandbox* - den Modus, zu dem `auto` aufgelöst wurde (`landlock`,
`bubblewrap` oder `basic`), mit den einzelnen Prüfergebnissen. Zeigt sie
**basic**, bietet Ihr Host keine Dateisystem-Isolation pro Pfad (Erweiterungen
laufen nur mit UID-Wechsel + seccomp); die Prüfungen sagen, warum, und die
Abschnitte unten, wie Sie es beheben.

---

## Wählen Sie Ihre Situation

| Ihr Server | Was Sie ohne weiteres Zutun bekommen | Etwas zu tun? |
|---|---|---|
| **Aktuelles Linux in Docker** (Kernel ≥ 5.13 - praktisch alles ab ~2021) - *der Normalfall* | **Stärkste Isolation** (Landlock), automatisch. | **Nichts.** Voreinstellungen lassen. |
| **Altes NAS in Docker** (z. B. älteres Synology, Kernel < 5.13) | Gute Isolation, **ohne die Dateiwände pro Ordner**. Geheimnisse, Netzwerk und Systemaktionen bleiben abgeriegelt. | Optional: **UID-Trennung** einschalten (siehe unten), als doppelte Absicherung der Geheimnisse. |
| **Beliebiges Linux, ohne Docker** (Bare Metal) | **Stärkste Isolation**, automatisch (Landlock auf einem aktuellen Kernel, sonst bubblewrap-Namespaces). | Folgen Sie der [Anleitung ohne Container](../deployment-bare-metal.md). |

So erfahren Sie, welcher Fall bei Ihnen vorliegt: Öffnen Sie **Admin → Plugins** -
das Abzeichen im Sicherheitsbereich zeigt den tatsächlich geltenden
Sandbox-Modus: `landlock` oder `bubblewrap` heißt stark, `basic` heißt der Fall
„altes NAS“.

---

## Der eine Regler, den Sie vielleicht anfassen: UID-Trennung

**Was sie tut, einfach gesagt:** Sie lässt eine Erweiterung unter einem **anderen
Benutzerkonto** als Evey selbst laufen, sodass sie Eveys geheime Dateien physisch
nicht lesen kann - selbst wenn nichts anderes sie aufhielte.

**Wann sie zählt:** nur auf einem **alten Rechner ohne die Dateiwände pro Ordner**
(also im Fall `basic`). Auf aktuellen Rechnern halten die Dateiwände die
Geheimnisse schon außer Reichweite; dort ist die UID-Trennung nur eine
zusätzliche, redundante Schicht.

**Die Einstellung** - `EVEY_PLUGINS_UID_SEPARATION`:

| Wert | Bedeutung | Verwenden, wenn |
|---|---|---|
| `auto` *(Standard)* | Schaltet sich **nur ein, wenn sie wirklich gebraucht wird** (der Fall `basic`). Aus, wo sie redundant ist. | **Lassen Sie das** - es tut auf jedem Rechner das Richtige. |
| `on` | **Immer an**, auch auf aktuellen Rechnern - eine zweite, unabhängige Wand um die Geheimnisse, zusätzlich zu den Dateiwänden. | Sie wollen maximale doppelte Absicherung und nehmen den kleinen Preis unten in Kauf. |
| `off` | **Nie.** | Eine bestimmte Erweiterung verhält sich darunter falsch (selten). |

**Der Haken:** Die UID-Trennung ist **unverträglich mit der Docker-Härtung
`no-new-privileges`** - entscheiden Sie sich für eines. Kann Ihr Setup sie auf
einem alten Rechner (`basic`) nicht gewähren, **weigert sich Evey, die
Erweiterung zu starten**, statt sie neben Ihren Geheimnissen laufen zu lassen;
auf einem aktuellen Rechner schreibt es eine Warnung und startet die Erweiterung
normal (Sie bekommen nur die zusätzliche Wand nicht). Das offizielle Image bringt
schon alles Nötige mit; Sie setzen nur diese Variable.

**Der kleine Preis von `on`:** Die Erweiterung läuft unter einem eigenen Konto,
sodass eine Erweiterung, die Hilfsdateien *in ihren eigenen Ordner* schreibt,
stolpern könnte. Passiert das, setzen Sie `off`. (Deshalb ist `auto` der
Standard - es geht dieses Risiko nur dort ein, wo es einen echten Nutzen gibt.)

---

## Die Risiken, in einfachen Worten

Was eine **bösartige** Erweiterung tun könnte und was nicht - je nach Szenario:

**Aktuelles Linux (Docker oder Bare Metal):**
Eine böse Erweiterung ist vollständig eingesperrt. Sie **kann** Ihre Geheimnisse
**nicht** lesen, die Dateien anderer Erweiterungen **nicht** sehen oder
anfassen, **keine** beliebigen Netzwerkaufrufe machen und das System **nicht**
manipulieren. Der einzige realistische Ausweg ist ein ernster Fehler im
Linux-**Kernel** - das gilt für *jede* Sandbox, die keine vollständige virtuelle
Maschine ist.

**Altes NAS in Docker, ohne Wände pro Ordner (`basic`):**
- ✅ Ihre **Geheimnisse bleiben geschützt** (Anmeldeschlüssel, Sicherungen) -
  *besonders, wenn Sie die UID-Trennung auf `on` stellen*.
- ✅ Die Erweiterung bekommt **kein Netzwerk**, sofern nicht erlaubt.
- ✅ **Gefährliche Systemaktionen bleiben** im Kernel **blockiert**.
- ⚠️ Ohne Wände pro Ordner **könnte** eine böse Erweiterung aber **gewöhnliche,
  nicht geheime Dateien lesen**, **laufende Prozesse auflisten** und - weil sich
  Erweiterungen ein Konto teilen - möglicherweise **die Dateien einer anderen
  Erweiterung lesen oder stören**.
- **Fazit:** Installieren Sie auf so einem Host nur Erweiterungen, denen Sie
  vertrauen, oder wechseln Sie für die vollen Dateiwände auf einen aktuellen
  Kernel.

---

## Konfigurationsbeispiele (zum Kopieren)

Diese gehören in Ihre `.env` (Docker Compose) oder in `EVEY_*`-Umgebungsvariablen.

**1. Normaler aktueller Server - empfohlen (nichts tun):**
```bash
# Defaults are correct. Optionally make it explicit:
EVEY_PLUGINS_SANDBOX_MODE=auto          # picks the strongest available
# EVEY_PLUGINS_UID_SEPARATION=auto      # (default) on only where needed
```

**2. Altes NAS, Geheimnisse maximal schützen:**
```bash
EVEY_PLUGINS_SANDBOX_MODE=auto
EVEY_PLUGINS_UID_SEPARATION=on          # force the secret wall on
# and do NOT set no-new-privileges in your compose security_opt
```

**3. Maximale doppelte Absicherung auf einem aktuellen Server (beide Wände):**
```bash
EVEY_PLUGINS_SANDBOX_MODE=auto          # → landlock
EVEY_PLUGINS_UID_SEPARATION=on          # add the independent uid wall on top
```

**4. Einen bestimmten Modus festlegen (fortgeschritten):**
```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock      # refuse to start plugins if Landlock is unavailable (fail‑closed)
```

**5. Eine Erweiterung funktioniert unter UID-Trennung nicht:**
```bash
EVEY_PLUGINS_UID_SEPARATION=off
```

**6. Ressourcengrenzen (alle später unter Admin → Plugins änderbar):**
```bash
EVEY_PLUGINS_MEMORY_MB=256              # max RAM per plugin
EVEY_PLUGINS_PIDS_MAX=64                # max processes/threads
EVEY_PLUGINS_DISK_MAX_MB=100            # max install‑dir size (0 = off)
EVEY_PLUGINS_CPU_NICE=10                # scheduling priority 0-19 (higher = nicer)
```

---

## Den Sandbox-Modus im laufenden Betrieb ändern

Sie können den Modus über die Karte **Resource limits** unter **Admin → Plugins**
ändern. Laufende Erweiterungen werden **automatisch neu gestartet**, damit der
neue Modus sofort greift (kein manuelles Deaktivieren/Aktivieren). Das
Sicherheitsabzeichen zeigt den tatsächlich geltenden Modus. Kann eine
Erweiterung unter dem neuen Modus nicht laufen (z. B. wenn Sie unter Docker
`bubblewrap` gewählt haben, das dort Privilegien braucht), erscheint sie im
Dashboard als **fehlerhaft** - schalten Sie einfach zurück.

---

## Prüfen, ob es funktioniert

```bash
# the launcher carries the capability used for uid separation:
docker exec evey-server getcap /evey-plugin-launcher        # → cap_setuid=ep

# which user a running plugin runs as (evey = shared with server, evey-plugin = separated):
docker exec evey-server ps -o user,pid,args | grep -v grep | grep -iE "node|python"

# the sandbox mode actually in force: the Security badge on the Admin → Plugins page.
```

---

## Siehe auch

- [SECURITY.md](../SECURITY.md) - das vollständige Sicherheitsmodell der
  Erweiterungen (jede Schicht, die Tabelle je Modus, bekannte Grenzen).
- [deployment-bare-metal.md](../deployment-bare-metal.md) - Betrieb ohne Docker.
- [installation.md](../installation.md) - die Anleitung zum Selbsthosten mit
  Docker.
