package procfork

import "strings"

// childEnvExact names the variables a media child (ffmpeg, mkvgo, the prober)
// is handed as they are: where to find binaries and libraries, where to put
// temporary files, locale and time zone.
var childEnvExact = map[string]struct{}{
	"PATH": {}, "HOME": {}, "TMPDIR": {}, "TMP": {}, "TEMP": {},
	"LANG": {}, "LANGUAGE": {}, "TZ": {},
	"LD_LIBRARY_PATH": {}, "LD_PRELOAD": {},
	"XDG_RUNTIME_DIR": {}, "XDG_CACHE_HOME": {},
	"DRI_PRIME": {}, "DISPLAY": {}, "WAYLAND_DISPLAY": {},
	"FFREPORT": {},
}

// childEnvPrefixes names the variable families the hardware paths read:
// VA-API, NVIDIA and CUDA, VDPAU, Intel media and oneVPL, Mesa and AMD, the
// OpenCL loader, and ffmpeg's own log switches.
var childEnvPrefixes = []string{
	"LC_", "LIBVA_", "NVIDIA_", "CUDA_", "VDPAU_", "INTEL_", "ONEVPL_", "MFX_",
	"MESA_", "LIBGL_", "AMD_", "HSA_", "ROC_", "OCL_ICD_", "OPENCL_", "AV_LOG_",
}

// childEnv keeps, from the server's environment, what a media child needs
// and nothing of what it must not see: the database connection string, the
// provider passwords and the server's own tokens are in the parent's
// environment and would otherwise be readable in the child's /proc entry by
// anything that can read it, and by the child itself.
func childEnv(environ []string) []string {
	kept := make([]string, 0, len(environ))
	for _, kv := range environ {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || !childEnvAllowed(name) {
			continue
		}
		kept = append(kept, kv)
	}
	return kept
}

func childEnvAllowed(name string) bool {
	if _, ok := childEnvExact[name]; ok {
		return true
	}
	for _, p := range childEnvPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}
