// Categorical series palette. The hex literals stay as fallbacks on purpose -
// a categorical scale needs controlled hue separation, so it cannot simply
// inherit the theme accent for every slot. Routing each slot through its own
// variable is what lets a theme restyle charts; before, only slot 1 was
// reachable and the other seven were hard-coded past any theme.
// Defaults live with the other tokens in styles/main.css, where --chart-1 is
// wired to --accent so the first series keeps following the theme accent.
export const CHART_COLORS = [
  "var(--chart-1, #c81e1e)",
  "var(--chart-2, #f59e0b)",
  "var(--chart-3, #10b981)",
  "var(--chart-4, #ef4444)",
  "var(--chart-5, #8b5cf6)",
  "var(--chart-6, #ec4899)",
  "var(--chart-7, #14b8a6)",
  "var(--chart-8, #f97316)",
];
