// Icon scale, the counterpart to the type scale in main.css's @theme block.
//
// The app carried 14 distinct lucide sizes, including one-off values (10, 11,
// 15, 22) that existed only because someone eyeballed a single spot. A ladder
// removes the guesswork: pick the step, not a number.
//
// Steps are paired with the type scale so an icon sits right next to its label:
//   ICON.xs  next to text-xs      ICON.md next to text-base
//   ICON.sm  next to text-sm      ICON.lg next to text-lg / buttons
export const ICON = {
  xs: 12,
  sm: 14,
  md: 16,
  lg: 20,
  xl: 24,
  "2xl": 32,
  "3xl": 40,
  "4xl": 48,
} as const;

export type IconSize = (typeof ICON)[keyof typeof ICON];
