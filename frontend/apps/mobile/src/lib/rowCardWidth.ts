import { Dimensions } from "react-native";

// A poster in a home row is a third of a phone's width. Proportional alone,
// a tablet drew 280-420 dp posters and showed two of them: past the widest
// phone the card keeps its phone size and the row shows more of them.
export const ROW_CARD_MAX_WIDTH = 160;

export function rowCardWidth(windowWidth: number): number {
  return Math.min(windowWidth * 0.35, ROW_CARD_MAX_WIDTH);
}

export const ROW_CARD_WIDTH = rowCardWidth(Dimensions.get("window").width);
