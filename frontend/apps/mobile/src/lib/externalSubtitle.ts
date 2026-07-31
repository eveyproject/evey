// Client-side external subtitle upload for the mobile player: picks a local
// SRT/VTT/ASS/SSA file and converts it to WebVTT, mirroring the web player's
// loadSubtitleFile (PlayerSettingsPanel.tsx) - no server upload involved,
// only subtitleParse.ts's already-tested conversion helpers.

import * as DocumentPicker from "expo-document-picker";
// From "/legacy": readAsStringAsync imported from the package root throws at
// runtime in expo-file-system 55. See downloads.ts.
import * as FileSystem from "expo-file-system/legacy";
import { externalSubtitleKind, toVtt } from "./subtitleParse";

export interface ExternalSubtitleFile {
  name: string;
  vtt: string;
}

// Subtitle rip extensions report inconsistent (or missing) MIME types across
// Android/iOS document providers - an .srt often shows up as text/plain or
// application/octet-stream - so the picker accepts anything and
// externalSubtitleKind gates on the file extension afterwards, same as the
// web <input accept=".srt,.vtt,.ass,.ssa">.
const PICKER_TYPES = ["*/*"];

// pickExternalSubtitle opens the system document picker, reads the chosen
// file, and converts it to WebVTT. Returns null on cancel, an unsupported
// extension, or any read failure - never throws, so callers can await it
// without a try/catch of their own.
export async function pickExternalSubtitle(): Promise<ExternalSubtitleFile | null> {
  try {
    const result = await DocumentPicker.getDocumentAsync({
      type: PICKER_TYPES,
      copyToCacheDirectory: true,
      multiple: false,
    });
    if (result.canceled || !result.assets?.[0]) return null;
    const asset = result.assets[0];
    const kind = externalSubtitleKind(asset.name);
    if (!kind) return null;
    const text = await FileSystem.readAsStringAsync(asset.uri);
    return { name: asset.name, vtt: toVtt(text, kind) };
  } catch (err) {
    console.warn("[subtitles] external subtitle pick failed:", err);
    return null;
  }
}
