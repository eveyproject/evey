// Device-quality gating: a weak device gets fewer galaxy nodes (more
// aggressive "other" merging) and no glow pass. Pure mapping, tier detection
// itself lives in the render adapter (it needs navigator/canvas probing).

export type QualityTier = "low" | "mid" | "high";

export function maxNodesForTier(tier: QualityTier): number {
  switch (tier) {
    case "low":
      return 6;
    case "mid":
      return 9;
    default:
      return 14;
  }
}

export function glowEnabledForTier(tier: QualityTier): boolean {
  return tier !== "low";
}
