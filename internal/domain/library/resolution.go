package library

import "fmt"

// ResolutionTier is the standard tier a frame belongs to (2160, 1440, 1080,
// 720), or 0 below 720p. The width counts as much as the height: a scope film
// cropped to 1920x818 is a 1080p file, and a label read off its height said
// "818p". Same rule as resolutionTier in @evey/client, so the server's session
// label and every client's badge agree.
func ResolutionTier(width, height int) int {
	byHeight := 0
	switch {
	case height >= 2160:
		byHeight = 2160
	case height >= 1440:
		byHeight = 1440
	case height >= 1080:
		byHeight = 1080
	case height >= 720:
		byHeight = 720
	}
	byWidth := 0
	switch {
	case width >= 3800:
		byWidth = 2160
	case width >= 2500:
		byWidth = 1440
	case width >= 1880:
		byWidth = 1080
	case width >= 1240:
		byWidth = 720
	}
	return max(byHeight, byWidth)
}

// ResolutionLabel names a frame the way every surface prints it: "4K",
// "1440p", "1080p", "720p", and below that its height ("576p"). Empty when
// the height is unknown.
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
	return fmt.Sprintf("%dp", height)
}
