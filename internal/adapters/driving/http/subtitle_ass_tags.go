package httpapi

import (
	"bytes"
	"regexp"
	"strings"
)

// SRT tracks ripped from ASS keep override blocks such as {\an8} in the text;
// VLC honours them, a WebVTT player prints them. The alignment is kept as cue
// settings (an7-9 top, an4-6 middle, an1/4/7 left, an3/6/9 right), every other
// override is dropped.
var (
	assOverrideRE = regexp.MustCompile(`\{\\[^}]*\}`)
	assAlignRE    = regexp.MustCompile(`\\an([1-9])`)
)

const (
	vttAuthoredTopLine    = "line:8%"
	vttAuthoredMiddleLine = "line:50%"
)

// sanitizeSubtitleVTT strips ASS override blocks from every cue and turns an
// alignment override into WebVTT cue settings. Bytes without a single "{\"
// come back untouched.
func sanitizeSubtitleVTT(vtt []byte) []byte {
	if !bytes.Contains(vtt, []byte(`{\`)) {
		return vtt
	}
	blocks := strings.Split(string(vtt), "\n\n")
	for i, block := range blocks {
		lines := strings.Split(block, "\n")
		timing := -1
		for j, l := range lines {
			if strings.Contains(l, "-->") {
				timing = j
				break
			}
		}
		if timing < 0 {
			continue
		}
		align := 0
		for j := timing + 1; j < len(lines); j++ {
			if m := assAlignRE.FindStringSubmatch(lines[j]); m != nil && align == 0 {
				align = int(m[1][0] - '0')
			}
			lines[j] = strings.TrimSpace(assOverrideRE.ReplaceAllString(lines[j], ""))
		}
		if s := vttSettingsForASSAlign(align, lines[timing]); s != "" {
			lines[timing] = strings.TrimRight(lines[timing], " ") + " " + s
		}
		blocks[i] = strings.Join(lines, "\n")
	}
	return []byte(strings.Join(blocks, "\n\n"))
}

// vttSettingsForASSAlign maps an ASS numpad alignment to the cue settings the
// timing line lacks; "" for the default bottom-centre or no alignment.
func vttSettingsForASSAlign(align int, timing string) string {
	if align == 0 {
		return ""
	}
	var out []string
	if !strings.Contains(timing, "line:") {
		switch {
		case align >= 7:
			out = append(out, vttAuthoredTopLine)
		case align >= 4:
			out = append(out, vttAuthoredMiddleLine)
		}
	}
	if !strings.Contains(timing, "align:") {
		switch align % 3 {
		case 1:
			out = append(out, "align:start")
		case 0:
			out = append(out, "align:end")
		}
	}
	return strings.Join(out, " ")
}
