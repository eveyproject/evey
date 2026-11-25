package main

import (
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	"github.com/eveyproject/evey/internal/config"
	domainplugin "github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/pkg/demo"
)

// newPluginAutoUpdater builds the hourly plugin check. On the public demo,
// where no visitor can approve anything, a release the check holds for one of
// the demo's own plugins (signed by the project key) is taken at once: held,
// it stayed on its first version forever.
func newPluginAutoUpdater(cfg *config.Config, installer *appplugin.InstallerService) *appplugin.AutoUpdater {
	u := appplugin.NewAutoUpdater(installer)
	if cfg.Server.Demo {
		u.WithHeldUpdateApprover(func(p *domainplugin.Plugin) bool {
			return demo.TakesHeldUpdate(p.Name, p.UpdateSource, p.PublisherKeyFingerprint)
		})
	}
	return u
}
