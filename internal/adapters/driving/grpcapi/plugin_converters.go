package grpcapi

import (
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/plugin"
)

var pluginStateToProto = map[plugin.PluginStatus]eveyv1.PluginState{
	plugin.Stopped:      eveyv1.PluginState_PLUGIN_STATE_STOPPED,
	plugin.Initializing: eveyv1.PluginState(5),
	plugin.Running:      eveyv1.PluginState_PLUGIN_STATE_RUNNING,
	plugin.Error:        eveyv1.PluginState_PLUGIN_STATE_ERROR,
	plugin.Installed:    eveyv1.PluginState_PLUGIN_STATE_INSTALLED,
}

func pluginToProto(p *plugin.Plugin) *eveyv1.Plugin {
	pb := &eveyv1.Plugin{
		Id:            p.ID,
		Name:          p.Name,
		Description:   p.Description,
		Version:       p.Version,
		Author:        p.Author,
		Language:      p.Language,
		Enabled:       p.Enabled,
		State:         pluginStateToProto[p.Status],
		InstalledAt:   toTimestamppb(p.InstalledAt),
		Permissions:   p.Permissions,
		Capabilities:  p.Capabilities,
		SourceHash:    p.SourceHash,
		ErrorMessage:  p.ErrorMessage,
		Kind:          string(p.Kind),
		ExternalFetch: p.ExternalFetch,
		WebRoot:       p.WebRoot,
	}
	for _, m := range p.Frontend {
		pb.Frontend = append(pb.Frontend, &eveyv1.FrontModule{Slot: m.Slot, Src: m.Src})
	}
	if p.AnalysisReport != nil {
		rep := &eveyv1.PluginAnalysisReport{
			Safe:      p.AnalysisReport.Safe,
			Issues:    p.AnalysisReport.Issues,
			FileCount: int32(p.AnalysisReport.FileCount),
		}
		for _, u := range p.AnalysisReport.URLs {
			rep.Urls = append(rep.Urls, &eveyv1.PluginExtractedURL{
				Url:  u.URL,
				File: u.File,
				Line: int32(u.Line),
			})
		}
		pb.AnalysisReport = rep
	}
	pb.Signature = signatureToProto(p.Signature)
	return pb
}

// signatureToProto maps a signature verdict; nil (policy off) stays nil.
func signatureToProto(s *plugin.SignatureStatus) *eveyv1.PluginSignature {
	if s == nil {
		return nil
	}
	return &eveyv1.PluginSignature{
		Verified:       s.Verified,
		KeyFingerprint: s.KeyFingerprint,
		Publisher:      s.Publisher,
		Trusted:        s.Trusted,
		Continuity:     s.Continuity,
		Error:          s.Error,
	}
}

func trustedPublisherToProto(p *plugin.TrustedPublisher) *eveyv1.TrustedPublisher {
	return &eveyv1.TrustedPublisher{
		Fingerprint:  p.Fingerprint,
		Name:         p.Name,
		PublicKeyPem: p.PublicKeyPEM,
		AddedAt:      p.AddedAt.UTC().Format(time.RFC3339),
		Seeded:       p.Seeded,
	}
}

func pluginRuntimeToProto(rt *plugin.PluginRuntime) *eveyv1.PluginRuntime {
	return &eveyv1.PluginRuntime{
		Id:       rt.ID,
		Language: rt.Language,
		Version:  rt.Version,
	}
}
