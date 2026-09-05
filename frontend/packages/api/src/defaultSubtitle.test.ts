import { describe, expect, it } from "vitest";
import { pickDefaultSubtitle } from "./defaultSubtitle";

// The ordinary shape of a retail rip, taken from a real file (2026-09-05): the
// French FORCED track is the one the container marks as default.
const STREAMS = [
  { index: 3, language: "fre", isForced: true, isDefault: true },
  { index: 4, language: "fre", isForced: false, isDefault: false },
  { index: 5, language: "eng", isForced: true, isDefault: false },
  { index: 6, language: "eng", isForced: false, isDefault: false },
];

describe("pickDefaultSubtitle", () => {
  // The regression that started this: not one profile on a real server had
  // subtitle_languages set, so the old rule answered null every single time and
  // the file's own default never came on.
  it("falls back to the track the FILE marks as default", () => {
    expect(pickDefaultSubtitle(STREAMS, [])).toBe(3);
  });

  it("lets an explicit preference win over the file", () => {
    expect(pickDefaultSubtitle(STREAMS, ["eng"])).toBe(5);
  });

  it("stays off when the file names no default", () => {
    const noDefault = STREAMS.map((s) => ({ ...s, isDefault: false }));
    expect(
      pickDefaultSubtitle(noDefault, []),
      "no preference and no default is genuinely 'no subtitle' - guessing one is a track the viewer has to notice and turn off",
    ).toBeNull();
  });

  // The container is the authority: whoever mastered the release knew which
  // subtitle belongs with which audio.
  it("honours the default even when it is not the first track", () => {
    const later = [
      { index: 3, language: "fre", isDefault: false },
      { index: 4, language: "eng", isDefault: true },
    ];
    expect(pickDefaultSubtitle(later, [])).toBe(4);
  });

  // Taken as written. A default bitmap track costs a server-side burn-in, but
  // second-guessing the file is exactly how the previous rule ended up choosing
  // nothing at all.
  it("honours a default bitmap track too", () => {
    const image = [{ index: 3, language: "fre", isDefault: true, burnIn: true }];
    expect(pickDefaultSubtitle(image, [])).toBe(3);
  });

  // 639-2/B vs 639-2/T: "fre" and "fra" are the same language.
  it("matches a preference across ISO 639-2 spellings", () => {
    expect(pickDefaultSubtitle([{ index: 3, language: "fra" }], ["fre"])).toBe(3);
  });

  it("prefers the first matching preference in order", () => {
    expect(pickDefaultSubtitle(STREAMS, ["ita", "eng", "fre"])).toBe(5);
  });

  it("handles an empty track list", () => {
    expect(pickDefaultSubtitle([], ["fre"])).toBeNull();
    expect(pickDefaultSubtitle([], [])).toBeNull();
  });
});
