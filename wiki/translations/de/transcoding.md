---
source_sha: 0de496a664ed813f5ad423017ddfe0563349e3baaee95acf2ee582c2e970dd84
---
# Transkodierung

Beherrscht ein Client das Ausgangsformat nicht, transkodiert Evey das Medium im
laufenden Betrieb.

**Kurz gesagt.** Die meisten Installationen müssen hier nichts tun: Der Server
misst, was die Maschine umwandeln kann, und lässt entsprechend viele
Umwandlungen gleichzeitig zu. Um eine Grafikkarte zu nutzen, machen Sie sie im
Container sichtbar (siehe unten) und lassen **Hardware-Beschleunigung** in
**Admin → Transkodierung** auf **Auto**. Der Rest dieser Seite erklärt, was
hinter dieser Wahl passiert.

## Wie das funktioniert

Evey wählt den besten Weg zur Wiedergabe von selbst:
1. **Direktwiedergabe** - das Format wird unterstützt, es fällt keine Arbeit an
2. **Remux** - der Container wird neu verpackt (MKV → MP4), ohne neu zu kodieren
3. **Transkodierung** - neu kodieren nach HLS mit anpassbarer Bitrate

## Konfiguration

Beide Werte unten belegen nur den ersten Start vor: Der Administrationsbereich
(**Admin → Transkodierung**) gewinnt, sobald dort ein Wert gespeichert ist.

```env
EVEY_TRANSCODING_MAX_CONCURRENT=2
# auto | off | nvenc | vaapi | qsv | rkmpp - siehe Hardware-Beschleunigung weiter unten
EVEY_TRANSCODING_HW_MODE=off
```

## Hardware-Beschleunigung

Evey liefert **ein einziges Image** aus, dessen ffmpeg NVENC, VAAPI und QSV
allesamt einkompiliert hat. Ein GPU-Treiber steckt nicht im Image - den bringen
Sie zur Laufzeit ein, indem Sie das Gerät durchreichen, und Sonderrechte braucht
es dafür nicht. Ob dasselbe Image reine Software, NVENC oder VAAPI/QSV
verwendet, hängt einzig daran, was Sie durchreichen. Die Begründung steht in
[ADR-0015](adr/0015-hardware-acceleration.md).

### NVIDIA (NVENC)

Installieren Sie das [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html), reservieren Sie danach die GPU **und** fordern Sie die Treiber-Fähigkeit `video` an (ohne sie bringt das Toolkit zwar `nvidia-smi` und CUDA ein, aber nicht `libnvidia-encode.so`, und NVENC lässt sich dann nicht laden):

```yaml
services:
  evey:
    environment:
      NVIDIA_DRIVER_CAPABILITIES: all   # muss `video` enthalten, sonst kein NVENC
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
```

### Intel / AMD (VAAPI, QSV)

Reichen Sie den DRM-Render-Node durch; libva wählt den passenden Treiber
(iHD, i965 oder radeonsi) anhand der PCI-Kennung der GPU von selbst:

```yaml
services:
  evey:
    devices:
      - /dev/dri:/dev/dri
```

Sie dürfen beides gleichzeitig durchreichen (die NVIDIA-Reservierung und
`/dev/dri`) - das eine Image bedient dann, welches Backend Sie auch wählen.

### Einschalten

Gesteuert wird die Hardware-Beschleunigung unter **Admin → Transkodierung →
Encode-Kalibrierung**:

- Ein Hauptschalter **Hardwarebeschleunigung**. Aus = nur Software (der Notausgang
  für eine unzuverlässige GPU oder einen wackeligen Treiber), an = die GPU wird
  benutzt.
- Ist er an, kommt eine Auswahl des **Backends** hinzu: `Auto` (die Kalibrierung
  nimmt das beste erkannte) oder fest `NVENC`, `VAAPI`, `QSV` beziehungsweise `RKMPP`.
  Erweist sich ein fest gewähltes Backend als unbrauchbar, fällt Evey auf
  Software zurück, und die Karte zeigt an, welches Backend tatsächlich läuft.

**Was `Auto` tut.** Sie machen die Hardware im Container sichtbar (`/dev/dri`,
die NVIDIA-Reservierung, den Rockchip-MPP-Knoten); den Rest erledigt Evey, und
zwar nur auf Grundlage von Messungen - nie anhand eines CPU- oder GPU-Namens:

1. **Erkennen**: Jeder DRM-Render-Knoten wird auf VAAPI geprüft, ein
   Intel-Knoten zusätzlich auf QSV; jede NVIDIA-GPU über ihren CUDA-Index; der
   Rockchip-MPP-Knoten. Jeder Encoder muss eine echte Kodierung bestehen, bevor
   er zählt.
2. **Zertifizieren**: Jeder erkannte Beschleuniger durchläuft die
   Kodierungsleiter (720p → 2160p) in genau der Pipeline-Form, die die Laufzeit
   benutzt, behält die höchste Stufe, die er mit 1,2-facher Echtzeit hält, und
   besteht dann eine Prüfung auf visuelle Korrektheit. Unter Echtzeit bei 720p =
   abgelehnt, nur Software. Jedes zertifizierte Backend behält seine **eigene**
   Obergrenze, Dekodiermatrix, Funktionen und bewiesenen Routen - ein
   Beschleuniger, der nur 1080p schafft, neben einem, der 2160p schafft, bedient
   1080p-Leitern, nie die des anderen.
3. **Die Routen messen**: Für jede Route, die ein Backend bewiesen hat (SDR,
   HEVC, AV1, GPU-Tone-Mapping, CPU-Brücke, HDR-Durchreichung, entkoppelte
   Dekodierung) und für die Software-Routen wird eine warme Produktionssitzung
   auf den Referenzstufen und an der Obergrenze der Route gemessen - so hat auch
   ein nur für 720p zertifiziertes Backend oder der 720p-Software-Boden seine
   Kosten.
4. **Je Last wählen**: Der Resolver nimmt je Anfrage das günstigste
   zertifizierte Backend, das die Route bewiesen hat und den Codec unterstützt
   (HDR→SDR kann auf dem OpenCL-Tone-Mapping von VAAPI landen, während HEVC auf
   derselben Maschine ein schnelleres QSV nimmt). Die Zulassung bepreist die
   Sitzung nach der Messung dieser Route gegenüber einem CPU-Pool und einem
   Beschleuniger-Pool; ein voller Pool verlagert einen GPU-Stream auf die CPU
   oder lehnt ab.

Die Software-Leiter hat einen Boden bei **720p SDR / 480p HDR**: Eine CPU, die
keine Stufe mit Reserve schafft, bietet diese Stufen trotzdem an und lässt dann
nur eine Sitzung zugleich zu. Direktwiedergabe und copy/Remux belegen nie einen
Kodierplatz und sind von alldem nicht betroffen.

Die Wahl bleibt erhalten und wird im laufenden Betrieb neu vermessen (kein
Neustart nötig). Für Installationen ohne Oberfläche lässt sie sich auch aus der
Umgebung vorbelegen:

```env
# Entweder der alte An-/Aus-Schalter…
EVEY_TRANSCODING_HW_ACCEL_ENABLED=true
# …oder der vereinheitlichte Modus (auto|off|nvenc|vaapi|qsv|rkmpp); der Wert aus dem Administrationsbereich gewinnt.
EVEY_TRANSCODING_HW_MODE=auto
# Eine hängende GPU nach dieser Frist zügig auf Software umschalten (Standard 5s).
EVEY_TRANSCODING_HW_FIRST_SEGMENT_TIMEOUT=5s
# Auf dem Hardware-Weg je ABR-Stufe einen eigenen ffmpeg starten (ein Encoder je
# Stufe), statt eines einzigen Prozesses mit mehreren Ausgängen - das grenzt
# Fehler auf eine Stufe ein und vermeidet den Graphen mit mehreren Encodern, an
# dem empfindliche VA-Treiber abstürzen. Standardmäßig aus; Software nimmt immer
# den Graphen im einen Prozess. Siehe docs/adr/0015.
EVEY_TRANSCODING_HW_PER_VARIANT=false
# Nur die eine ABR-Stufe kodieren, die der Client gerade anfordert (auf Abruf,
# ein Encoder je Stufe), statt einen ffmpeg die ganze Leiter erzeugen zu lassen.
# Standardmäßig AN (=1). Mehrere Stufen teilen sich einen Dekodiervorgang, kodieren
# aber auch jede Stufe, die der Zuschauer nie ansieht - in Produktion gemessen:
# drei verschwendete Kodierungen ließen die vierte verhungern. Die Kalibrierung
# misst DIESELBE Form, die die Laufzeit fährt (eine Stufe je Probe bei einer
# einzelnen Stufe, die kumulierte parallele Leiter bei mehreren), für die
# Software- UND die Hardware-Leitern, damit die Obergrenzen dem entsprechen, was
# eine Sitzung tatsächlich kostet. Setzen Sie =0 nur auf einem Rechner ohne
# Hardware-Dekodierung UND mit schweren Quellen, um das geteilte Dekodieren
# zurückzubekommen. Zum Cache passt beides.
EVEY_TRANSCODING_SINGLE_VARIANT=1
```

> **Fünf Einstellungen stehen inzwischen im Administrationsbereich** (Admin →
> Transkodierung → Erweiterte Grenzwerte): `HW_PER_VARIANT`, `SINGLE_VARIANT`,
> `MAX_SESSIONS`, `SOFTWARE_MAX_HEIGHT` und `SOFTWARE_HDR_MAX_HEIGHT`. Ein dort
> gespeicherter Wert überdauert Neustarts und sticht die Umgebung, Sie müssen
> also nicht länger eine Compose-Datei bearbeiten und den Container neu
> erzeugen, um eine Obergrenze für die Wiedergabe zu ändern. Alle fünf werden
> beim Start gelesen, und die Oberfläche sagt das auch: Speichern hält Ihre Wahl
> fest, angewandt wird sie beim Neustart.

### Das Muster für die Encode-Kalibrierung

Der Benchmark misst an einem 4K-Clip, wie viel diese Maschine in Echtzeit
kodieren kann, und zwar getrennt für SDR und HDR (`software_max_height` /
`software_hdr_max_height`). **Der Start führt ihn nie aus**: Der Server startet
in Millisekunden mit dem, was er hat (der gespeicherten Kalibrierung oder beim
allerersten Start einem 720p-Software-Rückfall), und lässt den Benchmark im
ersten *ruhigen Moment* laufen - fünf aufeinanderfolgende Proben im Abstand von
4 s, ohne Wiedergabe und mit einer CPU-Last unter 20 %. So bleibt ein Benchmark
von etwa 2-5 Minuten aus dem Startpfad heraus und von einem Rechner fern, der
mit einer Migration oder einem Bibliotheksscan beschäftigt ist (ein unter Last
gemessener Benchmark wird als *vorläufig* markiert und im Leerlauf wiederholt).
Solange der Benchmark läuft, wird eine neue Wiedergabe gebeten, etwa 2 Minuten
zu warten (`calibrating`); **Admin → Transkodierung → Encode-Kalibrierung**
zeigt, ob die Kalibrierung aussteht, läuft oder fertig ist, und „Neu
kalibrieren“ (`POST /api/v1/admin/transcode/calibration/recalibrate`) startet
sie sofort. Abgeleitet wird die Obergrenze aus dem Aufwand für
Dekodieren, Tone-Mapping und Kodieren dieses Clips - **ein Clip mit echtem
Inhalt kalibriert daher weit ehrlicher als ein synthetischer**. Ein
synthetischer setzt den Aufwand zu hoch an und drückt die Obergrenze nach unten
(das bekannte Symptom: Ein 4K-HDR-Titel, den die Maschine tatsächlich in 1080p
abspielen könnte, wird auf 480p oder 360p gedeckelt).

Der Benchmark sucht sein Eingangsmaterial in dieser Reihenfolge:

1. **Datei des Betreibers** - ein echter Clip, den Sie selbst hinterlegen. Wird unverändert genommen.
2. **Festgeschriebener Download** - ein lizenzrechtlich unbedenklicher Referenzclip, einmal geladen (über SHA256 geprüft) und danach im Cache.
3. **Synthetisch** - im laufenden Betrieb erzeugt (`testsrc2` plus etwas Körnung bei rund 5 Mbit/s). Immer verfügbar, der Rückfall für Maschinen ohne Netz.

```bash
# Den Benchmark auf eine echte Datei zeigen lassen (Ihre eigene oder ein heruntergeladener Referenzclip):
EVEY_BENCH_HDR_SAMPLE=/data/bench/my-4k-hdr.hevc
EVEY_BENCH_SDR_SAMPLE=/data/bench/my-4k-sdr.hevc
# …oder auf einen festgeschriebenen Download (die URL muss https sein; mit gesetzter URL ist SHA256 Pflicht):
EVEY_BENCH_HDR_SAMPLE_URL=https://example.org/evey-bench-hdr10.hevc
EVEY_BENCH_HDR_SAMPLE_SHA256=<sha256>
EVEY_BENCH_SDR_SAMPLE_URL=https://example.org/evey-bench-sdr.hevc
EVEY_BENCH_SDR_SAMPLE_SHA256=<sha256>
```

Bauen Sie die Referenzclips aus einer freizügig lizenzierten Quelle (die Open
Movies von Blender, CC-BY - **nicht** aus Material unter CC-BY-NC-ND wie
„Sol Levante“ von Netflix), und zwar mit
`scripts/make-bench-sample.sh <source-4k>`; das Skript gibt die
SHA256-Prüfsumme aus, die Sie festschreiben.

### Vorbehalt zu WSL2

Unter WSL2 führt der Weg zur GPU über den experimentellen Treiber **d3d12** von
Mesa. **NVENC funktioniert** (echtes CUDA wird durchgereicht), **VAAPI und QSV
nicht** - der VA-Treiber von d3d12 stürzt bei Eveys Kodierung mit mehreren
Stufen ab. Evey erkennt WSL und stellt VAAPI und QSV im Administrationsbereich
ausgegraut dar. Prüfen Sie VAAPI und QSV auf einem echten Linux-Rechner nach.

### HDR-Durchreichung und HDR10+

Auf einer GPU, die für Main10-Kodierung geprüft ist (`hw_hdr10_encode`), wird
eine HDR-Quelle, die ein HDR-fähiger und HEVC dekodierender Client abspielt, als
HEVC Main10 mit erhaltenem HDR neu kodiert: Die statische HDR10-Signalisierung
(bt2020 / PQ oder HLG, Mastering-Display-SEI) wird durchgereicht.
Dynamische HDR10+-Metadaten werden **nicht** neu ausgegeben, ein HDR10+-Titel
läuft auf diesem Weg also als einfaches HDR10. Dolby Vision Profil 7 fällt
ebenso auf seine HDR10-Basisschicht zurück; Profil 5 wird per Tone-Mapping in
SDR umgewandelt.

Die Abtastrate des Tons ist nie ein Grund zum Neukodieren: Eine 96-kHz-Spur,
die die Codec-Liste des Clients akzeptiert, wird unverändert kopiert. Ein
Client, dessen Decoder bei 48 kHz endet, muss das über seine Codec-Deklaration
sagen; der Server unterstellt es nicht.

## AC-3- und E-AC-3-Ton im Browser (Decoder in WebAssembly)

Die meisten Browser können AC-3 und E-AC-3 aus Lizenzgründen nicht selbst
dekodieren, was üblicherweise eine Neukodierung des Tons auf dem Server
erzwingt. Evey schickt dem Browser stattdessen einen Decoder in WebAssembly: Die
Master-Playlist enthält nur noch Video, die AC-3-Spur wird als eigene Rendition
daneben ausgeliefert (kopiert, nicht neu kodiert), und der Client dekodiert sie
nach PCM, taktgenau zum Bild. Für den Ton verbraucht der Server damit keine
einzige CPU-Sekunde, sofern sich die Quelle unverändert verpacken lässt.

- Browser, die AC-3 sehr wohl selbst dekodieren (die Safari-Klasse), werden
  ausgenommen - erkannt an ihren Fähigkeiten, nicht am Namen. Sie bekommen die
  gewöhnliche Durchreichung.
- **Voraussetzung beim Reverse Proxy**: Die CSP, die beim Browser ankommt, muss
  `'wasm-unsafe-eval'` in `script-src` enthalten, sonst lässt sich der Decoder
  nicht kompilieren und der Film läuft stumm. Siehe
  [reverse-proxy.md](reverse-proxy.md#security-headers--csp---read-this-if-your-proxy-sets-its-own).

## Nachprüfen

Öffnen Sie **Admin → Transkodierung → Encode-Kalibrierung**: Dort stehen die
erkannten Beschleuniger, das aktive Backend, die gemessenen Obergrenzen für
Software und Hardware sowie jeder automatische Rückfall von Hardware auf
Software. Die Schaltfläche „Neu kalibrieren“ lässt den Benchmark nach einem
Wechsel von Hardware oder Treiber erneut laufen.
