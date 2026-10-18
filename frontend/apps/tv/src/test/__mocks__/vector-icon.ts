// Stand-in for @expo/vector-icons/Ionicons under vitest, whose real source
// vite cannot parse. The app imports the family by its own path rather than
// through the barrel, so Metro ships only the Ionicons font instead of all
// nineteen; a per-test vi.mock still overrides this.
import React from "react";

function VectorIcon({ name }: { name?: string }) {
  return React.createElement("span", { "data-icon": name, "data-testid": `icon-${name}` });
}

export default VectorIcon;
