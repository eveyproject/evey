import { useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { AdminNav } from "@/features/admin/AdminNav";
import { useQueryClient } from "@tanstack/react-query";
import {
  useAdminServiceListPlugins,
  useAdminServiceEnablePlugin,
  useAdminServiceDisablePlugin,
  useAdminServiceUninstallPlugin,
  useAdminServiceInstallPlugin,
} from "@evey/api";
import { toast } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { Input } from "@/components/ui/Input";
import { PluginCard } from "@/features/admin/PluginCard";

export const Route = createLazyFileRoute("/_auth/admin/plugins")({
  component: PluginsPage,
});

const PLUGINS_QUERY_KEY = ["/api/v1/admin/plugins"];

function InstallPluginForm({ onSuccess }: { onSuccess: () => void }) {
  const [source, setSource] = useState("");
  const [name, setName] = useState("");

  const installPlugin = useAdminServiceInstallPlugin({ mutation: { onSuccess } });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    installPlugin.mutate({ data: { source, name } });
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-4">
      <Input
        label="Source URL"
        value={source}
        onChange={(e) => setSource(e.target.value)}
        placeholder="https://example.com/plugin.zip"
        required
      />
      <Input
        label="Name (optional)"
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder="my-plugin"
      />
      <div className="flex justify-end pt-2">
        <Button type="submit" loading={installPlugin.isPending} disabled={!source.trim()}>
          Install
        </Button>
      </div>
    </form>
  );
}

function PluginsPage() {
  const [showModal, setShowModal] = useState(false);
  const [uninstallTarget, setUninstallTarget] = useState<{ id: string; name: string } | null>(null);
  const queryClient = useQueryClient();
  const { data, isLoading } = useAdminServiceListPlugins();
  const plugins = data?.plugins ?? [];

  const invalidate = () => queryClient.invalidateQueries({ queryKey: PLUGINS_QUERY_KEY });

  const enablePlugin = useAdminServiceEnablePlugin({
    mutation: {
      onSuccess: () => { invalidate(); toast.success("Plugin enabled"); },
      onError: (err: Error) => toast.error(err.message || "Failed to enable plugin"),
    },
  });
  const disablePlugin = useAdminServiceDisablePlugin({
    mutation: {
      onSuccess: () => { invalidate(); toast.success("Plugin disabled"); },
      onError: (err: Error) => toast.error(err.message || "Failed to disable plugin"),
    },
  });
  const uninstallPlugin = useAdminServiceUninstallPlugin({
    mutation: {
      onSuccess: () => { invalidate(); toast.success("Plugin uninstalled"); },
      onError: (err: Error) => toast.error(err.message || "Failed to uninstall plugin"),
    },
  });

  function handleInstallSuccess() {
    invalidate();
    setShowModal(false);
    toast.success("Plugin installed");
  }

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-bold mb-2" style={{ color: "var(--text)" }}>
          Admin
        </h1>
        <AdminNav />
      </div>
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold" style={{ color: "var(--text)" }}>
          Plugins
        </h2>
        <Button onClick={() => setShowModal(true)}>Install Plugin</Button>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : plugins.length === 0 ? (
        <div
          className="text-center py-16 rounded-[var(--radius-lg)]"
          style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
        >
          <p style={{ color: "var(--text-muted)" }}>No plugins installed.</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {plugins.map((plugin) => (
            <PluginCard
              key={plugin.id}
              plugin={plugin}
              onEnable={() => enablePlugin.mutate({ id: plugin.id! })}
              onDisable={() => disablePlugin.mutate({ id: plugin.id! })}
              onUninstall={() => setUninstallTarget({ id: plugin.id!, name: plugin.name ?? "this plugin" })}
            />
          ))}
        </div>
      )}

      <Modal open={showModal} onClose={() => setShowModal(false)} title="Install Plugin">
        <InstallPluginForm onSuccess={handleInstallSuccess} />
      </Modal>

      <Modal open={!!uninstallTarget} onClose={() => setUninstallTarget(null)} title="Uninstall Plugin">
        <p className="text-sm text-[var(--text-muted)] mb-4">
          Uninstall plugin "{uninstallTarget?.name}"? This cannot be undone.
        </p>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setUninstallTarget(null)}>Cancel</Button>
          <Button
            variant="danger"
            loading={uninstallPlugin.isPending}
            onClick={() => {
              if (uninstallTarget) {
                uninstallPlugin.mutate({ id: uninstallTarget.id });
                setUninstallTarget(null);
              }
            }}
          >
            Uninstall
          </Button>
        </div>
      </Modal>
    </div>
  );
}
