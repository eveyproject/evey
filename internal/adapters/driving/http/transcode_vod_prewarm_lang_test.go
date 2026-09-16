package httpapi

import (
	"context"
	"errors"
	"testing"

	domainuser "github.com/evey-project/evey/internal/domain/user"
)

func claimsCtx(profileID string) context.Context {
	return context.WithValue(context.Background(), httpClaimsCtxKey{}, &domainuser.Claims{UserID: "u1", ProfileID: profileID})
}

// The first subtitle track to extract is the caller's own subtitle language,
// else their audio language (whose forced track a file usually carries);
// "" means file order. Every failure resolves to file order, never a guess.
func TestSubtitlePrewarmLang(t *testing.T) {
	cases := []struct {
		name  string
		ctx   context.Context
		prefs ProfilePrefsGetter
		want  string
	}{
		{"no claims", context.Background(), &fakeProfilePrefsGetter{prefs: map[string]string{"subtitle_languages": "fr"}}, ""},
		{"no profile on the claims", claimsCtx(""), &fakeProfilePrefsGetter{prefs: map[string]string{"subtitle_languages": "fr"}}, ""},
		{"no prefs port wired", claimsCtx("p1"), nil, ""},
		{"prefs lookup fails", claimsCtx("p1"), &fakeProfilePrefsGetter{err: errors.New("db down")}, ""},
		{"nil prefs", claimsCtx("p1"), &fakeProfilePrefsGetter{}, ""},
		{"first subtitle language wins", claimsCtx("p1"), &fakeProfilePrefsGetter{prefs: map[string]string{"subtitle_languages": " fr , en", "audio_languages": "de"}}, "fr"},
		{"falls back to the audio language", claimsCtx("p1"), &fakeProfilePrefsGetter{prefs: map[string]string{"subtitle_languages": " ", "audio_languages": "de,en"}}, "de"},
		{"nothing set = file order", claimsCtx("p1"), &fakeProfilePrefsGetter{prefs: map[string]string{}}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &hlsVODServe{profilePrefs: tc.prefs}
			if got := h.subtitlePrewarmLang(tc.ctx); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// The nil adapters must yield a TRUE nil interface/func: a typed nil would
// slip past the handlers' `!= nil` guards and panic on first use.
func TestNilAdaptersAreTrueNil(t *testing.T) {
	if hwModeControllerOf(nil) != nil {
		t.Error("hwModeControllerOf(nil) must be a nil interface")
	}
	if admissionBindOf(nil) != nil {
		t.Error("admissionBindOf(nil) must be a nil func")
	}
}
