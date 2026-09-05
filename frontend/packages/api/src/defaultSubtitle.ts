import { normalizeLangCode } from "./langCodes";


/**
 * Which subtitle track to switch on when the viewer has expressed no choice.
 *
 * The player used to answer this with the profile's subtitle_languages
 * preference alone, and "no match" meant no subtitle at all. Not one profile on
 * a real server had that preference set, so the answer was ALWAYS none - and a
 * film whose first track is a forced French one, flagged default by the file
 * itself, opened with its foreign-language lines untranslated (prod,
 * 2026-09-05).
 *
 * The missing rule is the file's own: a container names the track it wants
 * played, exactly as it names the default audio track - and the audio side of
 * this player has always honoured that. Whoever mastered the release knew which
 * subtitle belongs with which audio; that is a better answer than anything
 * inferred here.
 *
 * On a retail rip the default subtitle is typically the FORCED one for the
 * original audio: the track carrying the lines spoken in another language (the
 * Na'vi in Avatar, a foreign radio in a thriller). Leaving it off does not make
 * the film subtitle-free, it makes parts of it unintelligible.
 */

export interface SubtitleCandidate {
  index: number;
  language?: string;
  isForced?: boolean;
  isDefault?: boolean;
  /** True for PGS/VobSub: the server has to burn it into the video. */
  burnIn?: boolean;
}

/**
 * pickDefaultSubtitle returns the absolute stream index to select, or null for
 * "no subtitle".
 *
 * Order:
 *   1. an explicit subtitle-language preference - a viewer who asked for one
 *      always wins over the file;
 *   2. the track the FILE marks as default;
 *   3. nothing. No guessing beyond that: a track picked on a hunch is one the
 *      viewer has to notice and turn off.
 */
export function pickDefaultSubtitle(
  subtitles: SubtitleCandidate[],
  preferredSubLangs: string[],
): number | null {
  // 1. Explicit preference.
  for (const lang of preferredSubLangs) {
    const want = normalizeLangCode(lang);
    if (!want) continue;
    const match = subtitles.find((s) => normalizeLangCode(s.language) === want);
    if (match) return match.index;
  }

  // 2. The file's own default. Taken as written, image tracks included: the
  //    container is the authority here, and second-guessing it is how the
  //    previous rule ended up choosing nothing at all. A default bitmap track
  //    does cost a server-side burn-in - see the player's restartForSubtitleChange.
  const def = subtitles.find((s) => s.isDefault);
  return def ? def.index : null;
}
