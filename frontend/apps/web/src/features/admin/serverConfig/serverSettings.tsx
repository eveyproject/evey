import { type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import {
  useServerSettingsFormState,
  type ServerSettingField,
  type ServerSettings,
  type ServerSettingsTranslate,
} from "@evey/client";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { Switch } from "@/components/ui/Switch";
import { Spinner } from "@/components/ui/Spinner";
import { SettingRow } from "@/components/ui/SettingRow";
import { SettingsGroup } from "@/components/ui/SettingsGroup";
import { SegmentedControl } from "@/components/ui/SegmentedControl";

// The /api/v1/admin/server-settings resource and its form live in
// @evey/client (useServerSettingsFormState), shared with the phone: the query
// key, the value type, the field list, what a save sends and what a refetch
// may overwrite. This file draws the web controls from the form's field view.
export {
  SERVER_SETTINGS_KEY,
  SERVER_SETTINGS_URL,
  SERVER_SETTING_FIELDS,
  useServerSettings,
  type SettingValue,
  type ServerSettings,
  type ServerSettingField,
} from "@evey/client";

export interface ServerSettingsForm {
  data: ServerSettings | undefined;
  isLoading: boolean;
  dirty: boolean;
  saving: boolean;
  /** Validates and saves the changed fields. Resolves false when an error kept something from being saved. */
  submit: () => Promise<boolean>;
  renderField: (key: ServerSettingField) => ReactNode;
}

// useServerSettingsForm edits a few fields of the server-settings resource
// (see useServerSettingsFormState for the rules) and renders each one as a
// web SettingRow.
export function useServerSettingsForm(fields: ServerSettingField[]): ServerSettingsForm {
  const { t } = useTranslation();
  const form = useServerSettingsFormState(fields, t as unknown as ServerSettingsTranslate);

  function renderField(key: ServerSettingField): ReactNode {
    const v = form.field(key);
    const f = v.spec;
    const id = `server-setting-${key}`;
    const common = {
      label: v.label,
      info: v.info,
      hint: v.hint,
      error: v.error,
      restart: v.restart,
      reset: v.reset,
      "data-testid": `setting-row-${key}`,
    };
    const draft = v.draft;

    if (f.kind === "toggle") {
      return (
        <SettingRow key={key} {...common} kind="toggle">
          <Switch checked={Boolean(draft)} onChange={() => v.onChange(!draft)} label={v.label} disabled={v.locked} data-testid={f.testId} />
        </SettingRow>
      );
    }
    if (f.kind === "segmented") {
      const labelId = `${id}-label`;
      return (
        <SettingRow key={key} {...common} labelId={labelId} kind="wide">
          <SegmentedControl
            id={id}
            label={v.label}
            labelledBy={labelId}
            data-testid={f.testId}
            value={String(draft)}
            disabled={v.locked}
            onChange={(x) => v.onChange(x)}
            options={v.options ?? []}
            formatUnknown={v.formatUnknown}
          />
        </SettingRow>
      );
    }
    if (f.kind === "select") {
      return (
        <SettingRow key={key} {...common} htmlFor={id}>
          <Select
            id={id}
            data-testid={f.testId}
            value={String(draft)}
            disabled={v.locked}
            onChange={(x) => v.onChange(x)}
            options={(v.options ?? []).map((o) => ({ label: o.label, value: o.value }))}
          />
        </SettingRow>
      );
    }
    return (
      <SettingRow key={key} {...common} htmlFor={id}>
        <Input
          id={id}
          data-testid={f.testId}
          className="w-full"
          type={f.kind === "number" ? "number" : "text"}
          inputMode={f.kind === "number" ? "numeric" : undefined}
          min={f.min}
          placeholder={v.placeholder}
          value={String(draft)}
          disabled={v.locked}
          aria-invalid={v.error ? true : undefined}
          onChange={(e) => v.onChange(e.target.value)}
        />
      </SettingRow>
    );
  }

  return { data: form.data, isLoading: form.isLoading, dirty: form.dirty, saving: form.saving, submit: form.submit, renderField };
}

interface ServerSettingsGroupProps {
  title: string;
  description?: string;
  info?: string;
  fields: ServerSettingField[];
  /** data-testid of the group's Save button. */
  saveTestId: string;
  /** A line shown above the fields (a note, a warning). */
  before?: ReactNode;
  "data-testid"?: string;
}

// ServerSettingsGroup is a titled card of server settings with its own Save.
export function ServerSettingsGroup({ title, description, info, fields, saveTestId, before, "data-testid": testId }: ServerSettingsGroupProps) {
  const { t } = useTranslation();
  const form = useServerSettingsForm(fields);
  return (
    <SettingsGroup
      title={title}
      description={description}
      info={info}
      data-testid={testId}
      footer={
        form.data ? (
          <Button onClick={() => void form.submit()} loading={form.saving} disabled={!form.dirty} data-testid={saveTestId}>
            {t("common.save")}
          </Button>
        ) : undefined
      }
    >
      {form.isLoading || !form.data ? (
        <div className="flex justify-center py-4"><Spinner size="md" /></div>
      ) : (
        <>
          {before}
          {fields.map((key) => form.renderField(key))}
        </>
      )}
    </SettingsGroup>
  );
}
