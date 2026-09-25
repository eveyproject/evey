# offlineaudio (Go core)

The container/bitstream half of the offline audio compatibility pass:
mkvgo walks the source MKV, ac3go decodes and downmixes the AC-3/E-AC-3
track to stereo PCM, and after the Kotlin side has encoded it to AAC the
same package remuxes the file with the video and every other track copied
verbatim. See the package doc in `session.go` for the call sequence.

It also carries `FrameDecoder` (`framedecoder.go`): ac3go as the phone and TV
players' AC-3/E-AC-3 decoder, behind the Kotlin `Ac3goAudioRenderer`.

## Android binding

`refresh-android.sh` builds the gomobile binding (`offlineaudio.jar` +
`jni/<abi>/libgojni.so`) into `../android/build/gobind`; the Gradle task
`bindGo` runs it before every Android build, so nothing generated is checked in.
Pinned: Go via `GOTOOLCHAIN` in the script, gomobile/gobind at the go.mod
version of golang.org/x/mobile (installed under `build/gobind/tools`), NDK from
the app's `ndkVersion`. Needs `go` (>= 1.21) in PATH. Links with
`-z max-page-size=16384` (Android 15 16 KiB pages) and refuses a misaligned .so.

One `.so` per ABI of the app's `reactNativeArchitectures`: arm64 alone on the
phone, all four on the TV (most boxes run a 32-bit userland).

## Tests

```sh
go test ./...
```

Desktop tests cover the whole pipeline against real AC-3 fixtures
(`testdata/`, generated tones from ac3go's own suite); the AAC encoder is
faked there, since MediaCodec only exists on device.
