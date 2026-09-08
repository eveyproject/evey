---
source_sha: aa2eb3901d0f475fc80f6f02196efd56ffd3beb911d1b5b63e4d270fd3d28c20
---
# Transkodierung

Beherrscht ein Client das Ausgangsformat nicht, transkodiert Evey das Medium im
laufenden Betrieb.

## Wie das funktioniert

Evey wählt den besten Weg zur Wiedergabe von selbst:
1. **Direktwiedergabe** - das Format wird unterstützt, es fällt keine Arbeit an
2. **Remux** - der Container wird neu verpackt (MKV → MP4), ohne neu zu kodieren
3. **Transkodierung** - neu kodieren nach HLS mit anpassbarer Bitrate

## Konfiguration

```env
EVEY_TRANSCODING_MAX_CONCURRENT=2
# auto | off | nvenc | vaapi | qsv - siehe Hardware-Beschleunigung weiter unten
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

Gesteuert wird die Hardware-Beschleunigung unter **Admin → Serverkonfiguration →
Encode-Kalibrierung**:

- Ein Hauptschalter **Hardwarebeschleunigung**. Aus = nur Software (der Notausgang
  für eine unzuverlässige GPU oder einen wackeligen Treiber), an = die GPU wird
  benutzt.
- Ist er an, kommt eine Auswahl des **Backends** hinzu: `Auto` (die Kalibrierung
  nimmt das beste erkannte) oder fest `NVENC`, `VAAPI` beziehungsweise `QSV`.
  Erweist sich ein fest gewähltes Backend als unbrauchbar, fällt Evey auf
  Software zurück, und die Karte zeigt an, welches Backend tatsächlich läuft.

Die Wahl bleibt erhalten und wird im laufenden Betrieb neu vermessen (kein
Neustart nötig). Für Installationen ohne Oberfläche lässt sie sich auch aus der
Umgebung vorbelegen:

```env
# Entweder der alte An-/Aus-Schalter…
EVEY_TRANSCODING_HW_ACCEL_ENABLED=true
# …oder der vereinheitlichte Modus (auto|off|nvenc|vaapi|qsv); der Wert aus dem Administrationsbereich gewinnt.
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
# Standardmäßig AUS (=0, die Leiter mit mehreren Stufen) - und genau das IST die
# richtige Einstellung für ein schwaches NAS: Mit mehreren Stufen teilen sich
# alle Stufen EINEN Dekodiervorgang der Quelle. Mit einer einzelnen Stufe (=1)
# wird die Quelle je Stufe erneut dekodiert; sobald der Player die Bitrate
# abtastet oder über Stufen hinweg springt, laufen mehrere Dekodiervorgänge
# (etwa in 4K) gleichzeitig und nehmen sich gegenseitig die Luft - das ist das
# bekannte Hin und Her aus 360p und Nachladen auf einem Rechner, dem das
# Dekodieren die Puste nimmt. Setzen Sie =1 nur auf einer Maschine, die sich
# einen Dekodiervorgang je Stufe leisten kann. Zum Cache passt beides.
EVEY_TRANSCODING_SINGLE_VARIANT=0
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

Beim Start (und bei
`POST /api/v1/admin/transcode/calibration/recalibrate`) misst der Benchmark an
einem 4K-Clip, wie viel diese Maschine in Echtzeit kodieren kann, und zwar
getrennt für SDR und HDR (`software_max_height` /
`software_hdr_max_height`). Abgeleitet wird die Obergrenze aus dem Aufwand für
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

Öffnen Sie **Admin → Serverkonfiguration → Encode-Kalibrierung**: Dort stehen die
erkannten Beschleuniger, das aktive Backend, die gemessenen Obergrenzen für
Software und Hardware sowie jeder automatische Rückfall von Hardware auf
Software. Die Schaltfläche „Neu kalibrieren“ lässt den Benchmark nach einem
Wechsel von Hardware oder Treiber erneut laufen.
