package pluginrunner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	domainplugin "github.com/evey-project/evey/internal/domain/plugin"
)

// CosignVerifier implements plugin.SignatureVerifier using the cosign CLI.
type CosignVerifier struct {
	policy  string // "off", "warn", "enforce"
	keyPath string // path to PEM public key
}

// NewCosignVerifier returns a CosignVerifier. policy must be "off", "warn",
// or "enforce". When policy is "off" or keyPath is empty, VerifySignature
// is a no-op that always returns (nil, nil).
func NewCosignVerifier(policy, keyPath string) *CosignVerifier {
	return &CosignVerifier{policy: policy, keyPath: keyPath}
}

// VerifySignature checks the cosign signature of the ZIP at zipPath.
// sigPath is the detached .sig file (empty → zipPath+".sig" is tried).
//
// Behaviour by policy:
//   - "off": returns (nil, nil) - no-op
//   - empty keyPath under "warn": returns (nil, nil) - best-effort, can't verify
//   - empty keyPath under "enforce": fails closed (config validation already
//     refuses to start in this state; this is defense-in-depth so a hand-built
//     enforce verifier without a key can never silently pass a plugin)
//   - "warn": returns (status, nil) even on failure - caller logs
//   - "enforce": returns (status, error) when verification fails - caller blocks
func (v *CosignVerifier) VerifySignature(ctx context.Context, zipPath, sigPath string) (*domainplugin.SignatureStatus, error) {
	if v.policy == "off" {
		return nil, nil
	}
	if v.keyPath == "" {
		// enforce without a key = the admin asked for mandatory signatures but
		// configured nothing to verify against. Never return a silent pass here:
		// that would defeat enforcement entirely. warn stays a no-op (best-effort).
		if v.policy == "enforce" {
			return &domainplugin.SignatureStatus{
					Verified: false,
					Error:    "cosign enforce policy is set but no public key is configured",
				},
				fmt.Errorf("plugin signature verification failed: enforce policy requires a public key but none is configured")
		}
		return nil, nil
	}
	if sigPath == "" {
		sigPath = zipPath + ".sig"
	}

	cosignBin, lookErr := exec.LookPath("cosign")
	if lookErr != nil {
		status := &domainplugin.SignatureStatus{
			Verified: false,
			Error:    "cosign binary not found in $PATH",
		}
		if v.policy == "enforce" {
			return status, fmt.Errorf("plugin signature verification failed: cosign binary not found in $PATH")
		}
		return status, nil
	}

	cmd := exec.CommandContext(ctx, cosignBin,
		"verify-blob",
		"--key", v.keyPath,
		"--signature", sigPath,
		"--insecure-ignore-tlog", // key-based local verification doesn't use Rekor
		zipPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if runErr := cmd.Run(); runErr != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = runErr.Error()
		}
		status := &domainplugin.SignatureStatus{Verified: false, Error: errMsg}
		if v.policy == "enforce" {
			return status, fmt.Errorf("plugin signature verification failed: %s", errMsg)
		}
		return status, nil
	}

	return &domainplugin.SignatureStatus{
		Verified:   true,
		VerifiedBy: v.keyPath,
	}, nil
}
