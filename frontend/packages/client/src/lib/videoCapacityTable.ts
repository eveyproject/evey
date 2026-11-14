import type { V1VideoPool } from "@evey/api";

/** One column of the capacity table: a kind of encode inside one budget. */
export interface VideoCapacityColumn {
  pool: string;
  ladderClass: string;
  /** Translation key of the column heading. */
  labelKey: string;
}

/** One output height, with the streams each column fits at it (undefined when not offered). */
export interface VideoCapacityRow {
  height: number;
  streams: (number | undefined)[];
}

export interface VideoCapacityTable {
  columns: VideoCapacityColumn[];
  rows: VideoCapacityRow[];
}

function columnLabelKey(pool: string, ladderClass: string): string {
  if (pool === "gpu") return "admin.transcoding.videoPoolGpu";
  if (ladderClass === "software-hdr") return "admin.systemHealth.videoCapacityCpuHdr";
  if (ladderClass === "software-av1") return "admin.systemHealth.videoCapacityCpuAv1";
  return "admin.transcoding.videoPoolCpu";
}

/**
 * Lays the per-budget rung fits out as a table: one column per kind of encode,
 * one row per output height. These are the numbers admission enforces.
 */
export function videoCapacityTable(pools: V1VideoPool[] | undefined): VideoCapacityTable {
  const columns: VideoCapacityColumn[] = [];
  const fits: Map<number, number>[] = [];
  const heights = new Set<number>();
  for (const pool of pools ?? []) {
    for (const cls of pool.classes ?? []) {
      const byHeight = new Map<number, number>();
      for (const rung of cls.rungFits ?? []) {
        if (!rung.height) continue;
        byHeight.set(rung.height, rung.maxStreams ?? 0);
        heights.add(rung.height);
      }
      if (byHeight.size === 0) continue;
      const poolName = pool.pool ?? "";
      const ladderClass = cls.ladderClass ?? "";
      columns.push({ pool: poolName, ladderClass, labelKey: columnLabelKey(poolName, ladderClass) });
      fits.push(byHeight);
    }
  }
  const rows = [...heights]
    .sort((a, b) => a - b)
    .map((height) => ({ height, streams: fits.map((f) => f.get(height)) }));
  return { columns, rows };
}
