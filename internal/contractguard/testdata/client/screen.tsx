// Fixture client: reads all three response fields; only dead_field must flag.
export function Screen({ thing }: { thing: { liveField?: number; deadField?: number; kindTag?: number; nestedNote?: string } }) {
  return `${thing.liveField} ${thing.deadField} ${thing.kindTag}`;
}
