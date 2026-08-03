// One stand-in for every @expo/vector-icons family.
//
// The app imports each family by its own path (@expo/vector-icons/Ionicons)
// rather than through the barrel, so Metro ships only the fonts actually used
// instead of all nineteen. Under vitest those paths resolve to the real module,
// whose source vite cannot parse - hence this stub, aliased once in
// vitest.config.ts rather than mocked in every screen test.
import React from "react";

// Both attributes: the screen tests were written against per-file mocks that
// did not agree on one convention, and this stub replaces all of them.
function VectorIcon({ name }: { name?: string }) {
  return React.createElement("span", { "data-icon": name, "data-testid": `icon-${name}` });
}

export default VectorIcon;
export const Ionicons = VectorIcon;
export const MaterialIcons = VectorIcon;
