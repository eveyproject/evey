package ffmpeg

import "testing"

func TestParseFramecrcFirstPTS(t *testing.T) {
	out := []byte("#format: stream, dts, pts, duration, size, hash\n#tb 0: 1/1000\n#media_type 0: video\n#codec_id 0: h264\n0,    1350433,    1350516,       42,   123456, 0xabcd1234\n")
	sec, err := parseFramecrcFirstPTS(out)
	if err != nil {
		t.Fatal(err)
	}
	if sec < 1350.515 || sec > 1350.517 {
		t.Fatalf("pts = %v, want 1350.516", sec)
	}
	// 90kHz timebase (mp4-style)
	out = []byte("#tb 0: 1/90000\n0, 100, 180000, 3000, 1, 0x0\n")
	sec, err = parseFramecrcFirstPTS(out)
	if err != nil || sec != 2.0 {
		t.Fatalf("90k pts = %v err=%v, want 2.0", sec, err)
	}
	if _, err = parseFramecrcFirstPTS([]byte("#tb 0: 1/1000\n")); err == nil {
		t.Fatal("no packet line must error")
	}
}
