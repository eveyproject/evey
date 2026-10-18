import { ActivityIndicator, Pressable, Text, View } from "react-native";
import { Stack, useLocalSearchParams, useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useUserServiceListUsers } from "@evey/api";
import { useTheme } from "../../../../src/providers/ThemeProvider";
import { HeaderBack } from "../../../../src/components/layout/HeaderBack";
import { EditUserScreen } from "../../../../src/features/admin/EditUserScreen";

export default function AdminEditUserRoute() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();

  const { data, isLoading, isError, refetch } = useUserServiceListUsers();
  const user = data?.users?.find((u: { id?: string }) => u.id === id);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: user?.username ?? t("admin.users.editUser", "Edit User"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
          headerLeft: () => <HeaderBack />,
        }}
      />
      {isLoading ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center" }}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : !user && isError ? (
        // A list that failed to load says nothing about the user: not "not found".
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center", gap: 12 }}>
          <Text style={{ color: theme.colors.textMuted }}>{t("common.loadError", "Failed to load")}</Text>
          <Pressable onPress={() => refetch()} accessibilityRole="button" hitSlop={8}>
            <Text style={{ color: theme.colors.accent, fontWeight: "600" }}>{t("common.retry", "Retry")}</Text>
          </Pressable>
        </View>
      ) : !user ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center" }}>
          <Text style={{ color: theme.colors.textMuted }}>
            {t("admin.users.userNotFound", "User not found")}
          </Text>
        </View>
      ) : (
        <EditUserScreen
          user={user}
          onSaved={() => router.back()}
          onCancel={() => router.back()}
        />
      )}
    </View>
  );
}
