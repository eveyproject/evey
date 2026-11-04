package library

import "fmt"

// ResolutionTier is the standard tier a frame belongs to (2160, 1440, 1080,
// 720), or 0 below 720p. The long side counts as much as the short one: a
// scope film cropped to 1920x818 is a 1080p file, and a label read off its
// height said "818p". The sides are taken by length, not by axis, so a
// portrait 1080x1920 is the 1080p frame it is and not a 1440p one read off
// its height. Same rule as resolutionTier in @evey/api, so the server's
// session label and every client's badge agree. With one side unknown, the
// known one keeps its axis: a lone height is a short side, a lone width a
// long one.
func ResolutionTier(width, height int) int {
	short, long := min(width, height), max(width, height)
	if width <= 0 {
		short = height
	}
	byShort := 0
	switch {
	case short >= 2160:
		byShort = 2160
	case short >= 1440:
		byShort = 1440
	case short >= 1080:
		byShort = 1080
	case short >= 720:
		byShort = 720
	}
	byLong := 0
	switch {
	case long >= 3800:
		byLong = 2160
	case long >= 2500:
		byLong = 1440
	case long >= 1880:
		byLong = 1080
	case long >= 1240:
		byLong = 720
	}
	return max(byShort, byLong)
}

// ResolutionLabel names a frame the way every surface prints it: "4K",
// "1440p", "1080p", "720p", and below that its short side ("576p", portrait
// or not). Empty when the height is unknown.
func ResolutionLabel(width, height int) string {
	if height <= 0 {
		return ""
	}
	switch tier := ResolutionTier(width, height); {
	case tier == 2160:
		return "4K"
	case tier > 0:
		return fmt.Sprintf("%dp", tier)
	}
	if width > 0 && width < height {
		return fmt.Sprintf("%dp", width)
	}
	return fmt.Sprintf("%dp", height)
}
