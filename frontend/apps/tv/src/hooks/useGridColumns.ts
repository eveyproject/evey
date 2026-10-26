import { useWindowDimensions } from "react-native";
import { gridColumns } from "@evey/client";
import { FOCUS_RING_WIDTH } from "../lib/focusRing";

// Gap between the cards of every poster grid (the columnWrapperStyle gap).
export const GRID_COLUMN_GAP = 12;
// What a grid's row cannot use: the screen's own horizontal padding, plus
// the sidebar rail on a tab screen.
export const DEEP_SCREEN_GRID_INSET = 24 * 2;
export const TAB_SCREEN_GRID_INSET = 72 + 24 * 2;

/** Columns a grid of `cardWidth` cards gets on this screen. */
export function useGridColumns(cardWidth: number, horizontalInset: number): number {
  const { width } = useWindowDimensions();
  // A card's footprint is its content plus the focus ring on each side.
  return gridColumns(width - horizontalInset, cardWidth + 2 * FOCUS_RING_WIDTH, GRID_COLUMN_GAP);
}
