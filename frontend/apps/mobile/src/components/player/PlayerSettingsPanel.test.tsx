import { describe, it, expect, vi } from "vitest";
import { render, fireEvent } from "@testing-library/react";
import React from "react";
import { DEFAULT_SUBTITLE_STYLE } from "../../lib/subtitleStyle";

// subtitleStyle.ts imports expo-secure-store at module scope for its
// persistence helpers - unused by this panel (persistence lives in the player
// screen), but the real module requires the native runtime and crashes
// outside it if not stubbed here.
vi.mock("expo-secure-store", () => ({
  getItemAsync: vi.fn(),
  setItemAsync: vi.fn(),
}));

vi.mock("react-native", () => ({
  View: ({ children, style, ...rest }: Record<string, unknown>) =>
    React.createElement("div", { ...rest, "data-style": JSON.stringify(style) }, children as React.ReactNode),
  Text: ({ children, style, ...rest }: Record<string, unknown>) =>
    React.createElement("span", { ...rest, "data-style": JSON.stringify(style) }, children as React.ReactNode),
  Pressable: ({ children, onPress, accessibilityLabel, style, disabled, ...rest }: Record<string, unknown>) =>
    React.createElement(
      "button",
      {
        ...rest,
        onClick: disabled ? undefined : (onPress as () => void),
        disabled: disabled as boolean,
        "aria-label": accessibilityLabel as string,
        "data-style": JSON.stringify(style),
      },
      children as React.ReactNode,
    ),
  ScrollView: ({ children, style }: Record<string, unknown>) =>
    React.createElement("div", { "data-style": JSON.stringify(style) }, children as React.ReactNode),
  StyleSheet: { create: (s: Record<string, unknown>) => s, hairlineWidth: 1 },
  useWindowDimensions: () => ({ width: 400, height: 800 }),
}));

vi.mock("@expo/vector-icons", () => ({
  Ionicons: ({ name }: { name: string }) => React.createElement("span", { "data-icon": name }),
}));

vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      surface: "#111",
      border: "#222",
      surfaceRaised: "#333",
      accent: "#818cf8",
      onAccent: "#000",
      text: "#fff",
      textMuted: "#888",
    },
  }),
}));

// Real key -> English string lookup (from i18n/en/player.yaml): most calls in
// PlayerSettingsPanel don't pass a fallback, so a fallback-only mock would
// just echo the raw key back and every accessibilityLabel assertion below
// would be testing nothing.
const TRANSLATIONS: Record<string, string> = {
  "player.quality.auto": "Auto",
  "player.settings.subtitles": "Subtitles",
  "player.settings.off": "Off",
  "player.settings.uploadSubtitle": "Upload subtitle…",
  "player.settings.subtitleStyle": "Subtitle Style",
  "player.settings.subtitleSync": "Subtitle sync",
  "player.settings.subtitleSyncHint": "Positive = subtitles later. Applied instantly.",
  "player.subtitleStyle.size": "Size",
  "player.subtitleStyle.textColor": "Text Color",
  "player.subtitleStyle.background": "Background",
  "player.subtitleStyle.position": "Position",
  "player.subtitleStyle.top": "Top",
  "player.subtitleStyle.bottom": "Bottom",
  "player.subtitleStyle.textShadow": "Text shadow",
  "player.colors.white": "White",
  "player.colors.yellow": "Yellow",
  "player.colors.green": "Green",
  "player.colors.cyan": "Cyan",
  "player.colors.orange": "Orange",
  "player.bgColors.black": "Black",
  "player.bgColors.darkGrey": "Dark grey",
  "player.bgColors.navy": "Navy",
  "player.bgColors.none": "None",
  "common.back": "Back",
  "player.settings.audio": "Audio",
  "media.burnIn": "burn-in",
};

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => TRANSLATIONS[key] ?? fallback ?? key, i18n: { language: "en" } }),
}));

import { PlayerSettingsPanel, QUALITY_HEIGHTS } from "./PlayerSettingsPanel";

const baseProps = {
  audioStreams: [],
  currentAudioIndex: 0,
  onAudioChange: vi.fn(),
  subtitleStreams: [],
  currentSubtitleIndex: null,
  onSubtitleChange: vi.fn(),
  currentQuality: 0,
  onQualityChange: vi.fn(),
  playbackSpeed: 1,
  onSpeedChange: vi.fn(),
  brightness: 1,
  onBrightnessChange: vi.fn(),
  onClose: vi.fn(),
};

describe("PlayerSettingsPanel - qualite", () => {
  // The defect: the menu offered "Auto" AND "Original", both sending 0. Two
  // names, one behaviour - the viewer picks between them expecting a
  // difference that does not exist. Asserted on the VALUES, because a
  // label-based check misses the duplicate the moment it comes back under a
  // name the test does not know.
  it("ne propose jamais deux hauteurs identiques", () => {
    expect(new Set(QUALITY_HEIGHTS).size).toBe(QUALITY_HEIGHTS.length);
  });

  it("offre bien la ligne qui laisse le lecteur choisir", () => {
    expect(QUALITY_HEIGHTS).toContain(0);
    const { queryByLabelText } = render(<PlayerSettingsPanel {...baseProps} />);
    expect(queryByLabelText("Auto")).toBeTruthy();
    // "Original" described a behaviour the value never had.
    expect(queryByLabelText("Original")).toBeNull();
  });

  it("chaque ligne rapporte sa hauteur a l'appelant", () => {
    const onQualityChange = vi.fn();
    const { getByLabelText } = render(<PlayerSettingsPanel {...baseProps} onQualityChange={onQualityChange} />);
    fireEvent.click(getByLabelText("720p"));
    expect(onQualityChange).toHaveBeenCalledWith(720);
  });

  // The fixed ladder offered 1080p on a 720p source and then served whatever
  // the server had: a choice that does nothing is worse than no choice. The
  // manifest is the only thing that knows what exists.
  it("n'offre que les barreaux que le manifeste declare", () => {
    const { queryByLabelText } = render(
      <PlayerSettingsPanel {...baseProps} availableQualities={[798, 532]} />,
    );
    // Labelled by tier - 798p reads as a bug - and 1080p is simply not offered.
    expect(queryByLabelText("720p")).toBeTruthy();
    expect(queryByLabelText("480p")).toBeTruthy();
    expect(queryByLabelText("1080p")).toBeNull();
    expect(queryByLabelText("360p")).toBeNull();
    expect(queryByLabelText("Auto")).toBeTruthy();
  });

  // The value sent back is the TIER: it goes to the server as maxHeight, and
  // the server caps on each rung's box. Since rungs declare their encoded size,
  // a scope 1080p rung reads 1920x800, and sending 800 dropped that very rung.
  it("renvoie le palier, celui que le serveur plafonne", () => {
    const onQualityChange = vi.fn();
    const { getByLabelText } = render(
      <PlayerSettingsPanel {...baseProps} availableQualities={[798, 532]} onQualityChange={onQualityChange} />,
    );
    fireEvent.click(getByLabelText("720p"));
    expect(onQualityChange).toHaveBeenCalledWith(720);
  });

  it("un film scope demande 1080, pas les 800 lignes encodees", () => {
    const onQualityChange = vi.fn();
    const { getByLabelText } = render(
      <PlayerSettingsPanel
        {...baseProps}
        availableQualities={[{ width: 1920, height: 800 }, { width: 1280, height: 534 }]}
        onQualityChange={onQualityChange}
      />,
    );
    fireEvent.click(getByLabelText("1080p"));
    expect(onQualityChange).toHaveBeenCalledWith(1080);
  });

  // Direct play has no manifest, and a manifest not read yet is not an answer:
  // in both cases the menu keeps what it always showed.
  it("garde l'echelle statique quand le manifeste ne dit rien", () => {
    const { queryByLabelText } = render(<PlayerSettingsPanel {...baseProps} availableQualities={[]} />);
    expect(queryByLabelText("1080p")).toBeTruthy();
    expect(queryByLabelText("360p")).toBeTruthy();
  });
});

describe("PlayerSettingsPanel - external subtitle upload", () => {
  it("hides the Subtitles entry entirely when there are no server tracks and no upload handler", () => {
    const { queryByLabelText } = render(<PlayerSettingsPanel {...baseProps} />);
    expect(queryByLabelText("Subtitles")).toBeNull();
  });

  it("shows the Subtitles entry (with only Off + Upload) when there are no server tracks but an upload handler is provided", () => {
    const { getByLabelText, getByText } = render(
      <PlayerSettingsPanel {...baseProps} onPickExternalSubtitle={vi.fn()} />,
    );
    fireEvent.click(getByLabelText("Subtitles"));
    expect(getByText("Off")).toBeTruthy();
    expect(getByText("Upload subtitle…")).toBeTruthy();
  });

  it("hides the Upload entry when onPickExternalSubtitle is not provided", () => {
    const { getByLabelText, queryByText } = render(
      <PlayerSettingsPanel {...baseProps} subtitleStreams={[{ index: 1, trackIndex: 0, language: "en" }]} />,
    );
    fireEvent.click(getByLabelText("Subtitles"));
    expect(queryByText("Upload subtitle…")).toBeNull();
  });

  it("pressing Upload calls onPickExternalSubtitle and does not close the panel", () => {
    const onPickExternalSubtitle = vi.fn();
    const onClose = vi.fn();
    const { getByLabelText } = render(
      <PlayerSettingsPanel {...baseProps} onClose={onClose} onPickExternalSubtitle={onPickExternalSubtitle} />,
    );
    fireEvent.click(getByLabelText("Subtitles"));
    fireEvent.click(getByLabelText("Upload subtitle…"));
    expect(onPickExternalSubtitle).toHaveBeenCalledTimes(1);
    expect(onClose).not.toHaveBeenCalled();
  });

  it("lists the loaded external file as a selectable, checkable row", () => {
    const onSelectExternalSubtitle = vi.fn();
    const onClose = vi.fn();
    const { getByLabelText, getByText } = render(
      <PlayerSettingsPanel
        {...baseProps}
        onClose={onClose}
        externalSubtitleName="movie.srt"
        externalSubtitleActive={true}
        onPickExternalSubtitle={vi.fn()}
        onSelectExternalSubtitle={onSelectExternalSubtitle}
      />,
    );
    fireEvent.click(getByLabelText("Subtitles"));
    expect(getByText("movie.srt")).toBeTruthy();
    fireEvent.click(getByLabelText("movie.srt"));
    expect(onSelectExternalSubtitle).toHaveBeenCalledTimes(1);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("shows the loaded file name as the current value when it is active and no server track is selected", () => {
    const { getByLabelText } = render(
      <PlayerSettingsPanel
        {...baseProps}
        externalSubtitleName="movie.srt"
        externalSubtitleActive={true}
        onPickExternalSubtitle={vi.fn()}
      />,
    );
    expect(getByLabelText("Subtitles").textContent).toContain("movie.srt");
  });
});

describe("PlayerSettingsPanel - subtitle style", () => {
  it("hides the Subtitle Style entry when onSubtitleStyleChange is not provided", () => {
    const { queryByLabelText } = render(<PlayerSettingsPanel {...baseProps} />);
    expect(queryByLabelText("Subtitle Style")).toBeNull();
  });

  it("navigates into the subtitle-style submenu", () => {
    const { getByLabelText, getByText } = render(
      <PlayerSettingsPanel {...baseProps} subtitleStyle={DEFAULT_SUBTITLE_STYLE} onSubtitleStyleChange={vi.fn()} />,
    );
    fireEvent.click(getByLabelText("Subtitle Style"));
    expect(getByText("Size")).toBeTruthy();
    expect(getByText("Text Color")).toBeTruthy();
    expect(getByText("Background")).toBeTruthy();
    expect(getByText("Position")).toBeTruthy();
  });

  it("picking a text color swatch reports the patch", () => {
    const onSubtitleStyleChange = vi.fn();
    const { getByLabelText } = render(
      <PlayerSettingsPanel {...baseProps} subtitleStyle={DEFAULT_SUBTITLE_STYLE} onSubtitleStyleChange={onSubtitleStyleChange} />,
    );
    fireEvent.click(getByLabelText("Subtitle Style"));
    fireEvent.click(getByLabelText("Yellow"));
    expect(onSubtitleStyleChange).toHaveBeenCalledWith({ color: "#fde047" });
  });

  it("picking the 'none' background swatch also zeroes the opacity", () => {
    const onSubtitleStyleChange = vi.fn();
    const { getByLabelText } = render(
      <PlayerSettingsPanel {...baseProps} subtitleStyle={DEFAULT_SUBTITLE_STYLE} onSubtitleStyleChange={onSubtitleStyleChange} />,
    );
    fireEvent.click(getByLabelText("Subtitle Style"));
    fireEvent.click(getByLabelText("None"));
    expect(onSubtitleStyleChange).toHaveBeenCalledWith({ bgColor: "#000000", bgOpacity: 0 });
  });

  it("toggling position reports the patch", () => {
    const onSubtitleStyleChange = vi.fn();
    const { getByLabelText } = render(
      <PlayerSettingsPanel {...baseProps} subtitleStyle={DEFAULT_SUBTITLE_STYLE} onSubtitleStyleChange={onSubtitleStyleChange} />,
    );
    fireEvent.click(getByLabelText("Subtitle Style"));
    fireEvent.click(getByLabelText("Top"));
    expect(onSubtitleStyleChange).toHaveBeenCalledWith({ position: "top" });
  });

  it("toggling the shadow checkbox flips the current value", () => {
    const onSubtitleStyleChange = vi.fn();
    const { getByLabelText } = render(
      <PlayerSettingsPanel {...baseProps} subtitleStyle={DEFAULT_SUBTITLE_STYLE} onSubtitleStyleChange={onSubtitleStyleChange} />,
    );
    fireEvent.click(getByLabelText("Subtitle Style"));
    fireEvent.click(getByLabelText("Text shadow"));
    expect(onSubtitleStyleChange).toHaveBeenCalledWith({ shadow: true });
  });

  it("does not close the panel when a style option is picked (live preview)", () => {
    const onClose = vi.fn();
    const { getByLabelText } = render(
      <PlayerSettingsPanel
        {...baseProps}
        onClose={onClose}
        subtitleStyle={DEFAULT_SUBTITLE_STYLE}
        onSubtitleStyleChange={vi.fn()}
      />,
    );
    fireEvent.click(getByLabelText("Subtitle Style"));
    fireEvent.click(getByLabelText("Yellow"));
    expect(onClose).not.toHaveBeenCalled();
  });

  it("subtitle offset stepper steps by 100ms and resets to zero", () => {
    const onSubtitleOffsetChange = vi.fn();
    const { getByLabelText } = render(
      <PlayerSettingsPanel
        {...baseProps}
        subtitleStyle={DEFAULT_SUBTITLE_STYLE}
        onSubtitleStyleChange={vi.fn()}
        subtitleOffsetMs={200}
        onSubtitleOffsetChange={onSubtitleOffsetChange}
      />,
    );
    fireEvent.click(getByLabelText("Subtitle Style"));
    fireEvent.click(getByLabelText("Subtitle sync +"));
    expect(onSubtitleOffsetChange).toHaveBeenCalledWith(300);
    fireEvent.click(getByLabelText("Subtitle sync -"));
    expect(onSubtitleOffsetChange).toHaveBeenCalledWith(100);
    fireEvent.click(getByLabelText("Subtitle sync 0"));
    expect(onSubtitleOffsetChange).toHaveBeenCalledWith(0);
  });
});

// Track selection was the one thing the player layout suite stubbed out
// (PlayerSettingsPanel rendered as null): the audio and subtitle menus, what
// they list, what they report and that the panel closes on a pick.
describe("PlayerSettingsPanel - track selection", () => {
  const audioStreams = [
    { index: 1, trackIndex: 0, language: "fre", title: "VF", channels: 6 },
    { index: 2, trackIndex: 1, language: "eng", channels: 2 },
  ];
  const subtitleStreams = [
    { index: 3, trackIndex: 0, language: "fre", title: "Francais" },
    { index: 4, trackIndex: 1, language: "eng", codec: "hdmv_pgs_subtitle", burnIn: true },
  ];

  it("hides the Audio entry without tracks and shows the current track's label with it", () => {
    expect(render(<PlayerSettingsPanel {...baseProps} />).queryByLabelText("Audio")).toBeNull();
    const { getByLabelText } = render(
      <PlayerSettingsPanel {...baseProps} audioStreams={audioStreams} currentAudioIndex={1} />,
    );
    expect(getByLabelText("Audio").textContent).toContain("VF");
  });

  it("lists every audio track (title or language, with its layout) and reports the picked index once, then closes", () => {
    const onAudioChange = vi.fn();
    const onClose = vi.fn();
    const { getByLabelText, getByText } = render(
      <PlayerSettingsPanel {...baseProps} audioStreams={audioStreams} currentAudioIndex={1} onAudioChange={onAudioChange} onClose={onClose} />,
    );
    fireEvent.click(getByLabelText("Audio"));
    expect(getByText("VF - 5.1")).toBeTruthy();
    // A track with no title reads as its language's name, not its ISO code.
    fireEvent.click(getByLabelText("English - 2.0"));
    expect(onAudioChange).toHaveBeenCalledTimes(1);
    expect(onAudioChange).toHaveBeenCalledWith(2);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("lists Off plus every subtitle track, flags burn-in, and reports null for Off", () => {
    const onSubtitleChange = vi.fn();
    const onClose = vi.fn();
    const { getByLabelText, getByText } = render(
      <PlayerSettingsPanel {...baseProps} subtitleStreams={subtitleStreams} currentSubtitleIndex={3} onSubtitleChange={onSubtitleChange} onClose={onClose} />,
    );
    expect(getByLabelText("Subtitles").textContent).toContain("Francais");
    fireEvent.click(getByLabelText("Subtitles"));
    expect(getByText("burn-in")).toBeTruthy();
    fireEvent.click(getByLabelText("English"));
    expect(onSubtitleChange).toHaveBeenCalledWith(4);
    fireEvent.click(getByLabelText("Off"));
    expect(onSubtitleChange).toHaveBeenLastCalledWith(null);
    expect(onClose).toHaveBeenCalledTimes(2);
  });
});

