package main

import (
	"bytes"
	"context"

	"github.com/evey-project/evey/internal/pkg/procfork"
)

// demoAssembler runs the demo's one-off muxes (manifest Build entries) as a
// background ffmpeg: niced and inside the CPU budget, like any scan task.
func demoAssembler(ctx context.Context, args []string) error {
	cmd, finish, err := procfork.Command(ctx, procfork.Options{Class: procfork.Scan, Site: "demo.build"}, "ffmpeg", args...)
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	return finish(cmd.Run(), stderr.Bytes())
}
