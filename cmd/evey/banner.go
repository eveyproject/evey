package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/eveyproject/evey/internal/config"
)

// ANSI truecolor codes for the boot banner. Brand red is #DC2626 (the wordmark
// V); the rest is a soft white with slate/dim/green supports.
const (
	bReset = "\x1b[0m"
	bWhite = "\x1b[38;2;236;238;244m"
	bRed   = "\x1b[38;2;220;38;38m"
	bSlate = "\x1b[38;2;136;145;163m"
	bDim   = "\x1b[38;2;120;128;145m"
	bGreen = "\x1b[38;2;108;192;139m"
)

// wordmarkRows is "EVEY" in the ANSI Shadow face, split per letter so the V (col
// 2) can be brand red while E·E·Y take the text colour - mirroring the app mark.
var wordmarkRows = [...][4]string{
	{"███████╗", "██╗   ██╗", "███████╗", "██╗   ██╗"},
	{"██╔════╝", "██║   ██║", "██╔════╝", "╚██╗ ██╔╝"},
	{"█████╗  ", "██║   ██║", "█████╗  ", " ╚████╔╝ "},
	{"██╔══╝  ", "╚██╗ ██╔╝", "██╔══╝  ", "  ╚██╔╝  "},
	{"███████╗", " ╚████╔╝ ", "███████╗", "   ██║   "},
	{"╚══════╝", "  ╚═══╝  ", "╚══════╝", "   ╚═╝   "},
}

// printBootBanner writes the Evey wordmark + a compact status block to w. Purely
// cosmetic: the structured logs that follow carry the authoritative detail.
// Skipped entirely when EVEY_BANNER=off; colour dropped when NO_COLOR is set.
func printBootBanner(w io.Writer, cfg *config.Config, build buildIdentity) {
	if os.Getenv("EVEY_BANNER") == "off" {
		return
	}
	color := os.Getenv("NO_COLOR") == ""
	paint := func(code, s string) string {
		if !color {
			return s
		}
		return code + s + bReset
	}

	var b strings.Builder
	b.WriteByte('\n')
	for _, r := range wordmarkRows {
		b.WriteString("  ")
		b.WriteString(paint(bWhite, r[0]))
		b.WriteString(paint(bRed, r[1]))
		b.WriteString(paint(bWhite, r[2]))
		b.WriteString(paint(bWhite, r[3]))
		b.WriteByte('\n')
	}
	b.WriteString("  " + paint(bDim, "self-hosted media server") + "              " + paint(bSlate, "v"+build.Version) + "\n")
	b.WriteString("  " + paint(bDim, "build "+shortCommit(build.Commit)+" · "+build.BuildDate) + "\n\n")

	httpPort := cfg.Server.Port + 1
	plugins := paint(bDim, "disabled")
	if cfg.Server.PluginsEnabled {
		plugins = paint(bGreen, "enabled")
	}
	row := func(label, val string) {
		b.WriteString("  " + paint(bRed, "▸") + " " + paint(bSlate, label) + "  " + val + "\n")
	}
	row("HTTP    ", paint(bWhite, fmt.Sprintf("http://0.0.0.0:%d", httpPort)))
	row("gRPC    ", paint(bWhite, fmt.Sprintf(":%d", cfg.Server.Port)))
	row("Plugins ", plugins)
	b.WriteByte('\n')

	fmt.Fprint(w, b.String())
}
