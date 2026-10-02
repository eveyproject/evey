// The player's own screen brightness, kept for the whole app process: a level
// picked by swipe or slider is reapplied when the viewer comes back to a title,
// instead of being re-read from the system each time.
import * as b from "expo-brightness";

let remembered: number | null = null;

/** Test seam: module state would otherwise leak between cases. */
export function _resetPlayerBrightnessForTest(): void {
  remembered = null;
}

export async function setPlayerBrightness(value: number): Promise<void> {
  await b.setBrightnessAsync(value);
  remembered = value;
}

// Where a player opening now starts. With adaptive brightness on, Android has no
// readable level (expo-brightness derives it from screen_auto_brightness_adj,
// deprecated since Android 9 and often -1, i.e. 0%): a swipe starting there sent
// the screen to its minimum, so the middle is used instead.
export async function openingPlayerBrightness(): Promise<number> {
  if (remembered !== null) {
    await b.setBrightnessAsync(remembered);
    return remembered;
  }
  if (await b.isUsingSystemBrightnessAsync()) {
    const mode = await b.getSystemBrightnessModeAsync().catch(() => b.BrightnessMode.UNKNOWN);
    if (mode !== b.BrightnessMode.MANUAL) return 0.5;
  }
  return b.getBrightnessAsync();
}

// Leaving the player hands the screen back to the system level, the rest of the
// app no longer inherits the player's.
export async function releasePlayerBrightness(): Promise<void> {
  await b.restoreSystemBrightnessAsync();
}
