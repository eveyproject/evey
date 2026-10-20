import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
import React from "react";

vi.mock("react-native", async () => (await import("../../../test/rnDom")).rnDom);

vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { border: "#333", surface: "#111", surfaceRaised: "#222", accent: "#f00", onAccent: "#fff", text: "#fff" },
    radii: { sm: 4, md: 8, lg: 12 },
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

  // The chips used to wrap freely, leaving a ragged 2+1 grid on a phone.
  it("is a named radio group whose options all take the same width", () => {
    render(<ChipSelect label="Memory profile" options={options} value="normal" onChange={vi.fn()} />);
    const group = screen.getByRole("radiogroup", { name: "Memory profile" });
    const radios = within(group).getAllByRole("radio");
    expect(radios).toHaveLength(3);
    for (const r of radios) expect(JSON.parse(r.getAttribute("data-style") ?? "{}")).toMatchObject({ flexGrow: 1, flexBasis: 0 });
    expect(radios.find((r) => r.getAttribute("aria-checked") === "true")?.textContent).toBe("Normal");
  });
});
