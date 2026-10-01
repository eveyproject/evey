import { useMemo } from "react";
import { View, Text, TouchableOpacity, TextInput, ScrollView, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import {
  languageDisplayName,
  type LibraryServiceListMediaItemsType,
  type LibraryServiceListMediaItemsSortField,
  type LibraryServiceListMediaItemsSortOrder,
} from "@evey/api";
import { BottomSheet } from "../BottomSheet";
import { GenreDot } from "../GenreDot";
import { useTheme } from "../../providers/ThemeProvider";
import type { FilterFacets } from "./libraryFilterUtils";

// Display name for a normalized audio/subtitle filter code. The shared rule
// (languageDisplayName) asks Intl first and falls back to a table of
// endonyms, because Hermes ships without Intl language data unless built
// against it: the panel once showed "en fr ru zh" where web said "English".
function langChipLabel(code: string, locale: string): string {
  return languageDisplayName(code, locale) ?? code;
}

const SORT_OPTIONS: Array<{ labelKey: string; value: LibraryServiceListMediaItemsSortField }> = [
  { labelKey: "media.library.titleSort", value: "MEDIA_SORT_FIELD_TITLE" },
  { labelKey: "media.library.year", value: "MEDIA_SORT_FIELD_YEAR" },
  { labelKey: "media.library.ratingSort", value: "MEDIA_SORT_FIELD_RATING" },
  { labelKey: "media.library.dateAdded", value: "MEDIA_SORT_FIELD_DATE_ADDED" },
];

const SHOW_TYPE_OPTIONS: Array<{ labelKey: string; value: LibraryServiceListMediaItemsType }> = [
  { labelKey: "media.library.series", value: "MEDIA_ITEM_TYPE_SERIES" },
  { labelKey: "media.library.seasons", value: "MEDIA_ITEM_TYPE_SEASON" },
  { labelKey: "media.library.episodes", value: "MEDIA_ITEM_TYPE_EPISODE" },
];

const RATING_STARS = [1, 2, 3, 4, 5] as const;

// Single-select chip row (sort field, sort order, type).
function SingleChip({ label, active, onPress }: { label: string; active: boolean; onPress: () => void }) {
  const theme = useTheme();
  return (
    <TouchableOpacity
      style={[styles.chip, { backgroundColor: active ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.sm }]}
      activeOpacity={0.7}
      onPress={onPress}
    >
      <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 13 }}>{label}</Text>
    </TouchableOpacity>
  );
}

// Multi-select chip (resolution/codec/audio+subtitle language/genres). Wraps
// onto multiple lines instead of the horizontal-scroll row used for single
// selects, since these lists can be long and the whole point is to see what's
// already picked at a glance.
// dot: show the genre's colour swatch. Only while inactive - the checkmark
// already occupies that slot on a selected chip, and two markers in front of a
// three-word label is noise, not information.
function MultiChip({ label, active, onPress, dot = false }: { label: string; active: boolean; onPress: () => void; dot?: boolean }) {
  const theme = useTheme();
  return (
    <TouchableOpacity
      style={[
        styles.chip,
        styles.multiChip,
        { backgroundColor: active ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.sm },
      ]}
      activeOpacity={0.7}
      onPress={onPress}
    >
      {active && <Ionicons name="checkmark" size={12} color={theme.colors.onAccent} style={{ marginRight: 4 }} />}
      {!active && dot && <View style={{ marginRight: 5 }}><GenreDot label={label} /></View>}
      <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 13 }}>{label}</Text>
    </TouchableOpacity>
  );
}

function SectionLabel({ children }: { children: string }) {
  const theme = useTheme();
  return <Text style={[styles.filterLabel, { color: theme.colors.textMuted }]}>{children}</Text>;
}

export interface LibraryFiltersPanelProps {
  visible: boolean;
  onClose: () => void;

  sortField: LibraryServiceListMediaItemsSortField;
  onSortFieldChange: (f: LibraryServiceListMediaItemsSortField) => void;
  sortOrder: LibraryServiceListMediaItemsSortOrder;
  onSortOrderChange: (o: LibraryServiceListMediaItemsSortOrder) => void;

  isShowLibrary: boolean;
  typeFilter: LibraryServiceListMediaItemsType | "";
  onTypeFilterChange: (v: LibraryServiceListMediaItemsType | "") => void;

  // Free-text single genre filter (mirrors web's GenreTextFilter) - kept
  // separate from the multi-select chips below since a genre chip search
  // still supports `genre` on ListMediaItems/FilterMediaItems/SearchMedia.
  genreText: string;
  onGenreTextChange: (v: string) => void;

  // Multi-select genre chips (mirrors web's GenreChips) - the actual "gap"
  // this panel closes: the library screen previously only had this single
  // free-text genre filter, no way to OR-match several genres server-side.
  genres: string[];
  onGenresChange: (v: string[]) => void;
  availableGenres: string[];

  yearMin: string;
  onYearMinChange: (v: string) => void;
  yearMax: string;
  onYearMaxChange: (v: string) => void;

  minRating?: number;
  onMinRatingChange: (v: number | undefined) => void;

  resolutions: string[];
  onResolutionsChange: (v: string[]) => void;
  codecs: string[];
  onCodecsChange: (v: string[]) => void;
  audioLangs: string[];
  onAudioLangsChange: (v: string[]) => void;
  subLangs: string[];
  onSubLangsChange: (v: string[]) => void;
  availableFacets: FilterFacets;

  activeCount: number;
  onReset: () => void;
}

function toggle(list: string[], value: string): string[] {
  return list.includes(value) ? list.filter((v) => v !== value) : [...list, value];
}

export function LibraryFiltersPanel(props: LibraryFiltersPanelProps) {
  const {
    visible, onClose,
    sortField, onSortFieldChange, sortOrder, onSortOrderChange,
    isShowLibrary, typeFilter, onTypeFilterChange,
    genreText, onGenreTextChange,
    genres, onGenresChange, availableGenres,
    yearMin, onYearMinChange, yearMax, onYearMaxChange,
    minRating, onMinRatingChange,
    resolutions, onResolutionsChange,
    codecs, onCodecsChange,
    audioLangs, onAudioLangsChange,
    subLangs, onSubLangsChange,
    availableFacets,
    activeCount, onReset,
  } = props;
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const langLabel = useMemo(() => (code: string) => langChipLabel(code, i18n.language), [i18n.language]);

  return (
    <BottomSheet visible={visible} onClose={onClose} title={t("media.library.filters")}>
      {activeCount > 0 && (
        <TouchableOpacity style={styles.resetRow} activeOpacity={0.7} onPress={onReset} testID="library-filters-reset">
          <Text style={{ color: theme.colors.accent, fontSize: 13, fontWeight: "600" }}>
            {t("media.library.clearAll")} ({activeCount})
          </Text>
        </TouchableOpacity>
      )}

      {/* Sort */}
      <SectionLabel>{t("media.search.sortBy")}</SectionLabel>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} style={{ marginBottom: 10 }}>
        {SORT_OPTIONS.map((opt) => (
          <SingleChip key={opt.value} label={t(opt.labelKey)} active={sortField === opt.value} onPress={() => onSortFieldChange(opt.value)} />
        ))}
      </ScrollView>
      <View style={{ flexDirection: "row", gap: 8, marginBottom: 10 }}>
        <SingleChip
          label={`↑ ${t("media.library.ascending") || "A-Z"}`}
          active={sortOrder === "SORT_ORDER_ASC"}
          onPress={() => onSortOrderChange("SORT_ORDER_ASC")}
        />
        <SingleChip
          label={`↓ ${t("media.library.descending") || "Z-A"}`}
          active={sortOrder === "SORT_ORDER_DESC"}
          onPress={() => onSortOrderChange("SORT_ORDER_DESC")}
        />
      </View>

      {/* Type (series/seasons/episodes) - shown only for SHOWS libraries */}
      {isShowLibrary && (
        <>
          <SectionLabel>{t("media.library.type", "Type")}</SectionLabel>
          <ScrollView horizontal showsHorizontalScrollIndicator={false} style={{ marginBottom: 10 }}>
            <SingleChip label={t("media.library.typeAll", "All")} active={!typeFilter} onPress={() => onTypeFilterChange("")} />
            {SHOW_TYPE_OPTIONS.map((opt) => (
              <SingleChip key={opt.value} label={t(opt.labelKey)} active={typeFilter === opt.value} onPress={() => onTypeFilterChange(opt.value)} />
            ))}
          </ScrollView>
        </>
      )}

      {/* Genre free-text quick filter (mirrors web's GenreTextFilter) */}
      <SectionLabel>{t("media.library.genreText")}</SectionLabel>
      <TextInput
        placeholder={t("media.library.genrePlaceholder")}
        placeholderTextColor={theme.colors.textMuted}
        value={genreText}
        onChangeText={onGenreTextChange}
        autoCapitalize="none"
        autoCorrect={false}
        style={[styles.yearInput, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.sm, borderColor: theme.colors.border, marginBottom: 10 }]}
      />

      {/* Genres (multi-select) - server-side OR-match via FilterMediaItems, mirrors web's GenreChips */}
      {availableGenres.length > 0 && (
        <>
          <SectionLabel>{t("media.library.genreLabel")}</SectionLabel>
          <View style={styles.wrapRow}>
            {availableGenres.map((g) => (
              <MultiChip key={g} label={g} active={genres.includes(g)} dot onPress={() => onGenresChange(toggle(genres, g))} />
            ))}
          </View>
        </>
      )}

      {/* Year range */}
      <SectionLabel>{t("media.search.year")}</SectionLabel>
      <View style={{ flexDirection: "row", gap: 8, marginBottom: 10 }}>
        <TextInput
          placeholder={t("media.search.fromYear")}
          placeholderTextColor={theme.colors.textMuted}
          value={yearMin}
          onChangeText={(v) => onYearMinChange(v.replace(/\D/g, ""))}
          keyboardType="number-pad"
          maxLength={4}
          style={[styles.yearInput, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.sm, borderColor: theme.colors.border }]}
        />
        <TextInput
          placeholder={t("media.search.toYear")}
          placeholderTextColor={theme.colors.textMuted}
          value={yearMax}
          onChangeText={(v) => onYearMaxChange(v.replace(/\D/g, ""))}
          keyboardType="number-pad"
          maxLength={4}
          style={[styles.yearInput, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.sm, borderColor: theme.colors.border }]}
        />
      </View>

      {/* Minimum rating (stars) */}
      <SectionLabel>{t("media.library.minRating")}</SectionLabel>
      <View style={{ flexDirection: "row", alignItems: "center", gap: 2, marginBottom: 10 }}>
        {RATING_STARS.map((star) => {
          const active = minRating != null && star <= minRating;
          return (
            <TouchableOpacity
              key={star}
              onPress={() => onMinRatingChange(minRating === star ? undefined : star)}
              accessibilityRole="button"
              accessibilityLabel={t("media.library.minStars", { count: star })}
              testID={`library-filter-star-${star}`}
            >
              <Ionicons name={active ? "star" : "star-outline"} size={22} color={active ? theme.colors.accent : theme.colors.textMuted} />
            </TouchableOpacity>
          );
        })}
        {minRating != null && (
          <TouchableOpacity onPress={() => onMinRatingChange(undefined)} style={{ marginLeft: 6 }}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{t("media.library.clearRating")}</Text>
          </TouchableOpacity>
        )}
      </View>

      {/* Resolution */}
      {availableFacets.resolutions.length > 0 && (
        <>
          <SectionLabel>{t("media.library.resolution")}</SectionLabel>
          <View style={styles.wrapRow}>
            {availableFacets.resolutions.map((r) => (
              <MultiChip key={r} label={r} active={resolutions.includes(r)} onPress={() => onResolutionsChange(toggle(resolutions, r))} />
            ))}
          </View>
        </>
      )}

      {/* Video codec */}
      {availableFacets.codecs.length > 0 && (
        <>
          <SectionLabel>{t("media.library.videoCodec")}</SectionLabel>
          <View style={styles.wrapRow}>
            {availableFacets.codecs.map((c) => (
              <MultiChip key={c} label={c} active={codecs.includes(c)} onPress={() => onCodecsChange(toggle(codecs, c))} />
            ))}
          </View>
        </>
      )}

      {/* Audio language */}
      {availableFacets.audioLangs.length > 0 && (
        <>
          <SectionLabel>{t("media.library.audioLanguage")}</SectionLabel>
          <View style={styles.wrapRow}>
            {availableFacets.audioLangs.map((l) => (
              <MultiChip key={l} label={langLabel(l)} active={audioLangs.includes(l)} onPress={() => onAudioLangsChange(toggle(audioLangs, l))} />
            ))}
          </View>
        </>
      )}

      {/* Subtitle language */}
      {availableFacets.subLangs.length > 0 && (
        <>
          <SectionLabel>{t("media.library.subtitleLanguage")}</SectionLabel>
          <View style={styles.wrapRow}>
            {availableFacets.subLangs.map((l) => (
              <MultiChip key={l} label={langLabel(l)} active={subLangs.includes(l)} onPress={() => onSubLangsChange(toggle(subLangs, l))} />
            ))}
          </View>
        </>
      )}

      {availableFacets.resolutions.length === 0 &&
        availableFacets.codecs.length === 0 &&
        availableFacets.audioLangs.length === 0 &&
        availableFacets.subLangs.length === 0 &&
        availableGenres.length === 0 && (
          <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{t("media.library.noFilterOptions")}</Text>
        )}
    </BottomSheet>
  );
}

const styles = StyleSheet.create({
  filterLabel: {
    fontSize: 11,
    fontWeight: "700",
    textTransform: "uppercase",
    letterSpacing: 0.5,
    marginBottom: 6,
  },
  chip: {
    paddingHorizontal: 12,
    paddingVertical: 6,
    marginRight: 6,
  },
  multiChip: {
    flexDirection: "row",
    alignItems: "center",
    marginBottom: 6,
  },
  wrapRow: {
    flexDirection: "row",
    flexWrap: "wrap",
    marginBottom: 10,
  },
  yearInput: {
    flex: 1,
    paddingVertical: 8,
    paddingHorizontal: 10,
    fontSize: 13,
    borderWidth: 1,
  },
  resetRow: {
    alignSelf: "flex-end",
    marginBottom: 10,
  },
});
