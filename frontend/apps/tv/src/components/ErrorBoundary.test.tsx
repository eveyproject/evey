import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// The user's requirement for the television was blunt: "aucune erreur ne doit
// faire planter l'app". That is a behaviour, so it needs tests - the boundary
// existing is not evidence that it recovers.

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, testID, numberOfLines, ...p }: any) =>
    React.createElement("span", { "data-testid": testID, ...p }, children),
  Pressable: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus, ...p }: any) =>
    React.createElement(
      "button",
      {
        onClick: onPress,
        "aria-label": accessibilityLabel,
        "data-tv-preferred-focus": hasTVPreferredFocus ? "true" : undefined,
        ...p,
      },
      children,
    ),
  StyleSheet: { create: (s: any) => s, absoluteFillObject: {} },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const replaceMock = vi.fn();
vi.mock("expo-router", () => ({ router: { replace: (...a: unknown[]) => replaceMock(...a) } }));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000",
      text: "#fff",
      textMuted: "#888",
      accent: "#f00",
      onAccent: "#000",
      surface: "#111",
      error: "#f00",
    },
  }),
}));

const logError = vi.fn();
vi.mock("../lib/log", () => ({
  logger: () => ({ error: (...a: unknown[]) => logError(...a), info: vi.fn(), warn: vi.fn() }),
}));

vi.mock("../lib/crashGuard", () => ({
  crashContext: () => "12:00:00 [player] source introuvable\n12:00:01 [ui] rendu interrompu",
}));

import { ErrorBoundary } from "./ErrorBoundary";

/** Throws until told otherwise, so a retry can be made to succeed or to fail again. */
function Bomb({ live }: { live: { throws: boolean } }) {
  if (live.throws) throw new Error("boom");
  return <span>contenu</span>;
}

describe("ErrorBoundary", () => {
  beforeEach(() => {
    // React logs caught errors to the console; silence the noise, not the test.
    vi.spyOn(console, "error").mockImplementation(() => {});
  });
  afterEach(() => {
    vi.restoreAllMocks();
    replaceMock.mockClear();
    logError.mockClear();
  });

  it("montre un ecran de secours au lieu de laisser l'erreur remonter", () => {
    render(
      <ErrorBoundary>
        <Bomb live={{ throws: true }} />
      </ErrorBoundary>,
    );
    expect(screen.getByText("common.somethingWentWrong")).toBeTruthy();
    expect(screen.getByText("boom")).toBeTruthy();
  });

  it("journalise l'erreur : degrader sans rien dire empeche tout diagnostic", () => {
    render(
      <ErrorBoundary>
        <Bomb live={{ throws: true }} />
      </ErrorBoundary>,
    );
    expect(logError).toHaveBeenCalled();
    expect(String(logError.mock.calls[0][0])).toContain("boom");
  });

  // Retry has to actually give the subtree another go once the cause is gone -
  // React unmounts the children when the fallback renders, so clearing the flag
  // is what brings them back.
  it("reessayer redonne sa chance au sous-arbre", () => {
    const live = { throws: true };
    render(
      <ErrorBoundary>
        <Bomb live={live} />
      </ErrorBoundary>,
    );
    live.throws = false;
    fireEvent.click(screen.getByLabelText("common.tryAgain"));
    expect(screen.getByText("contenu")).toBeTruthy();
    expect(screen.queryByText("common.somethingWentWrong")).toBeNull();
  });

  it("cesse de proposer un reessai qui rejoue la meme panne, et bascule sur l'accueil", () => {
    const live = { throws: true };
    render(
      <ErrorBoundary>
        <Bomb live={live} />
      </ErrorBoundary>,
    );
    fireEvent.click(screen.getByLabelText("common.tryAgain"));
    fireEvent.click(screen.getByLabelText("common.tryAgain"));

    expect(screen.queryByLabelText("common.tryAgain")).toBeNull();
    const home = screen.getByLabelText("common.nav.home");
    expect(home.getAttribute("data-tv-preferred-focus")).toBe("true");
    fireEvent.click(home);
    expect(replaceMock).toHaveBeenCalledWith("/(auth)/(tabs)");
  });

  // The television is the one surface with no log console and no adb reflex:
  // once retrying is off the table, whoever is in front of it has to be able to
  // read what led there, or the diagnosis is lost with the evening.
  it("montre le contexte de crash quand le reessai n'est plus propose", () => {
    const live = { throws: true };
    render(
      <ErrorBoundary>
        <Bomb live={live} />
      </ErrorBoundary>,
    );
    expect(screen.queryByTestId("crash-context")).toBeNull();

    fireEvent.click(screen.getByLabelText("common.tryAgain"));
    fireEvent.click(screen.getByLabelText("common.tryAgain"));

    expect(screen.getByTestId("crash-context").textContent).toContain("source introuvable");
  });

  // House rule: no state may leave the remote without a single, deterministic
  // place to land.
  it("expose toujours exactement une cible de focus initial", () => {
    render(
      <ErrorBoundary>
        <Bomb live={{ throws: true }} />
      </ErrorBoundary>,
    );
    expect(document.querySelectorAll('[data-tv-preferred-focus="true"]')).toHaveLength(1);
  });

  it("laisse passer les enfants quand rien n'echoue", () => {
    render(
      <ErrorBoundary>
        <span>ok</span>
      </ErrorBoundary>,
    );
    expect(screen.getByText("ok")).toBeTruthy();
  });
});
