package playback

// PlaySessionRepository composes command+query for adapters that implement both.
type PlaySessionRepository interface {
	SessionCommandRepo
	SessionQueryRepo
}
