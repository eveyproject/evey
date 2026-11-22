import { describe, it, expect, vi } from "vitest";
import { render } from "@testing-library/react";
import React from "react";

const hook = vi.hoisted(() => ({ calls: [] as unknown[][] }));
vi.mock("@evey/client/native/useClientUpdates", () => ({
  useClientUpdates: (...args: unknown[]) => {
    hook.calls.push(args);
    return { offerRestart: false, dismiss: () => {} };
  },
}));
const binding = vi.hoisted(() => ({ app: "tv" }));
vi.mock("../lib/clientUpdates", () => ({ clientUpdateBinding: binding }));
const reloadAsync = vi.hoisted(() => vi.fn());
vi.mock("expo-updates", () => ({ reloadAsync, useUpdates: () => ({ isUpdatePending: true }) }));

import { ClientUpdateFollower } from "./ClientUpdateFollower";

describe("ClientUpdateFollower", () => {
  it("follows the server's bundles, shows nothing and never reloads the app in place", () => {
    const { container } = render(React.createElement(ClientUpdateFollower));
    expect(container.innerHTML).toBe("");
    expect(hook.calls.at(-1)?.[0]).toBe(binding);
    expect(reloadAsync).not.toHaveBeenCalled();
  });
});
