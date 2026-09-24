// Shared implementation lives in @evey/client (packages/client/src/native/ThemeProvider.tsx).
// This module keeps the path the screens and their tests import.
export { ThemeProvider, useTheme } from "@evey/client/native/ThemeProvider";
// The CSS-variable to ThemeTokens mapping, re-exported for the callers and
// tests that import it from here.
export { mergeCSSVars } from "@evey/client";
