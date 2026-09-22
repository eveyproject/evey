import { View, Text, Pressable } from "react-native";
import { useTranslation } from "react-i18next";
import { formatAudioTrackLabel, type TrackPick, type V1MediaStream, trackDisplayName } from "@evey/api";
import { useTheme } from "../providers/ThemeProvider";

interface Props {
  streams: V1MediaStream[];
  pick: TrackPick;
  onChange: (pick: TrackPick) => void;
}

type Choice = { key: string; label: string; selected: boolean; onPress: () => void };

// Audio and subtitle chosen BEFORE playback, as chip rows under the Play
// button. "Auto" is the player's own resolution, so an untouched page plays
// exactly what it always did. Mirrors the web MediaDetailTracks.
export function TrackPickChips({ streams, pick, onChange }: Props) {
  const { t } = useTranslation();
  const theme = useTheme();
  const audio = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO" && s.index !== undefined);
  const subtitles = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_SUBTITLE" && s.index !== undefined);
  if (audio.length < 2 && subtitles.length === 0) return null;

  const audioChoices: Choice[] = [
    { key: "auto", label: t("media.streams.auto"), selected: pick.audio === undefined, onPress: () => onChange({ ...pick, audio: undefined }) },
    ...audio.map((s) => ({
      key: String(s.index),
      label: formatAudioTrackLabel(s, t("media.track", { number: s.index })),
      selected: pick.audio === s.index,
      onPress: () => onChange({ ...pick, audio: s.index }),
    })),
  ];
  const subtitleChoices: Choice[] = [
    { key: "auto", label: t("media.streams.auto"), selected: pick.subtitle === undefined, onPress: () => onChange({ ...pick, subtitle: undefined }) },
    { key: "off", label: t("media.streams.none"), selected: pick.subtitle === "off", onPress: () => onChange({ ...pick, subtitle: "off" }) },
    ...subtitles.map((s) => ({
      key: String(s.index),
      label: `${trackDisplayName(s, t("media.track", { number: s.index }))}${s.isForced ? ` - ${t("media.streams.forced")}` : ""}`,
      selected: pick.subtitle === s.index,
      onPress: () => onChange({ ...pick, subtitle: s.index }),
    })),
  ];

  const row = (title: string, choices: Choice[]) => (
    <View style={{ gap: 6 }}>
      <Text style={{ color: theme.colors.textMuted, fontSize: 12, fontWeight: "600" }}>{title}</Text>
      <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8 }}>
        {choices.map((c) => (
          <Pressable
            key={c.key}
            onPress={c.onPress}
            accessible={true}
            accessibilityRole="button"
            accessibilityState={{ selected: c.selected }}
            accessibilityLabel={`${title}: ${c.label}`}
            style={{
              paddingHorizontal: 12,
              paddingVertical: 6,
              borderRadius: theme.radii.md,
              backgroundColor: c.selected ? theme.colors.accent : theme.colors.surface,
            }}
          >
            <Text style={{ color: c.selected ? theme.colors.onAccent : theme.colors.text, fontSize: 13, fontWeight: "600" }}>
              {c.label}
            </Text>
          </Pressable>
        ))}
      </View>
    </View>
  );

  return (
    <View style={{ gap: theme.spacing.sm, marginTop: theme.spacing.md }} testID="track-pick-chips">
      {audio.length > 1 ? row(t("media.streams.audio"), audioChoices) : null}
      {subtitles.length > 0 ? row(t("media.streams.subtitles"), subtitleChoices) : null}
    </View>
  );
}
