package httpapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Prod 2026-09-16: an SRT ripped from ASS printed "{\an8}Une heure." on the
// web player where VLC draws the line at the top.
func TestSanitizeSubtitleVTT_TurnsAlignmentIntoCueSettingsAndDropsOverrides(t *testing.T) {
	in := "WEBVTT\n\n00:11:08.668 --> 00:11:10.462\n{\\an8}Une heure.\n\n" +
		"00:11:10.545 --> 00:11:11.380\n{\\an8}Il y a une agence\nd'{\\i1}épandage{\\i0}.\n\n" +
		"00:11:12.000 --> 00:11:13.000\n{\\an1}À gauche\n\n" +
		"00:11:14.000 --> 00:11:15.000\n{\\an6}Au milieu à droite\n\n" +
		"00:11:16.000 --> 00:11:17.000 line:20%\n{\\an9}Déjà placé\n\n" +
		"00:11:18.000 --> 00:11:19.000\nRien à changer\n"
	got := string(sanitizeSubtitleVTT([]byte(in)))
	assert.Equal(t, "WEBVTT\n\n00:11:08.668 --> 00:11:10.462 line:8%\nUne heure.\n\n"+
		"00:11:10.545 --> 00:11:11.380 line:8%\nIl y a une agence\nd'épandage.\n\n"+
		"00:11:12.000 --> 00:11:13.000 align:start\nÀ gauche\n\n"+
		"00:11:14.000 --> 00:11:15.000 line:50% align:end\nAu milieu à droite\n\n"+
		"00:11:16.000 --> 00:11:17.000 line:20% align:end\nDéjà placé\n\n"+
		"00:11:18.000 --> 00:11:19.000\nRien à changer\n", got)
}

func TestSanitizeSubtitleVTT_LeavesCleanBytesAlone(t *testing.T) {
	in := []byte("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHello {not an override}\n")
	assert.Equal(t, in, sanitizeSubtitleVTT(in))
}
