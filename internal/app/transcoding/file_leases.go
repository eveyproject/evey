package transcoding

import (
	"sync"
	"time"
)

// FileLeases marks the cache files a player is reading, so the cache budget
// never evicts one under it - in any pass: age, free-space floor or cap. A
// progressive /remux.mp4 is read range by range, each range reopening the
// file, and evicting it between two ranges makes the next one pay a whole new
// remux while the player stalls. A file is held while a request reads it and
// for Linger after the last one: a player's gaps between ranges are its
// buffer, minutes at most.
type FileLeases struct {
	Linger time.Duration

	mu    sync.Mutex
	files map[string]*fileLease
}

type fileLease struct {
	refs int
	last time.Time
}

// DefaultFileLeaseLinger is how long a file stays held after its last read.
const DefaultFileLeaseLinger = 10 * time.Minute

// NewFileLeases returns an empty registry holding files DefaultFileLeaseLinger
// past their last read.
func NewFileLeases() *FileLeases {
	return &FileLeases{Linger: DefaultFileLeaseLinger, files: map[string]*fileLease{}}
}

// Hold marks path as read from now until the returned release is called
// (and Linger past it). Nil-safe: a nil registry holds nothing.
func (l *FileLeases) Hold(path string) (release func()) {
	if l == nil {
		return func() {}
	}
	l.mu.Lock()
	f := l.files[path]
	if f == nil {
		f = &fileLease{}
		l.files[path] = f
	}
	f.refs++
	f.last = time.Now()
	l.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			f.refs--
			f.last = time.Now()
			l.mu.Unlock()
		})
	}
}

// Held reports whether path is being read or was within Linger of now, and
// forgets the leases that lapsed.
func (l *FileLeases) Held(path string, now time.Time) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for p, f := range l.files {
		if f.refs <= 0 && now.Sub(f.last) >= l.Linger {
			delete(l.files, p)
		}
	}
	f := l.files[path]
	return f != nil && (f.refs > 0 || now.Sub(f.last) < l.Linger)
}
