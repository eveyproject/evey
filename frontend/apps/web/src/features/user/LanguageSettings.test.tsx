import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect } from "vitest";
import { LanguageSettings } from "./LanguageSettings";

describe("LanguageSettings", () => {
  it("renders language heading and description", () => {
    render(<LanguageSettings />);
    expect(screen.getAllByText("Language").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("Choose your preferred language")).toBeInTheDocument();
  });

  it("renders all language options", () => {
    render(<LanguageSettings />);
    expect(screen.getByText("English")).toBeInTheDocument();
    expect(screen.getByText("Français")).toBeInTheDocument();
    expect(screen.getByText("Español")).toBeInTheDocument();
    expect(screen.getByText("中文")).toBeInTheDocument();
    expect(screen.getByText("हिन्दी")).toBeInTheDocument();
  });

  it("changes language on selection", async () => {
    const user = userEvent.setup();
    render(<LanguageSettings />);
    const select = screen.getByRole("combobox");
    await user.selectOptions(select, "fr");
    expect(select).toHaveValue("fr");
  });
});
