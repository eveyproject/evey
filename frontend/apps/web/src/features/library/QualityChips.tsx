import { hdrLabel, resolutionLabel, videoStream } from "@evey/client";
import type { V1MediaStream } from "@evey/api";

// The resolution and HDR of a title, as one welded chip on its poster. The
// colours are set for a poster, not for the page: they sit on any picture, so
// each has a fallback a theme that does not define the token still gets (an
// undefined token left the chip without a background). Themes restyle the
// chip through its stable class names: evey-quality, evey-quality-res,
// evey-quality-hdr. Every resolution wears the same chip: only HDR stands out.
export function qualityOf(streams?: V1MediaStream[], resolution?: string | null, isHdr?: boolean) {
  const video = videoStream(streams);
  const res = resolution ? (resolution === "2160p" ? "4K" : resolution) : resolutionLabel(video);
  const hdr = isHdr ? "HDR" : hdrLabel(video);
  // HDR10 is what "HDR" means on a poster; HLG says its own name.
  return { res, hdr: hdr === "HDR10" ? "HDR" : hdr };
}

export function QualityChips({ res, hdr, className = "" }: { res: string | null; hdr: string | null; className?: string }) {
  if (!res && !hdr) return null;
  return (
    <span
      className={`evey-quality inline-flex items-stretch overflow-hidden text-[10px] font-extrabold leading-none tracking-wide shadow-[0_1px_3px_rgba(0,0,0,0.6)] rounded-[var(--radius-sm)] ${className}`}
      data-testid="quality-chips"
    >
      {res && (
        <span
          className="evey-quality-res px-1.5 py-[3px]"
          style={{ background: "var(--badge-res-bg, #f5f5f4)", color: "var(--badge-res-text, #0c0a09)" }}
          data-testid="resolution-badge"
        >
          {res}
        </span>
      )}
      {hdr && (
        <span
          className="evey-quality-hdr px-1.5 py-[3px]"
          style={{ background: "var(--badge-hdr-bg, #f59e0b)", color: "var(--badge-hdr-text, #000000)" }}
          data-testid="hdr-badge"
        >
          {hdr}
        </span>
      )}
    </span>
  );
}
