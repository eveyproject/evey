// Offer a lighter copy when the original is too much for this phone.
//
// Two things can make a download unreasonable, and only one of them is
// knowable server-side: the file is simply enormous (the operator's
// downloads.reduce_above_bytes threshold), or it will not FIT on this
// particular device. The server answers the first in its /plan response; the
// second is ours alone. Either is enough to ask the question.
//
// Same shape as the mobile-data gate next door: one promise, one Alert, no
// state left behind. A user who is never offered anything sees no dialog at
// all - the plan lookup is a single cheap request that starts no work.

import { Alert } from "react-native";
import { formatBytes } from "@evey/api";
import { fetchReducedPlan, type ReducedPlan } from "./reducedDownload";

type T = (key: string, defaultValue: string, options?: Record<string, unknown>) => string;

export type DownloadQuality = "original" | "reduced" | "cancel";

export interface QualityChoice {
  quality: DownloadQuality;
  /** Present when the user chose "reduced" - what to hand startReducedDownload. */
  plan?: ReducedPlan;
}

/**
 * Free bytes on the device, or undefined when unknowable. Imported lazily on
 * purpose: expo-file-system reads globalThis.expo at import time, which only
 * exists in a real RN runtime, and pulling it in at module scope drags it into
 * every screen that merely renders a download button (it broke two screen test
 * suites the moment this gate joined the media page's import graph).
 */
async function availableDiskSpace(): Promise<number | undefined> {
  try {
    const { Paths } = await import("expo-file-system");
    return Paths?.availableDiskSpace;
  } catch {
    return undefined;
  }
}

/** Bytes a download of this size really needs, matching downloads.ts's own
 * headroom rule: a device with literally zero free bytes misbehaves well
 * beyond this app. */
function requiredFor(bytes: number): number {
  return bytes + 512 * 1024 * 1024;
}

/**
 * Decide what to download. Returns "original" with no dialog whenever there is
 * nothing worth asking about: the server has the feature off (or is too old),
 * the item is already small, or the original fits comfortably and is under the
 * operator's threshold.
 */
export async function chooseDownloadQuality(t: T, mediaItemId: string): Promise<QualityChoice> {
  const plan = await fetchReducedPlan(mediaItemId);
  if (!plan) return { quality: "original" };

  const free = await availableDiskSpace();
  // A missing free-space reading must not invent a problem: fall back to the
  // server's threshold alone rather than guessing the device is full.
  const fits = typeof free !== "number" || free <= 0 || requiredFor(plan.sourceBytes) <= free;
  if (fits && !plan.offer) return { quality: "original" };

  return new Promise<QualityChoice>((resolve) => {
    const original = formatBytes(plan.sourceBytes);
    const reduced = formatBytes(plan.estimatedBytes);
    const originalButton = {
      text: t("downloads.quality.original", "Original ({{size}})", { size: original }),
      onPress: () => resolve({ quality: "original" as const }),
    };
    const reducedButton = {
      text: t("downloads.quality.reduced", "{{height}}p ({{size}})", { height: plan.height, size: reduced }),
      onPress: () => resolve({ quality: "reduced" as const, plan }),
    };
    Alert.alert(
      t("downloads.quality.title", "Choose a quality"),
      fits
        ? t("downloads.quality.bodyLarge", "This film is {{original}}. A {{height}}p copy would take about {{reduced}}.", {
            original, reduced, height: plan.height,
          })
        : t("downloads.quality.bodyNoRoom", "This film is {{original}} and will not fit on this device. A {{height}}p copy would take about {{reduced}}.", {
            original, reduced, height: plan.height,
          }),
      [
        { text: t("common.cancel", "Cancel"), style: "cancel", onPress: () => resolve({ quality: "cancel" }) },
        // Order matters: Android puts the LAST button in the emphasised slot,
        // so the reasonable choice goes last. Offering a download that cannot
        // physically land as the emphasised option would be a trap.
        ...(fits
          ? [reducedButton, originalButton]
          : [originalButton, reducedButton]),
      ],
      { cancelable: true, onDismiss: () => resolve({ quality: "cancel" }) },
    );
  });
}
