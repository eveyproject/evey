# offlineaudio (Go core)

The container/bitstream half of the offline audio compatibility pass:
mkvgo walks the source MKV, ac3go decodes and downmixes the AC-3/E-AC-3
track to stereo PCM, and after the Kotlin side has encoded it to AAC the
same package remuxes the file with the video and every other track copied
verbatim. See the package doc in `session.go` for the call sequence.

## Rebuild the Android binding

`../android/libs/offlineaudio.jar` and
`../android/src/main/jniLibs/arm64-v8a/libgojni.so` are build artifacts of
this directory (the gomobile AAR, unpacked - AGP refuses a local .aar
dependency inside a library module). To regenerate after changing the Go
code:

```sh
go install golang.org/x/mobile/cmd/gomobile@latest golang.org/x/mobile/cmd/gobind@latest
./refresh-android.sh
```

arm64-only on purpose: the app ships `reactNativeArchitectures=arm64-v8a`.

## Tests

```sh
go test ./...
```

Desktop tests cover the whole pipeline against real AC-3 fixtures
(`testdata/`, generated tones from ac3go's own suite); the AAC encoder is
faked there, since MediaCodec only exists on device.
