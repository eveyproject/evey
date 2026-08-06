import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, disabled, ...p }: any) =>
    React.createElement("button", { onClick: onPress, disabled, ...p }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { border: "#333", surface: "#111", accent: "#f00", onAccent: "#fff", text: "#fff" },
  }),
}));

import { ChipSelect } from "./ChipSelect";

describe("ChipSelect", () => {
  const options = [
    { value: "", label: "Auto" },
    { value: "normal", label: "Normal" },
    { value: "low", label: "Low" },
  ];

  it("renders every option", () => {
    render(<ChipSelect options={options} value="" onChange={vi.fn()} />);
    expect(screen.getByText("Auto")).toBeTruthy();
    expect(screen.getByText("Normal")).toBeTruthy();
    expect(screen.getByText("Low")).toBeTruthy();
  });

  it("calls onChange with the pressed option's value", () => {
    const onChange = vi.fn();
    render(<ChipSelect options={options} value="" onChange={onChange} />);
    fireEvent.click(screen.getByText("Low"));
    expect(onChange).toHaveBeenCalledWith("low");
  });

  it("disables every chip when disabled", () => {
    render(<ChipSelect options={options} value="" onChange={vi.fn()} disabled />);
    const button = screen.getByText("Normal").closest("button");
    expect(button?.disabled).toBe(true);
  });
});
