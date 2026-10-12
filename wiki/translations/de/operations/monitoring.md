---
source_sha: d6335afa014ad627b79829f653197424d1a5c4572cdf3b9832ed19a3f7a079ac
reviewed: true
---
# Überwachung und Protokolle

Wo Sie sehen, was der Server tut, für den Administrator, der nach dem Rechten
sehen will, statt etwas zu reparieren: von dem, was gerade läuft, bis zur
Statistik des Monats, mit den Protokollen dazwischen. Wenn Sie einem
bestimmten Fehler nachgehen, beginnen Sie mit der
[Fehlerbehebung](troubleshooting.md).

## Was gerade läuft

**Verwaltung → Aktuelle Wiedergabe** listet jeden laufenden Stream auf: wer,
was, auf welchem Gerät und wie, Spur für Spur (kopiert, transkodiert, und mit
welchem Backend umgewandelt). Ein Administrator kann eine Sitzung mit
**Stoppen** beenden oder dem Zuschauer mit **Nachricht senden** schreiben.

Die Karte **Wer ist online** im Dashboard listet auf, wer gerade verbunden
ist, nach Profil, und wie lange es her ist, dass sich jede App zuletzt
gemeldet hat. Ein Chromecast, dessen Fernseher Eveys Empfänger geladen hält,
bleibt in der Liste, auch wenn seine Wiedergabe beendet ist und aus
**Aktuelle Wiedergabe** verschwunden ist; **Diesen Chromecast stoppen**
schickt ihm einen Stopp-Befehl, den der Fernseher innerhalb von 30 Sekunden
ausführt.

**Verwaltung → Systemzustand** zeigt Prozessor, Arbeitsspeicher, Bandbreite
und Speicherplatz und wie viel der Kapazität für die Transkodierung belegt
ist.

<figure class="evey-shot" markdown="span">
  ![Verwaltung → Systemzustand auf dem Demo-Server](../images/admin-system-health.webp){ loading=lazy }
  <figcaption><strong>Verwaltung → Systemzustand</strong>, mit den Hintergrundarbeiten darunter (Oberfläche auf Englisch).</figcaption>
</figure>

## Protokolle

**Verwaltung → Protokolle & Debug** durchsucht das Protokoll des Servers und
folgt ihm live (**Live**), mit **Bereich** eingegrenzt auf einen Teil (Player
und Transkodierung, Seek-Index, Untertitel, Intro-Erkennung,
Chromecast, Anmeldung, Infrastruktur...). Hier sehen Sie zuerst nach, wenn
ein Benutzer ein Problem meldet.

Dasselbe Protokoll liegt auf der Festplatte, eine JSON-Datei pro Tag in
`./evey-data/logs/` (`/data/logs/evey-YYYY-MM-DD.log` im Container),
standardmäßig 90 Tage aufbewahrt. `docker compose logs evey` zeigt nur, was
der Container beim Start ausgibt. Das lesen Sie, wenn der Server nicht
hochkommt. Siehe [Fehlerbehebung](troubleshooting.md#the-logs).

## Wie die Wiedergabe wirklich lief

**Verwaltung → Streaming-Qualität** beruht auf dem, was die Player während der
Wiedergabe melden (Pufferstand, erneutes Puffern, Durchsatz, im Format CMCD):
wie oft Zuschauer gewartet haben, welche Titel am häufigsten hängen und welche
Arten von Geräten Mühe haben, über den letzten Tag oder die letzte Woche.

Der Server handelt auch danach. Lässt eine bestimmte Art von Quelle die
Zuschauer immer wieder warten, senkt er für eine Weile die höchste Qualität
für diese Art von Quelle, zusätzlich zur kalibrierten Obergrenze, und hebt die
Grenze danach von selbst wieder auf. Die aktiven Grenzen stehen unter
**Automatische Qualitätsgrenzen**, mit Grund und Ablaufzeit; **Zurücksetzen**
hebt eine sofort auf.

## Verlauf und Statistik

- **Verwaltung → Analysen**: Wiedergaben heute und in dieser Woche, aktive
  Benutzer, die meistgesehenen Titel der letzten 30 Tage.
- **Verwaltung → Aktivität**: der Verlauf der Wiedergaben und der
  Verwaltungsaktionen sowie die laufenden Hintergrundarbeiten.

Wie lange jede Art von Verlauf aufbewahrt wird, legen Sie unter **Verwaltung
→ Serverkonfiguration → Servereinstellungen** fest (Wiedergabesitzungen,
Audit-Protokoll, Servermesswerte, Streaming-Qualität). Eine Änderung gilt ab
der nächsten täglichen Bereinigung.

## Prometheus-Metriken

`GET /metrics` liefert die Zähler des Servers im Textformat von Prometheus,
alle mit Namen `evey_...`. Es antwortet einer Administratorsitzung oder einem
festen Token, das in `EVEY_METRICS_SCRAPE_TOKEN` gesetzt und als
`Authorization: Bearer <token>` gesendet wird, damit Prometheus oder Grafana
die Werte abrufen können, ohne sich neu anzumelden
([Sicherungen, Protokolle und Überwachung](../environment/logs-monitoring.md)).
Die Gruppen, die sich für ein erstes Diagramm lohnen:

| Metrik | Was sie zeigt |
| --- | --- |
| `evey_transcodes_active`, `evey_transcode_queue_depth` | Transkodierungsaufträge im Hintergrund (außerhalb der laufenden Wiedergabe), die laufen und die auf einen freien Platz warten. |
| `evey_hlsvod_sessions_active`, `evey_hlsvod_sessions_rejected_total` | FFmpeg-Prozesse, die Segmente für die Wiedergabe erzeugen, und Starts, die abgelehnt wurden, weil der Server an seiner Grenze war. |
| `evey_hlsvod_cache_size_bytes`, `evey_hlsvod_cache_evicted_bytes_total` | Größe des Segment-Caches auf der Festplatte und was die Bereinigung freigegeben hat. |
| `evey_mkvgo_walks_in_flight`, `evey_mkvgo_plan_stats` | Der eingebaute Packager: Abschnitte, die gerade gelesen werden, Lesevorgänge in der Warteschlange (`stat="slot_waits"`), siehe [MKVGo in Evey](../playback/mkvgo.md#load-replaced-files-and-updates). |
| `evey_http_requests_total`, `evey_http_request_duration_seconds` | Anfragen und Antwortzeit nach Methode und Pfad. |
| `evey_watcher_alive`, `evey_scan_queue_depth` | Ob die Ordnerüberwachung jeder Bibliothek auf ihre letzte Prüfung geantwortet hat (1 oder 0), und Dateien, die auf den Scan warten. |

## Kommandozeile

`evey admin doctor` prüft den ganzen Server in einem Bericht; die anderen
Befehle zur Diagnose stehen unter [Kommandozeile](../runbooks/cli.md).

<!-- sources
who's online: frontend/apps/web/src/features/admin/DashboardOnline.tsx (presence heartbeat, profile names, idle, SendCastReceiverCommand) ; i18n admin.online.*
logs: internal/pkg/logger/logger.go - New (file only outside DevMode), rotatingWriter ; config EVEY_LOG_RETENTION_DAYS ; docker-compose.yml ./evey-data:/data
labels: i18n admin.sessions.*, admin.systemHealth.*, admin.logs.where.*, admin.logs.live, admin.qoe.*, admin.analytics.*, admin.activity.*, admin.serverSettings.*RetentionDays, retentionSweepHint
automatic limits: i18n admin.qoe.overrides.*
metrics: internal/pkg/metrics/metrics.go (names) ; internal/adapters/driving/http/auth_middleware.go - httpScrapeOrAdminMiddleware (Bearer scrape token or admin JWT) ; docs/adr/0018-cmcd-qoe-pipeline.md
-->
