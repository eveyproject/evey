// Pure navigation/rotation logic for the photo lightbox, shared by the tests
// below so the bounds (first photo, last photo, empty list) are verified
// independently of React state and the Modal/FlatList wiring in photos.tsx.

// prevLightboxIndex/nextLightboxIndex clamp instead of wrapping: stepping past
// either end of the list would silently jump to the opposite photo, which
// reads as a random jump rather than "there is nothing more this way" (mirrors
// the web lightbox in index.lazy.tsx, which hides the prev/next button
// entirely at the ends rather than wrapping).
export function prevLightboxIndex(index: number, total: number): number {
  if (total <= 0) return index;
  return index > 0 ? index - 1 : index;
}

export function nextLightboxIndex(index: number, total: number): number {
  if (total <= 0) return index;
  return index < total - 1 ? index + 1 : index;
}

// rotateClockwise/rotateCounterClockwise step by 90deg and wrap into [0, 360)
// so repeated taps never accumulate an ever-growing degree value.
export function rotateClockwise(degrees: number): number {
  return (degrees + 90) % 360;
}

export function rotateCounterClockwise(degrees: number): number {
  return (degrees + 270) % 360;
}
