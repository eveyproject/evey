import { useState } from "react";
import { View, Text, Pressable, ScrollView, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useMetadataServiceGetArtwork,
  getMetadataServiceGetArtworkQueryKey,
  getLibraryServiceGetMediaItemQueryKey,
  eveyFetch,
} from "@evey/api";
import { toast } from "@evey/store";
import { FONT_SIZE } from "@evey/theme";
import { useTheme } from "../../providers/ThemeProvider";

const POSTER_WIDTH = 96;
const BACKDROP_WIDTH = 160;

// Mirrors apps/web/src/features/library/MediaDetailEdit.tsx's ArtworkPicker:
// same endpoints, same "only show a picker once there's a genuine choice"
// rule (a single candidate isn't a choice). The web build highlights index 0
// as selected because the server always returns the currently-selected
// artwork first; that ordering contract is reused here rather than adding
// a second source of truth.
export function ArtworkPicker({ mediaItemId }: { mediaItemId: string }) {
  const { t } = useTranslation();
  const theme = useTheme();
  const qc = useQueryClient();
  const [selecting, setSelecting] = useState<string | null>(null);

  const { data } = useMetadataServiceGetArtwork(mediaItemId, undefined);
  const allArtwork = data?.artwork ?? [];
  const posters = allArtwork.filter((a) => a.type === "ARTWORK_TYPE_PRIMARY");
  const backdrops = allArtwork.filter((a) => a.type === "ARTWORK_TYPE_BACKDROP");

  async function selectArtwork(artworkId: string) {
    setSelecting(artworkId);
    try {
      await eveyFetch<void>({ url: `/api/v1/media/${mediaItemId}/artwork/${artworkId}/select`, method: "POST" });
      void qc.invalidateQueries({ queryKey: getMetadataServiceGetArtworkQueryKey(mediaItemId) });
      void qc.invalidateQueries({ queryKey: getLibraryServiceGetMediaItemQueryKey(mediaItemId) });
      toast.success(t("media.detail.artworkUpdated"));
    } catch {
      toast.error(t("media.detail.artworkFailed"));
    } finally {
      setSelecting(null);
    }
  }

  if (posters.length <= 1 && backdrops.length <= 1) return null;

  const s = styles(theme);

  return (
    <View style={{ gap: theme.spacing.md }}>
      {posters.length > 1 && (
        <View>
          <Text style={[s.label, { color: theme.colors.textMuted }]}>{t("media.detail.poster")}</Text>
          <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={s.row}>
            {posters.map((a, i) => (
              <Pressable
                key={a.id}
                onPress={() => a.id && selectArtwork(a.id)}
                disabled={selecting === a.id}
                accessibilityRole="button"
                accessibilityLabel={`${t("media.detail.poster")} ${i + 1}`}
                style={[
                  s.thumb,
                  {
                    width: POSTER_WIDTH,
                    aspectRatio: 2 / 3,
                    borderColor: i === 0 ? theme.colors.accent : "transparent",
                    opacity: selecting === a.id ? 0.5 : 1,
                  },
                ]}
              >
                <Image
                  source={{ uri: a.url }}
                  placeholder={a.blurhash ? { blurhash: a.blurhash } : undefined}
                  style={StyleSheet.absoluteFillObject}
                  contentFit="cover"
                />
              </Pressable>
            ))}
          </ScrollView>
        </View>
      )}
      {backdrops.length > 1 && (
        <View>
          <Text style={[s.label, { color: theme.colors.textMuted }]}>{t("media.detail.backdrop")}</Text>
          <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={s.row}>
            {backdrops.map((a, i) => (
              <Pressable
                key={a.id}
                onPress={() => a.id && selectArtwork(a.id)}
                disabled={selecting === a.id}
                accessibilityRole="button"
                accessibilityLabel={`${t("media.detail.backdrop")} ${i + 1}`}
                style={[
                  s.thumb,
                  {
                    width: BACKDROP_WIDTH,
                    aspectRatio: 16 / 9,
                    borderColor: i === 0 ? theme.colors.accent : "transparent",
                    opacity: selecting === a.id ? 0.5 : 1,
                  },
                ]}
              >
                <Image
                  source={{ uri: a.url }}
                  placeholder={a.blurhash ? { blurhash: a.blurhash } : undefined}
                  style={StyleSheet.absoluteFillObject}
                  contentFit="cover"
                />
              </Pressable>
            ))}
          </ScrollView>
        </View>
      )}
    </View>
  );
}

const styles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    label: { fontSize: FONT_SIZE.xs, fontWeight: "600", textTransform: "uppercase", marginBottom: 6 },
    row: { gap: t.spacing.sm, paddingBottom: 2 },
    thumb: { borderRadius: t.radii.sm, borderWidth: 2, overflow: "hidden" },
  });
