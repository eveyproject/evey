# Decode probe fixtures

Real bitstreams for the GPU decode probe matrix (`hwDecodeProbes` in
`hwdetect.go`) when ffmpeg has no encoder to synthesise the class. Every file
here is embedded in the binary and decoded at calibration exactly like the
synthesised samples: HW decode on the backend, `hwdownload` mandatory (no
silent software fallback), decoded frames counted, timeout, any error = the
class is not listed.

| file | class | status |
|---|---|---|
| `vc1-yuv420p.vc1` | `DecodeClassKey("vc1", "yuv420p")` | **missing** - ffmpeg has no VC-1 encoder (wmv3/vc1 are decode-only) and no synthetic, clearly redistributable VC-1 elementary stream is available to bundle. The public clips (SMPTE conformance suite, FATE samples, disc rips) carry no licence that makes redistribution clearly safe. Until a licence-clean fixture lands here, VC-1 sources software-decode. |

Requirements for a fixture: a raw elementary stream the named `-f` demuxer
reads (`vc1` = VC-1 Advanced Profile), progressive, at least 144 rows (NVDEC
minimum), a handful of frames, no third-party content. An operator can supply
one at runtime without rebuilding: drop it under the directory named by
`EVEY_HW_DECODE_FIXTURES_DIR` with the same file name.
