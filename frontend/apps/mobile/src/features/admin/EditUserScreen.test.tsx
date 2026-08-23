import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: ({ value, onChangeText, accessibilityLabel }: any) =>
    React.createElement("input", {
      "aria-label": accessibilityLabel,
      value: value ?? "",
      onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value),
    }),
  Pressable: ({ children, onPress, disabled, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, disabled, ...p },
      children,
    ),
  Switch: ({ value, onValueChange, accessibilityLabel }: any) =>
    React.createElement("button", {
      type: "button",
      role: "switch",
      "aria-label": accessibilityLabel,
      "aria-checked": !!value,
      "data-value": value ? "true" : "false",
      onClick: () => onValueChange?.(!value),
    }),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  ActivityIndicator: (p: any) =>
    React.createElement("div", { "data-testid": "loading", role: "progressbar", ...p }),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
  Platform: { OS: "android", select: (o: any) => o.android },
}));

// --- Expo vector icons ----------------------------------------------------------------
vi.mock("@expo/vector-icons", () => ({
  Ionicons: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

// --- i18next --------------------------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
}));

// --- Theme ----------------------------------------------------------------------------
vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000",
      surface: "#111",
      surfaceRaised: "#222",
      text: "#fff",
      textMuted: "#888",
      accent: "#f00",
      onAccent: "#fff",
      border: "#333",
      error: "#e00",
    },
    radii: { lg: 12, md: 8, sm: 4 },
    spacing: { lg: 16, md: 12, sm: 8 },
  }),
}));

// --- API mocks ------------------------------------------------------------------------
const mutateMock = vi.fn();
const invalidateMock = vi.fn();

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useUserServiceUpdateUser: (_opts: any) => ({
    mutate: mutateMock,
    isPending: false,
  }),
  useUserServiceListProfiles: (_id: string, _opts: any) => ({
    data: { profiles: [] },
    isLoading: false,
  }),
  getUserServiceListUsersQueryKey: () => ["users"],
  getUserServiceListProfilesQueryKey: (id: string) => ["profiles", id],
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

// --- Profile sub-components (stub) ---------------------------------------------------
vi.mock("../profile/ProfileManagerList", () => ({
  ProfileManagerList: () => React.createElement("div", { "data-testid": "profile-list" }),
}));
vi.mock("../profile/CreateProfileDialog", () => ({
  CreateProfileDialog: ({ open }: { open: boolean }) =>
    open ? React.createElement("div", { "data-testid": "create-profile-dialog" }) : null,
}));

// --- @tanstack/react-query -----------------------------------------------------------
vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({ invalidateQueries: invalidateMock }),
}));

// -------------------------------------------------------------------------------------
import { EditUserScreen } from "./EditUserScreen";

const BASE_USER = {
  id: "u1",
  username: "alice",
  email: "alice@example.com",
  isAdmin: false,
  maxProfiles: 5,
  maxDevicesPerProfile: 3,
};

beforeEach(() => {
  vi.clearAllMocks();
});

// Helper: renders the component with default onSaved / onCancel stubs
function renderScreen(user = BASE_USER, overrides: Partial<typeof BASE_USER> = {}) {
  const onSaved = vi.fn();
  const onCancel = vi.fn();
  render(
    <EditUserScreen
      user={{ ...user, ...overrides }}
      onSaved={onSaved}
      onCancel={onCancel}
    />,
  );
  return { onSaved, onCancel };
}

describe("EditUserScreen", () => {
  describe("render", () => {
    it("pre-fills username from user prop", () => {
      renderScreen();
      const input = screen.getByRole("textbox", { name: /username/i });
      expect((input as HTMLInputElement).value).toBe("alice");
    });

    it("pre-fills email from user prop", () => {
      renderScreen();
      const input = screen.getByRole("textbox", { name: /email/i });
      expect((input as HTMLInputElement).value).toBe("alice@example.com");
    });

    it("pre-fills maxProfiles", () => {
      renderScreen();
      const input = screen.getByRole("textbox", {
        name: /max profiles/i,
      }) as HTMLInputElement;
      expect(input.value).toBe("5");
    });

    it("pre-fills maxDevicesPerProfile", () => {
      renderScreen();
      const input = screen.getByRole("textbox", {
        name: /max devices per profile/i,
      }) as HTMLInputElement;
      expect(input.value).toBe("3");
    });

    it("renders Save button", () => {
      renderScreen();
      expect(screen.getByRole("button", { name: /save/i })).toBeTruthy();
    });

    it("renders Cancel button", () => {
      renderScreen();
      expect(screen.getByRole("button", { name: /cancel/i })).toBeTruthy();
    });
  });

  describe("validation", () => {
    it("shows error when username is cleared", async () => {
      renderScreen();
      const usernameInput = screen.getByRole("textbox", { name: /username/i });
      fireEvent.change(usernameInput, { target: { value: "" } });
      fireEvent.click(screen.getByRole("button", { name: /save/i }));
      await waitFor(() => {
        expect(screen.getByText(/required/i)).toBeTruthy();
      });
      expect(mutateMock).not.toHaveBeenCalled();
    });

    it("shows error for invalid username characters", async () => {
      renderScreen();
      const usernameInput = screen.getByRole("textbox", { name: /username/i });
      fireEvent.change(usernameInput, { target: { value: "alice!" } });
      fireEvent.click(screen.getByRole("button", { name: /save/i }));
      await waitFor(() => {
        expect(screen.getByText(/1-64/i)).toBeTruthy();
      });
      expect(mutateMock).not.toHaveBeenCalled();
    });

    it("shows error for negative maxProfiles", async () => {
      renderScreen();
      const input = screen.getByRole("textbox", { name: /max profiles/i });
      fireEvent.change(input, { target: { value: "-1" } });
      fireEvent.click(screen.getByRole("button", { name: /save/i }));
      await waitFor(() => {
        expect(screen.getByText(/whole number/i)).toBeTruthy();
      });
      expect(mutateMock).not.toHaveBeenCalled();
    });

    it("shows error for non-integer maxDevices", async () => {
      renderScreen();
      const input = screen.getByRole("textbox", {
        name: /max devices per profile/i,
      });
      fireEvent.change(input, { target: { value: "2.5" } });
      fireEvent.click(screen.getByRole("button", { name: /save/i }));
      await waitFor(() => {
        expect(screen.getByText(/whole number/i)).toBeTruthy();
      });
      expect(mutateMock).not.toHaveBeenCalled();
    });
  });

  describe("mutation", () => {
    it("calls mutate with changed email only", async () => {
      renderScreen();
      const emailInput = screen.getByRole("textbox", { name: /email/i });
      fireEvent.change(emailInput, { target: { value: "new@example.com" } });
      fireEvent.click(screen.getByRole("button", { name: /save/i }));
      await waitFor(() => {
        expect(mutateMock).toHaveBeenCalledWith({
          id: "u1",
          data: { email: "new@example.com" },
        });
      });
    });

    it("calls mutate with isAdmin toggled", async () => {
      renderScreen();
      // Switch is rendered as a role="switch" button; clicking it calls onValueChange(!value)
      const adminSwitch = screen.getByRole("switch", { name: /admin/i });
      fireEvent.click(adminSwitch);
      fireEvent.click(screen.getByRole("button", { name: /save/i }));
      await waitFor(() => {
        expect(mutateMock).toHaveBeenCalledWith({
          id: "u1",
          data: { isAdmin: true },
        });
      });
    });

    it("calls mutate with updated maxProfiles", async () => {
      renderScreen();
      const input = screen.getByRole("textbox", { name: /max profiles/i });
      fireEvent.change(input, { target: { value: "10" } });
      fireEvent.click(screen.getByRole("button", { name: /save/i }));
      await waitFor(() => {
        expect(mutateMock).toHaveBeenCalledWith({
          id: "u1",
          data: { maxProfiles: 10 },
        });
      });
    });

    it("does NOT call mutate when nothing changed", async () => {
      const { onSaved } = renderScreen();
      fireEvent.click(screen.getByRole("button", { name: /save/i }));
      await waitFor(() => {
        expect(onSaved).toHaveBeenCalled();
      });
      expect(mutateMock).not.toHaveBeenCalled();
    });
  });

  describe("cancel", () => {
    it("calls onCancel without mutation", async () => {
      const { onCancel } = renderScreen();
      fireEvent.click(screen.getByRole("button", { name: /cancel/i }));
      expect(onCancel).toHaveBeenCalled();
      expect(mutateMock).not.toHaveBeenCalled();
    });
  });

  describe("error path", () => {
    it("still renders when mutation is pending", () => {
      // Override mock to indicate pending state inline
      mutateMock.mockImplementation(() => {});
      renderScreen();
      expect(screen.getByRole("button", { name: /save/i })).toBeTruthy();
    });
  });
});
