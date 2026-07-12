-- HDR ground truth for the HEVC copy/direct-play decision.
--
-- color_transfer: ffprobe color_transfer (smpte2084 = PQ/HDR10, arib-std-b67 =
-- HLG, bt709 = SDR, ...). Until now PQ and HLG were indistinguishable (only
-- color_space was stored) and the HDR type was guessed from bt2020 + 10-bit.
--
-- dovi_profile / dovi_bl_compat: Dolby Vision configuration record (ffprobe
-- side_data dv_profile + dv_bl_signal_compatibility_id). NULL = no DoVi.
-- Profile 5 (IPTC2 base layer) MUST always transcode - serving it raw renders
-- green/purple on any non-DoVi pipeline. Profile 8 with a compatible base
-- layer (bl_compat 1 = HDR10, 2 = SDR, 4 = HLG) may be served as its base.
--
-- NULL color_transfer on an HEVC stream means "not probed since this column
-- exists" and gates the HEVC copy rung OFF (same self-healing pattern as
-- level: the backfill worker re-probes and fills it).
ALTER TABLE media_streams ADD COLUMN IF NOT EXISTS color_transfer TEXT;
ALTER TABLE media_streams ADD COLUMN IF NOT EXISTS dovi_profile SMALLINT;
ALTER TABLE media_streams ADD COLUMN IF NOT EXISTS dovi_bl_compat SMALLINT;
