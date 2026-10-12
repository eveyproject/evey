---
source_sha: 1a4e824d130bb84b068353b3024d71ea59b8db2ff04ee70f219e637e02212468
reviewed: true
---
# Hardware-Beschleunigung

**Kurz gesagt:**

- Eine Grafikkarte, oder der Grafikteil, der in viele Prozessoren eingebaut
  ist (zum Beispiel Intel Quick Sync), lässt den Server mehr Videos
  gleichzeitig und in höheren Auflösungen umwandeln als der Prozessor allein.
- Sie hilft nur, wenn ein Video umgewandelt (transkodiert) wird. Filme, die
  unverändert gesendet oder nur neu verpackt werden, nutzen sie nicht.
- Was Sie tun: ein paar Zeilen in Ihre Compose-Datei schreiben, damit Evey die
  Karte benutzen kann, und die Einstellung dann auf **Auto (bestmöglich)**
  lassen.
- Evey testet die Karte mit einer echten Umwandlung, bevor es sie für eine
  Wiedergabe verwendet. Eine Karte, die den Test nicht besteht, wird nicht
  benutzt.

Hier folgen die Schritte, danach, wie der Server nachweist, was die Karte
kann.

## Einschalten

1. Öffnen Sie **Verwaltung → Transkodierung → Encode-Kalibrierung**.
   **Hardwarebeschleunigung** steht standardmäßig auf **Auto
   (bestmöglich)**, und die Zeile **Erkannt:** nennt die Beschleuniger, die
   der Host meldet.
2. Steht dort **Kein GPU-Beschleuniger erkannt**, geben Sie dem Container das
   Gerät (nächster Abschnitt) und erstellen Sie ihn neu.
3. Klicken Sie auf **Neu kalibrieren**, oder warten Sie: Der Server misst sich
   im ersten ruhigen Moment selbst. Die Karte zeigt dann das verwendete
   Backend und die Auflösungen, die für die GPU und für den Prozessor
   bestätigt sind.

Es gibt ein einziges Image für jeden Fall: Es ist kein Treiber für eine
bestimmte Karte fest eingebaut, und derselbe Container läuft nur mit Software,
mit einer Intel- oder AMD-GPU oder mit einer NVIDIA-Karte, je nachdem, welche
Geräte Sie ihm geben.

## Dem Container das Gerät geben

=== "Intel / AMD (VAAPI, Quick Sync)"

    ```yaml title="docker-compose.yml"
    services:
      evey:
        devices:
          - /dev/dri:/dev/dri
        group_add:
          - "104"   # die Gruppe render des Hosts: getent group render
    ```

    Die Gruppe muss die Gruppe `render` des Hosts sein
    (`getent group render` oder `stat -c %g /dev/dri/renderD128`). Ein
    falscher Wert macht nichts kaputt: Die GPU ist dann einfach nicht
    erreichbar, und der Server verwendet weiter den Prozessor.

=== "NVIDIA (NVENC)"

    Installieren Sie das NVIDIA Container Toolkit auf dem Host. Reservieren
    Sie dann die GPU und fordern Sie die Fähigkeit `video` an, ohne die die
    Encoder-Bibliothek nicht eingebunden wird:

    ```yaml title="docker-compose.yml"
    services:
      evey:
        environment:
          NVIDIA_DRIVER_CAPABILITIES: all   # muss "video" enthalten
        deploy:
          resources:
            reservations:
              devices:
                - driver: nvidia
                  count: all
                  capabilities: [gpu]
    ```

=== "Rockchip (RKMPP)"

    Binden Sie `/dev/mpp_service`, `/dev/dri` und, wo der Kernel es
    bereitstellt, `/dev/rga` ein. Siehe
    [ARM-Hosts](../operations/advanced-deployments.md#arm-hosts).

Eine NVIDIA-Reservierung und `/dev/dri` lassen sich auch gleichzeitig
einbinden.

## Was „durch eine echte Umwandlung nachgewiesen“ heißt

Mit **Auto** arbeitet der Server nur mit Messungen, nie mit dem Namen der
Karte:

1. **Erkennen.** Jeder Render-Knoten wird auf VAAPI und, bei Intel, auf Quick
   Sync geprüft; ebenso jede NVIDIA-GPU und der Rockchip-Encoder. Ein Encoder
   zählt nur, wenn eine kurze echte Umwandlung über ihn gelingt.
2. **Bestätigen.** Jeder Beschleuniger wandelt die Stufen der Auflösungen
   (720p bis 2160p) um, im selben Ablauf wie bei einer echten Wiedergabe, und
   behält die höchste Stufe, die er mit dem 1,2-Fachen der Echtzeit schafft.
   Einige Bilder jeder Probe werden zurück dekodiert und mit der Quelle
   verglichen, denn ein fehlerhafter Treiber kann ohne Fehlermeldung beenden und dabei
   grüne oder beschädigte Bilder liefern. Ein Beschleuniger, der 720p nicht
   schafft, wird nicht benutzt.
3. **Die Wege messen.** Für jede Fähigkeit, die der Beschleuniger
   nachgewiesen hat (SDR, HEVC, AV1, Tone-Mapping auf der GPU, HDR erhalten),
   werden die Kosten einer echten Sitzung gemessen.
4. **Pro Stream wählen.** Jede Wiedergabe geht an den günstigsten
   Beschleuniger, der nachgewiesen hat, was dieser Stream braucht, oder an den
   Prozessor. Siehe
   [Transkodierung und Serverkapazität](../playback/transcoding.md).

Die Messung wartet auf einen ruhigen Moment: kein laufender Stream und die
CPU des Hosts fünf Messungen in Folge, im Abstand von vier Sekunden, unter
20 %. So führen eine hohe Last beim Start oder ein Bibliotheks-Scan nicht
zu Obergrenzen, die zu niedrig sind. Sie läuft erneut, wenn sich die Hardware
ändert. **Neu kalibrieren** startet sie sofort: Nutzen Sie das, nachdem Sie
eine Karte eingebaut oder den Container auf einen anderen Rechner umgezogen
haben.

!!! note "Ein Fehler direkt nach dem Start ist nicht endgültig"
    Eine GPU, die sich in den ersten Sekunden nach dem Start des Containers
    nicht initialisieren lässt, wird in kurzen Abständen erneut geprüft, bevor
    irgendetwas festgehalten wird. Nur ein Fehler, der bleibt, oder einer von
    eindeutiger Art (fehlender Encoder, nicht unterstütztes Format), markiert
    den Beschleuniger als unbrauchbar.

## Ein Backend erzwingen oder ausschalten

**Aus (nur Software)** lässt jede Transkodierung auf dem Prozessor laufen.
Das ist die Ausweichlösung bei einem fehlerhaften Treiber. Wählen Sie ein
**Backend** von Hand (NVENC, VAAPI, QSV, RKMPP), wird dieses erzwungen. Stellt
es sich als unbrauchbar heraus, weicht der Server auf den Prozessor aus, und
die Karte zeigt das tatsächlich verwendete Backend.

Auf einem Rechner mit mehreren Beschleunigern hat Auto keine feste Rangfolge:
Jeder wird bestätigt und gemessen, und jeder Stream geht an das Backend, das
den benötigten Weg nachgewiesen hat, in der besten nachgewiesenen Qualität und
zu den niedrigsten gemessenen Kosten. Zwei Streams auf demselben Server können
also verschiedene Backends nutzen, zum Beispiel HDR-Tone-Mapping auf einer
Karte und eine HEVC-Umwandlung auf einer anderen.

## Prüfen, ob sie benutzt wird

**Verwaltung → Transkodierung → Encode-Kalibrierung** zeigt die erkannten
Beschleuniger, das verwendete Backend, die für die GPU und für den Prozessor
bestätigten Auflösungen und unter **Hardware-Fallbacks** die letzten
Umwandlungen, die auf den Prozessor ausgewichen sind, mit ihrem Grund. Während
einer Wiedergabe nennt **Verwaltung → Aktuelle Wiedergabe** das Backend, mit
dem jeder Stream umgewandelt wird.

Unter WSL2 funktioniert NVENC; VAAPI und Quick Sync nicht (der
WSL-Grafiktreiber scheitert bei diesen Umwandlungen), und der
Verwaltungsbereich kennzeichnet sie mit **unter WSL nicht unterstützt**.

<!-- sources
auto, certify, routes, recalibrate: docs/adr/0015-hardware-acceleration.md ; docs/adr/0029-transcoding-routes-are-measured-not-assumed.md
detection by real encode: internal/adapters/driven/ffmpeg/hwdetect.go - probeHWEncode
ladder, 1.2x realtime margin, 720p floor: internal/adapters/driven/ffmpeg/benchmark.go - hwBenchLadder, Margin
visual check: internal/adapters/driven/ffmpeg/hw_output_validation.go
cold-boot init retry: internal/adapters/driven/ffmpeg/hw_readiness.go - runHWReadinessProbe, hwReadinessRetryDelays
quiet moment: internal/app/transcoding/calibration_service.go - idleProbes, idleProbeEvery, idleCPUPercent
render group: docker-compose.gpu.yml ; nvidia: docker-compose.gpu-nvidia.yml
panel: frontend/apps/web/src/features/admin/EncodeCalibrationCard.tsx (hw_mode, detected_backends, is_wsl)
labels: i18n admin.calibration.cardTitle, hwModeLabel, hwModeAuto, hwModeOff, hwBackendLabel, detected, detectedNone, recalibrate, hwFallbacks, wslUnsupported ; admin.nav.transcodingConfig, nowPlaying
plain opening: rules below ; i18n admin.calibration.hwModeAuto
-->
