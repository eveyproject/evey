package library

import "errors"

// ErrDetectorKilled reports that an intro-detection decode was killed by a
// signal rather than exiting on its own. In practice that means the OOM killer:
// the decode grew until the host ran out of memory.
//
// It is deliberately NOT treated as a transient failure. The same file on the
// same build reproduces it, and the sweep's work list is derived from the
// database, so it survives a restart: handing the media back would make the
// host die again at the next boot. The sweep counts it against the attempt cap
// instead, which parks the media after the usual number of tries.
var ErrDetectorKilled = errors.New("intro detection: ffmpeg killed by signal")
