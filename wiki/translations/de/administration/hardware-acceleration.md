---
source_sha: c5dbea370fd3437769623f35f8bb5f32be76bca382bacfd248fac739d7bb12e9
reviewed: false
---
# Hardwarebeschleunigung

Eine Grafikkarte oder eine integrierte GPU lässt den Server mehr Streams
gleichzeitig und in höheren Auflösungen transkodieren als der Prozessor
allein. Es gibt ein Image für jeden Fall: Kein Treiber für eine bestimmte Karte
ist eingebaut, und derselbe Container läuft nur mit Software, mit einer Intel-
oder AMD-GPU oder mit einer NVIDIA-Karte, je nachdem, welche Geräte Sie ihm
geben.

Zwei Schritte: das Gerät für den Container freigeben, dann
**Hardware-Beschleunigung** unter **Admin → Transkodierung →
Encode-Kalibrierung** auf **Auto (bestmöglich)** lassen.

## Das Gerät freigeben

=== "Intel / AMD (VAAPI, Quick Sync)"

    ```yaml title="docker-compose.yml"
    services:
      evey:
        devices:
          - /dev/dri:/dev/dri
        group_add:
          - "104"   # die render-Gruppe des Hosts: getent group render
    ```

    Die Gruppe muss die `render`-Gruppe des Hosts sein (`getent group render`
    oder `stat -c %g /dev/dri/renderD128`). Ein falscher Wert macht nichts
    kaputt: Die GPU ist dann einfach nicht erreichbar und der Server bleibt auf
    dem Prozessor.

=== "NVIDIA (NVENC)"

    Installieren Sie das NVIDIA Container Toolkit auf dem Host, reservieren Sie
    dann die GPU und fordern Sie die Fähigkeit `video` an, ohne die die
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

    Reichen Sie `/dev/mpp_service`, `/dev/dri` und, wo der Kernel es
    bereitstellt, `/dev/rga` durch. Siehe
    [ARM-Hosts](../operations/advanced-deployments.md#arm-hosts).

Eine NVIDIA-Reservierung und `/dev/dri` können gleichzeitig durchgereicht
werden.

## Was Auto macht

Mit **Auto** arbeitet der Server nur mit Messungen, nie mit dem Namen der
Karte:

1. **Erkennen.** Jeder Render-Knoten wird auf VAAPI und, bei Intel, auf Quick
   Sync geprüft; ebenso jede NVIDIA-GPU und der Rockchip-Encoder. Ein Encoder
   zählt nur, wenn eine echte Kodierung gelingt.
2. **Zertifizieren.** Jeder Beschleuniger kodiert die Auflösungsstufen (720p
   bis 2160p) in derselben Pipeline-Form wie eine echte Wiedergabe, behält die
   höchste Stufe, die er mit dem 1,2-Fachen der Echtzeit hält, und besteht eine
   Sichtprüfung. Ein Beschleuniger, der 720p nicht hält, wird nicht verwendet.
3. **Die Wege messen.** Für jede Fähigkeit, die der Beschleuniger bewiesen hat
   (SDR, HEVC, AV1, Tone Mapping auf der GPU, beibehaltenes HDR), werden die
   Kosten einer echten Sitzung gemessen.
4. **Pro Stream wählen.** Jede Wiedergabe geht an den günstigsten
   Beschleuniger, der bewiesen hat, was dieser Stream braucht, oder an den
   Prozessor. Siehe [Transkodierung und Serverkapazität](../playback/transcoding.md).

Die Messung läuft im ersten ruhigen Moment des Servers und erneut, wenn sich die
Hardware ändert. **Neu kalibrieren** führt sie sofort aus: Nutzen Sie es nach
dem Hinzufügen einer Karte oder nach dem Umzug des Containers auf eine andere
Maschine.

**Aus (nur Software)** hält jede Transkodierung auf dem Prozessor, der Notausgang
bei einem fehlerhaften Treiber. Ein von Hand gewähltes **Backend** (NVENC,
VAAPI, QSV, RKMPP) erzwingt dieses; erweist es sich als unbrauchbar, fällt der
Server auf den Prozessor zurück und die Karte zeigt das tatsächlich genutzte
Backend.

Auf einer Maschine mit mehreren Beschleunigern hat Auto keine feste Vorliebe:
Jeder wird zertifiziert und gemessen, und jeder Stream geht an das Backend, das
den benötigten Weg bewiesen hat, in der besten bewiesenen Qualität und zu den
niedrigsten gemessenen Kosten. Zwei Streams auf demselben Server können daher
verschiedene Backends nutzen, zum Beispiel HDR-Tone-Mapping auf einer Karte und
eine HEVC-Kodierung auf einer anderen.

## Prüfen, dass sie genutzt wird

**Admin → Transkodierung → Encode-Kalibrierung** zeigt die erkannten
Beschleuniger, das genutzte Backend, die für GPU und Prozessor zertifizierten
Auflösungen und jeden Rückfall auf den Prozessor. Während der Wiedergabe nennt
**Admin → Aktuelle Wiedergabe** das Backend, mit dem jeder Stream kodiert wird.

Unter WSL2 funktioniert NVENC; VAAPI und Quick Sync nicht (der WSL-Grafiktreiber
scheitert an diesen Kodierungen), und der Admin-Bereich graut sie aus.
