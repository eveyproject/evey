import { createLazyFileRoute, Link } from "@tanstack/react-router";
import { ChevronLeft } from "lucide-react";
import { PageHeader } from "@/components/ui/PageHeader";
import { PluginIframe } from "@/features/plugins/PluginIframe";
import { usePluginSlotEntries } from "@/features/plugins/usePluginSlot";

export const Route = createLazyFileRoute("/_auth/admin/extensions/$pluginId")({
  component: PluginExtensionPage,
});

// Dedicated full-page route for a plugin's admin.nav module. The sidebar entry
// links here (instead of embedding the iframe inline), giving the plugin a
// real /admin URL to render into. The bridge's navigate() cannot be used to
// reach this or any other /admin/* route from inside the iframe - the
// allow-list (isAllowedNavPath) only covers /home, /library, /media, /player,
// /settings, /search.
function PluginExtensionPage() {
  const { pluginId } = Route.useParams();
  const entry = usePluginSlotEntries("admin.nav").find((e) => e.pluginId === pluginId);

  return (
    <div className="p-6">
      <Link
        to="/admin/plugins"
        className="inline-flex items-center gap-1 text-sm text-[var(--text-muted)] hover:text-[var(--text)] mb-4"
      >
        <ChevronLeft size={16} /> Plugins
      </Link>
      {entry ? (
        <>
          <PageHeader title={entry.name} />
          <div className="mt-4">
            <PluginIframe
              pluginId={entry.pluginId}
              slot="admin.nav"
              src={entry.entryPath}
              externalFetch={entry.externalFetch}
            />
          </div>
        </>
      ) : (
        <p className="text-[var(--text-muted)]">
          This plugin is not installed, not enabled, or has no admin page.
        </p>
      )}
    </div>
  );
}
