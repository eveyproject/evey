import { describe, expect, it } from "vitest";
import { gpuPipelineSummary } from "./EncodeCalibrationCard";

const t = (_key: string, fallback: string) => fallback;

describe("gpuPipelineSummary", () => {
  it("lists every probed pipeline with its verdict", () => {
    const out = gpuPipelineSummary(
      { hw_tonemap: true, hw_10bit_transcode: true, hw_hdr10_encode: false, hw_overlay: true, hw_features: ["low_power_h264", "low_power_hevc", "bframes"] },
      t,
    );
    expect(out).toEqual([
      "HDR→SDR: yes",
      "HDR→SDR OpenCL (bt2390): no",
      "10-bit: yes",
      "HDR10 out (HEVC Main10): no",
      "subtitle overlay: yes",
      "low power: h264, hevc",
    ]);
  });

  it("reads an un-probed host as no everywhere", () => {
    expect(gpuPipelineSummary({}, t)).toEqual([
      "HDR→SDR: no",
      "HDR→SDR OpenCL (bt2390): no",
      "10-bit: no",
      "HDR10 out (HEVC Main10): no",
      "subtitle overlay: no",
      "low power: no",
    ]);
  });
});
