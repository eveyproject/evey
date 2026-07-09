//go:build !linux

package main

import "errors"

// armParentDeathSignal is Linux-only (PR_SET_PDEATHSIG); elsewhere the
// launcher itself is not used in production, so report unsupported.
func armParentDeathSignal() error {
	return errors.New("parent-death signal unsupported on this OS")
}
