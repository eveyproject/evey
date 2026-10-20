import { useEffect, useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { PageHeader } from "@/components/ui/PageHeader";
import { Tabs, tabId, tabPanelId } from "@/components/ui/Tabs";
import { useServerConfig } from "@/features/admin/serverConfig/hooks";
import {
  SERVER_CONFIG_SECTIONS,
  ServerConfigSection,
  type ServerConfigSectionId,
} from "@/features/admin/serverConfig/ServerConfigSections";
import { DemoReadOnly } from "@/components/layout/DemoReadOnly";

export const Route = createLazyFileRoute("/_auth/admin/server-config")({
  component: ServerConfigPage,
});

const TABS_ID = "server-config";

function isSection(v: string): v is ServerConfigSectionId {
  return (SERVER_CONFIG_SECTIONS as readonly string[]).includes(v);
}

// sectionFromHash reads the open section from the address (#accounts), so a
// link or a reload lands on the section it named.
function sectionFromHash(): ServerConfigSectionId {
  if (typeof window === "undefined") return "general";
  const h = window.location.hash.replace(/^#/, "");
  return isSection(h) ? h : "general";
}

// ServerConfigPage splits the server settings into sections behind a tab
// menu. Every section stays mounted (only hidden), so an edit typed in one
// section survives a look at another before its Save.
function ServerConfigPage() {
  const { t } = useTranslation();
  const { data: entries, isLoading } = useServerConfig();
  const [section, setSection] = useState<ServerConfigSectionId>(sectionFromHash);

  useEffect(() => {
    const onHash = () => setSection(sectionFromHash());
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  function open(id: string) {
    if (!isSection(id)) return;
    setSection(id);
    try {
      window.history.replaceState(window.history.state, "", `#${id}`);
    } catch {
      // A sandboxed frame may refuse; the tab still switches.
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t("admin.serverConfig.title")} subtitle={t("admin.settingsLayout.pageSubtitle")} />

      {/* The menu sits outside DemoReadOnly: switching sections is reading,
          and the demo's disabled fieldset would freeze the tabs. */}
      <Tabs
        label={t("admin.settingsLayout.sectionsLabel")}
        idPrefix={TABS_ID}
        value={section}
        onChange={open}
        tabs={SERVER_CONFIG_SECTIONS.map((id) => ({ id, label: t(`admin.settingsLayout.sections.${id}.title`) }))}
      />

      <DemoReadOnly>
        {SERVER_CONFIG_SECTIONS.map((id) => (
          <div
            key={id}
            role="tabpanel"
            id={tabPanelId(TABS_ID, id)}
            aria-labelledby={tabId(TABS_ID, id)}
            tabIndex={0}
            hidden={id !== section}
          >
            <ServerConfigSection id={id} entries={entries} entriesLoading={isLoading} />
          </div>
        ))}
      </DemoReadOnly>
    </div>
  );
}
