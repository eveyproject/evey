package transcoding

import "time"

// eventuallyWait bounds the waits on a condition that must come true (a
// spawn, a reap, a pause). A healthy run meets it in milliseconds and moves
// on; the bound only has to outlast a loaded box: under -race with dozens of
// packages in parallel, one or two seconds expired before the GC tick or the
// fake process ever ran.
const eventuallyWait = 15 * time.Second
