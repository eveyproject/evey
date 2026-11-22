package transcoding

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const freebox = "EveyTV/0.2.11 (Android TV 7.1.1; Freebox Player Mini v2)"

var baseline = []string{"aac", "mp3", "opus", "flac", "vorbis"}

// The beacon the box sent a second after its first decision of the day.
func TestDeviceDecoderMemory_CompletesAnEarlyDeclaration(t *testing.T) {
	d := NewDeviceDecoderMemory()
	d.ObservePlayerEvent("usr-1", freebox, "ac3go_check", "declared=ac3,eac3 blocked=no rtf=0.246 cold=0.269 max=0.346 ch=6 wall=1089ms")

	out, added := d.Complete("usr-1", freebox, baseline)
	assert.Equal(t, []string{"aac", "mp3", "opus", "flac", "vorbis", "ac3", "eac3"}, out)
	assert.Equal(t, []string{"ac3", "eac3"}, added)
}

func TestDeviceDecoderMemory_LeavesOtherDevicesAndUsersAlone(t *testing.T) {
	d := NewDeviceDecoderMemory()
	d.ObservePlayerEvent("usr-1", freebox, "ac3go_check", "declared=ac3,eac3 blocked=no")

	_, added := d.Complete("usr-2", freebox, baseline)
	assert.Empty(t, added)
	_, added = d.Complete("usr-1", "EveyMobile/0.1.2 (Android 14; Pixel 7)", baseline)
	assert.Empty(t, added)
}

// A declaration made after the check already says what the app decodes.
func TestDeviceDecoderMemory_KeepsADeclarationThatNamesTheCodecs(t *testing.T) {
	d := NewDeviceDecoderMemory()
	d.ObservePlayerEvent("usr-1", freebox, "ac3go_check", "declared=ac3,eac3 blocked=no")

	declared := []string{"aac", "ac3"}
	out, added := d.Complete("usr-1", freebox, declared)
	assert.Equal(t, declared, out)
	assert.Empty(t, added)
}

func TestDeviceDecoderMemory_ForgetsADecoderThatFailed(t *testing.T) {
	for _, ev := range []struct{ typ, details string }{
		{"ac3go_check", "declared=none blocked=slow"},
		{"ac3go_check", "declared=ac3,eac3 blocked=abnormal_exit"},
		{"ac3go_too_slow_error", "rtf=1.4"},
		{"ac3go_silence_error", ""},
		{"ac3go_abnormal_exit", ""},
	} {
		d := NewDeviceDecoderMemory()
		d.ObservePlayerEvent("usr-1", freebox, "ac3go_check", "declared=ac3,eac3 blocked=no")
		d.ObservePlayerEvent("usr-1", freebox, ev.typ, ev.details)
		_, added := d.Complete("usr-1", freebox, baseline)
		assert.Empty(t, added, ev.typ+" "+ev.details)
	}
}

func TestDeviceDecoderMemory_ACheckGrowsStale(t *testing.T) {
	d := NewDeviceDecoderMemory()
	now := time.Now()
	d.now = func() time.Time { return now }
	d.ObservePlayerEvent("usr-1", freebox, "ac3go_check", "declared=ac3,eac3 blocked=no")

	d.now = func() time.Time { return now.Add(deviceDecoderTTL + time.Hour) }
	_, added := d.Complete("usr-1", freebox, baseline)
	assert.Empty(t, added)
}

func TestDeviceDecoderMemory_IgnoresAnonymousBeacons(t *testing.T) {
	d := NewDeviceDecoderMemory()
	d.ObservePlayerEvent("", freebox, "ac3go_check", "declared=ac3,eac3 blocked=no")
	d.ObservePlayerEvent("usr-1", "", "ac3go_check", "declared=ac3,eac3 blocked=no")
	assert.Empty(t, d.m)
	var nilMem *DeviceDecoderMemory
	out, added := nilMem.Complete("usr-1", freebox, baseline)
	assert.Equal(t, baseline, out)
	assert.Empty(t, added)
}
