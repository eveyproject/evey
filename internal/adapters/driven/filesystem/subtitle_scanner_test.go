package filesystem

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubtitleScanner_ScanForSubtitles(t *testing.T) {
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "Movie.2024.1080p.mkv")
	os.WriteFile(videoPath, []byte("video"), 0644)

	tests := []struct {
		name     string
		files    []string
		wantLen  int
		wantLang []string
		wantForc []bool
		wantFmt  []string
	}{
		{
			name:     "basic srt",
			files:    []string{"Movie.2024.1080p.srt"},
			wantLen:  1,
			wantLang: []string{""},
			wantForc: []bool{false},
			wantFmt:  []string{"srt"},
		},
		{
			name:     "language tag",
			files:    []string{"Movie.2024.1080p.en.srt", "Movie.2024.1080p.fr.ass"},
			wantLen:  2,
			wantLang: []string{"en", "fr"},
			wantForc: []bool{false, false},
			wantFmt:  []string{"srt", "ass"},
		},
		{
			name:     "forced flag",
			files:    []string{"Movie.2024.1080p.en.forced.srt"},
			wantLen:  1,
			wantLang: []string{"en"},
			wantForc: []bool{true},
			wantFmt:  []string{"srt"},
		},
		{
			name:     "vtt passthrough",
			files:    []string{"Movie.2024.1080p.vtt"},
			wantLen:  1,
			wantLang: []string{""},
			wantForc: []bool{false},
			wantFmt:  []string{"vtt"},
		},
		{
			name:    "unrelated file ignored",
			files:   []string{"Other.Movie.srt", "readme.txt"},
			wantLen: 0,
		},
		{
			name:     "3-letter lang code",
			files:    []string{"Movie.2024.1080p.eng.srt"},
			wantLen:  1,
			wantLang: []string{"en"},
			wantForc: []bool{false},
			wantFmt:  []string{"srt"},
		},
	}

	scanner := NewSubtitleScanner()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Clean up subtitle files from previous test
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if e.Name() != filepath.Base(videoPath) {
					os.Remove(filepath.Join(dir, e.Name()))
				}
			}

			for _, f := range tc.files {
				os.WriteFile(filepath.Join(dir, f), []byte("sub"), 0644)
			}

			subs, err := scanner.ScanForSubtitles(videoPath)
			require.NoError(t, err)
			assert.Len(t, subs, tc.wantLen)

			for i, sub := range subs {
				if i < len(tc.wantLang) {
					assert.Equal(t, tc.wantLang[i], sub.Language, "lang mismatch at %d", i)
				}
				if i < len(tc.wantForc) {
					assert.Equal(t, tc.wantForc[i], sub.IsForced, "forced mismatch at %d", i)
				}
				if i < len(tc.wantFmt) {
					assert.Equal(t, tc.wantFmt[i], sub.Format, "format mismatch at %d", i)
				}
			}
		})
	}
}
