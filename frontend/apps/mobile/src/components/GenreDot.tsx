import { View } from "react-native";
import { genreDotColor } from "@evey/api";

/**
 * The coloured dot that identifies a genre - the mobile twin of the web's
 * GenreDot, sharing the same hash so a genre wears the same colour on both.
 *
 * No ring here, unlike on web: mobile only draws it on the panel's own
 * surfaces, never on the accent fill, so there is nothing for it to separate
 * from.
 */
export function GenreDot({ label, size = 8 }: { label: string; size?: number }) {
  return (
    <View
      // The colour carries no information the name does not; a screen reader
      // reading the genre out loud is the whole story.
      accessibilityElementsHidden
      importantForAccessibility="no-hide-descendants"
      style={{
        width: size,
        height: size,
        borderRadius: size / 2,
        backgroundColor: genreDotColor(label),
      }}
    />
  );
}
