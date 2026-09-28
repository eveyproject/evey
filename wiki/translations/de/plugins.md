---
source_sha: d7ed6dd50005bc7e22c522df514264be5d132bc0ca91bd3052d5321a026acad0
---
# Erweiterungssystem

Erweiterungen rüsten den Server um Metadatenanbieter, Medienverarbeitung,
Webhooks, Benachrichtigungen, Frontend-Module und CSS-Themes nach. Eine
Erweiterung läuft als **abgeschotteter Unterprozess** und spricht mit dem Host
über gRPC auf einem Unix-Domain-Socket; bringt sie eine Oberfläche mit, wird
diese in einem **abgeschotteten iframe** dargestellt und erreicht den Host über
eine Brücke aus `postMessage`.

Diese Seite ist eine Übersicht. Die Einzelheiten stehen in drei eigenen
Dokumenten:

| Dokument | Was darin steht |
|---|---|
| **[plugins/dev-setup.md](plugins/dev-setup.md)** | Die erste eigene Erweiterung schreiben: Voraussetzungen, Gerüst, die Schleife aus Installieren und Nachbessern, Signieren, Verbreiten, Fehlersuche. **Hier anfangen.** |
| **[plugins/api-reference.md](plugins/api-reference.md)** | Der Vertrag: Schema des Manifests, IPC und Datenspeicher im Backend, die Frontend-Brücke `evey.*`, die Liste der Slots und jede einzelne Fähigkeit (`metadata.provider`, `media.read/write`, `notification.sender`, `webhook.receiver`, `public`, `prefs`). |
| **[plugins/security-model.md](plugins/security-model.md)** | Bedrohungsmodell und Schutzschichten: UID-Trennung, die seccomp-Sperrliste im Kernel und über LD_PRELOAD, die Sandbox-Modi (`auto` arbeitet landlock → bubblewrap → basic ab; `strict` ergänzt bubblewrap um die LD_PRELOAD-Bibliothek), der Ressourcenmonitor im Userspace, die statische Analyse, die CSS-Bereinigung, Signaturen und Versionierung, die Positivliste der Slots sowie die bekannten Grenzen. |

Wie die Rechtevergabe hinter den Tokens von Erweiterungen funktioniert
(standardmäßig verweigert, fest verriegelte Scopes, die Brücke, über die nie ein
Token in ein iframe gelangt), steht in
[adr/0012-plugin-token-deny-by-default.md](adr/0012-plugin-token-deny-by-default.md);
die Ressourcengrenzen behandelt
[adr/0013-userspace-plugin-resource-monitor.md](adr/0013-userspace-plugin-resource-monitor.md).

## Architektur

```
+------------------+      gRPC (Unix socket)      +-------------------+
|   Evey Server    | <--------------------------> |  Plugin Process   |
|                  |   /tmp/evey/plugins/{id}/    |                   |
|  HostService     |   plugin.sock                |  PluginService    |
|  (serves RPCs)   |                              |  (implements)     |
+------------------+                              +-------------------+
        |                                                  |
        v                                                  v
+------------------+                              +-------------------+
|  Permission /    |                              |  SDK stubs        |
|  scope checker   |                              |  (multi-language) |
+------------------+                              +-------------------+
        |
        v
+------------------+     iframe sandbox           +-------------------+
|  Frontend        | <--------------------------> |  Frontend module  |
|  PluginSlot      |   postMessage bridge         |  (in srcdoc)      |
+------------------+                              +-------------------+
```

Eine Erweiterung dürfen Sie in **jeder Sprache schreiben, die sich über
`entrypoint:` starten lässt** - der Runner schränkt Sie dabei nicht ein. Fertig
erzeugte gRPC-Stubs für das SDK liegen für Python, Node.js, PHP, Ruby, Dart,
Java und Swift bei (`sdk/<lang>/`, bei der Installation automatisch
mitkopiert); für Go gibt es keine mitgelieferten Stubs, die erzeugen Sie mit
`protoc-gen-go` und `protoc-gen-go-grpc`. Interpretersprachen ohne
mitgelieferte Stubs (Bash, Lua, Perl, …) funktionieren, indem Sie den
Interpreter unmittelbar aufrufen, und kompilierte Binärdateien (Go, Rust, …)
brauchen die Berechtigung `native-binary`. Die Einrichtung je Sprache:
[plugins/dev-setup.md](plugins/dev-setup.md#per-language).

Die drei Säulen:

- **Fähigkeiten und Scopes** - eine Erweiterung führt in ihrem Manifest auf,
  welche Fähigkeiten sie braucht; erteilt werden sie vom Administrator bei der
  Installation. Der Host setzt sie bei jedem Aufruf durch und verweigert im
  Zweifel. Der vollständige Katalog steht in
  [plugins/api-reference.md](plugins/api-reference.md#capabilities).
- **Sandbox** - die Prozesse im Backend laufen abgeschottet (eingehegt bei
  Dateisystem, Systemaufrufen und Netz) und unter einem Ressourcenmonitor im
  Userspace; der Frontend-Code läuft in einem streng verriegelten iframe. Die
  Einzelheiten: [plugins/security-model.md](plugins/security-model.md).
- **Vertrauen** - Erweiterungen sind über SHA256 festgeschrieben und lassen sich
  mit Cosign signieren; der Server kann so eingestellt werden, dass er eine
  Signatur verlangt. Siehe
  [plugins/security-model.md](plugins/security-model.md#update-workflow).

## Das Subsystem einschalten

Erweiterungen sind standardmäßig an (`EVEY_PLUGINS_ENABLED=true`). Der
Sandbox-Modus steht in `EVEY_PLUGINS_SANDBOX_MODE` (Standardwert `auto`, was im
offiziellen Image auf Landlock hinausläuft). Die Ressourcengrenzen
(`EVEY_PLUGINS_MEMORY_MB`, `_PIDS_MAX`, `_DISK_MAX_MB`, `_CPU_NICE`) sind unter
**Admin → Plugins** änderbar.
Siehe [configuration.md](configuration.md) und [plugins/dev-setup.md](plugins/dev-setup.md#7-resource-limits-and-sandbox).
