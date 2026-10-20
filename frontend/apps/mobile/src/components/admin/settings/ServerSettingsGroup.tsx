import { type ReactNode } from "react";
import { ActivityIndicator, StyleSheet, Switch, TextInput, View } from "react-native";
import { useTranslation } from "react-i18next";
import {
  useServerSettingsFormState,
  type ServerSettingField,
  type ServerSettingFieldView,
  type ServerSettingsFormState,
  type ServerSettingsTranslate,
} from "@evey/client";
import { FONT_SIZE } from "@evey/theme";
import { useTheme } from "../../../providers/ThemeProvider";
import { SettingRow } from "./SettingRow";
import { SettingsGroup } from "./SettingsGroup";
import { OptionGroup } from "./OptionGroup";
import { SaveButton } from "./SaveButton";

// useServerSettingsForm is the phone's binding of the shared server-settings
// form (@evey/client useServerSettingsFormState): same partial saves, same
// refetch and validation rules as the web.
export function useServerSettingsForm(fields: ServerSettingField[]): ServerSettingsFormState {
  const { t } = useTranslation();
  return useServerSettingsFormState(fields, t as unknown as ServerSettingsTranslate);
}

// ServerSettingFieldRow draws one field of the form as a SettingRow with the
// control its kind asks for.
export function ServerSettingFieldRow({ view: v }: { view: ServerSettingFieldView }) {
  const theme = useTheme();
  const f = v.spec;
  const common = {
    label: v.label,
    info: v.info,
    hint: v.hint,
    error: v.error,
    restart: v.restart,
    reset: v.reset,
    testID: `setting-row-${v.key}`,
  };

  if (f.kind === "toggle") {
    return (
      <SettingRow {...common} kind="toggle">
        <Switch
          value={Boolean(v.draft)}
          onValueChange={(x) => v.onChange(x)}
          disabled={v.locked}
          accessibilityLabel={v.label}
          testID={f.testId}
          trackColor={{ false: theme.colors.border, true: theme.colors.accent }}
        />
      </SettingRow>
    );
  }
  if (f.kind === "segmented" || f.kind === "select") {
    return (
      <SettingRow {...common} kind="wide">
        <OptionGroup
          label={v.label}
          options={v.options ?? []}
          value={String(v.draft)}
          onChange={(x) => v.onChange(x)}
          disabled={v.locked}
          formatUnknown={f.kind === "segmented" ? v.formatUnknown : undefined}
          testID={f.testId}
        />
      </SettingRow>
    );
  }
  return (
    <SettingRow {...common}>
      <TextInput
        value={String(v.draft)}
        onChangeText={(x) => v.onChange(x)}
        editable={!v.locked}
        placeholder={v.placeholder}
        placeholderTextColor={theme.colors.textMuted}
        keyboardType={f.kind === "number" ? "number-pad" : "default"}
        autoCapitalize="none"
        autoCorrect={false}
        accessibilityLabel={v.label}
        testID={f.testId}
        style={[
          styles.input,
          {
            backgroundColor: theme.colors.surfaceRaised,
            color: theme.colors.text,
            borderRadius: theme.radii.md,
            borderColor: v.error ? theme.colors.danger : theme.colors.border,
            opacity: v.locked ? 0.6 : 1,
          },
        ]}
      />
    </SettingRow>
  );
}

interface ServerSettingsGroupProps {
  title: string;
  description?: string;
  info?: string;
  fields: ServerSettingField[];
  /** testID of the group's Save button. */
  saveTestID: string;
  /** A line shown above the fields (a note, a warning). */
  before?: ReactNode;
  testID?: string;
}

// ServerSettingsGroup is a titled card of server settings with its own Save.
export function ServerSettingsGroup({ title, description, info, fields, saveTestID, before, testID }: ServerSettingsGroupProps) {
  const theme = useTheme();
  const form = useServerSettingsForm(fields);
  return (
    <SettingsGroup
      title={title}
      description={description}
      info={info}
      testID={testID}
      footer={
        form.data ? (
          <SaveButton onPress={() => void form.submit()} loading={form.saving} disabled={!form.dirty} testID={saveTestID} />
        ) : undefined
      }
    >
      {form.isLoading || !form.data ? (
        <View style={styles.loading}>
          <ActivityIndicator color={theme.colors.accent} />
        </View>
      ) : (
        <>
          {before}
          {fields.map((key) => (
            <ServerSettingFieldRow key={key} view={form.field(key)} />
          ))}
        </>
      )}
    </SettingsGroup>
  );
}

const styles = StyleSheet.create({
  input: { borderWidth: StyleSheet.hairlineWidth, paddingHorizontal: 14, paddingVertical: 12, fontSize: FONT_SIZE.sm },
  loading: { paddingVertical: 16, alignItems: "center" },
});
