//go:build !linux

package main

// ownerOf: chown-to-mount-owner is a Linux container concern; elsewhere the
// files simply belong to whoever runs the command.
func ownerOf(string) (int, int) { return -1, -1 }
