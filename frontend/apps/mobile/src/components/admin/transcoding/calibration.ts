import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";

// The encode calibration the server measured at boot (GET
// /api/v1/admin/transcode/calibration), same shape and query key as the web's
// EncodeCalibrationCard, so a mode change on either refreshes both.
export type HWFallback = { class: string; codec: string; at: string };

export type BenchRungSample = {
  ladder: string;
  height: number;
  fps: number;
  required_fps: number;
  passing: boolean;
  parallel_encodes: number;
};

export type EncodeCapability = {
  software_max_height: number;
  software_hdr_max_height?: number;
  hw_max_height: number;
  hw_backend?: string;
  hw_codecs?: string[];
  hw_decode_codecs?: string[];
  hw_features?: string[];
  hw_device?: string;
  gpu_class?: string;
  hw_mode?: string;
  detected_backends?: string[];
  is_wsl?: boolean;
  benchmarked_at: string;
  benchmark_duration_ms: number;
  hw_fallbacks?: HWFallback[];
  bench_samples?: BenchRungSample[];
};

export const CALIBRATION_KEY = ["admin", "transcode-calibration"];

export function useCalibration() {
  return useQuery<EncodeCapability>({
    queryKey: CALIBRATION_KEY,
    queryFn: () => eveyFetch<EncodeCapability>({ url: "/api/v1/admin/transcode/calibration", method: "GET" }),
  });
}
