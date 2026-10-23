import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { lazyZustandStorage, registerPersistedStore } from "./storage";

// The data saver is a property of the DEVICE, not of the profile: the phone
// on a metered connection abroad saves, the television at home keeps full
// quality. So it lives in local storage, never in profile preferences.
export type DataSaverLevel = "off" | "saver" | "travel";

export const DATA_SAVER_LEVELS: readonly DataSaverLevel[] = ["off", "saver", "travel"];

export interface DataSaverLimits {
  /** Line ceiling sent as ?maxBitrate (kbps), video plus audio. */
  maxBitrateKbps: number;
  /** Picture ceiling sent as ?maxHeight: the server transcodes above it. */
  maxHeight: number;
  /** Stereo: a surround track weighs several times an AAC stereo one. */
  maxAudioChannels: number;
}

const LIMITS: Record<Exclude<DataSaverLevel, "off">, DataSaverLimits> = {
  saver: { maxBitrateKbps: 3000, maxHeight: 720, maxAudioChannels: 2 },
  travel: { maxBitrateKbps: 1000, maxHeight: 480, maxAudioChannels: 2 },
};

/** The translation key naming each level in the settings of every client. */
export const DATA_SAVER_LABEL_KEY: Record<DataSaverLevel, string> = {
  off: "auth.settings.playback.dataSaverOff",
  saver: "auth.settings.playback.dataSaverSaver",
  travel: "auth.settings.playback.dataSaverTravel",
};

// The AAC stereo track the server encodes when the decision names no audio
// bitrate (defaultAudioBitrateKbps in the transcoding decision).
const AUDIO_KBPS = 192;

/**
 * The values a level's label interpolates: its picture height and the most
 * it costs per hour of viewing, in GB with one decimal in the viewer's locale.
 */
export function dataSaverLabelValues(level: DataSaverLevel, locale?: string): { height?: number; gb?: string } {
  const limits = dataSaverLimits(level);
  if (!limits) return {};
  const gbPerHour = ((limits.maxBitrateKbps + AUDIO_KBPS) * 3600) / 8 / 1_000_000;
  return {
    height: limits.maxHeight,
    gb: new Intl.NumberFormat(locale, { minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(gbPerHour),
  };
}

/** The ceilings a level puts on a stream, or null when it puts none. */
export function dataSaverLimits(level: DataSaverLevel): DataSaverLimits | null {
  return level === "off" ? null : LIMITS[level];
}

/** The tighter of two ceilings, where 0/undefined means none. */
export function tighterCeiling(a: number | undefined, b: number | undefined): number | undefined {
  if (!a || a <= 0) return b && b > 0 ? b : undefined;
  if (!b || b <= 0) return a;
  return Math.min(a, b);
}

interface DataSaverState {
  level: DataSaverLevel;
  setLevel: (level: DataSaverLevel) => void;
}

export const useDataSaverStore = create<DataSaverState>()(
  persist(
    (set) => ({
      level: "off",
      setLevel: (level) => set({ level }),
    }),
    {
      name: "evey-data-saver",
      storage: createJSONStorage(() => lazyZustandStorage),
      // A value this build does not know (a newer level, a hand edit) reads
      // as off rather than as an undefined ceiling.
      merge: (persisted, current) => {
        const level = (persisted as Partial<DataSaverState> | undefined)?.level;
        return { ...current, level: level && DATA_SAVER_LEVELS.includes(level) ? level : "off" };
      },
    },
  ),
);

// Late-storage rehydration (React Native): see storage.ts.
registerPersistedStore(useDataSaverStore);

/** The current device's ceilings, read outside React (stream setup). */
export function currentDataSaverLimits(): DataSaverLimits | null {
  return dataSaverLimits(useDataSaverStore.getState().level);
}
