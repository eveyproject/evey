import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { LibraryFiltersPanel } from "./LibraryFilters";

vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick }: { children: React.ReactNode; onClick?: () => void }) => (
    <button onClick={onClick}>{children}</button>
  ),
}));

const emptyOptions = { resolutions: [], codecs: [], audioLangs: [], subLangs: [] };

const baseSearch = {};

describe("LibraryFiltersPanel", () => {
  it("year_from input calls onUpdate with yearFrom when changed", () => {
    const onUpdate = vi.fn();
    render(
      <LibraryFiltersPanel
        search={baseSearch}
        availableGenres={[]}
        availableOptions={emptyOptions}
        onUpdate={onUpdate}
      />
    );

    const fromInput = screen.getByPlaceholderText("From");
    fireEvent.change(fromInput, { target: { value: "2010" } });

    expect(onUpdate).toHaveBeenCalledWith(
      expect.objectContaining({ yearFrom: 2010 })
    );
  });

  it("year_to input calls onUpdate with yearTo when changed", () => {
    const onUpdate = vi.fn();
    render(
      <LibraryFiltersPanel
        search={baseSearch}
        availableGenres={[]}
        availableOptions={emptyOptions}
        onUpdate={onUpdate}
      />
    );

    const toInput = screen.getByPlaceholderText("To");
    fireEvent.change(toInput, { target: { value: "2022" } });

    expect(onUpdate).toHaveBeenCalledWith(
      expect.objectContaining({ yearTo: 2022 })
    );
  });

  it("min_rating: clicking a star calls onUpdate with minRating", () => {
    const onUpdate = vi.fn();
    render(
      <LibraryFiltersPanel
        search={baseSearch}
        availableGenres={[]}
        availableOptions={emptyOptions}
        onUpdate={onUpdate}
      />
    );

    const starButtons = screen.getAllByTitle(/Min \d+ star/i);
    fireEvent.click(starButtons[2]); // 3rd star = min 3

    expect(onUpdate).toHaveBeenCalledWith({ minRating: 3 });
  });

  it("clicking 'Clear all' calls onUpdate to clear all filters", () => {
    const onUpdate = vi.fn();
    render(
      <LibraryFiltersPanel
        search={{ yearFrom: 2010, minRating: 3, genres: ["Action"] }}
        availableGenres={[]}
        availableOptions={emptyOptions}
        onUpdate={onUpdate}
      />
    );

    fireEvent.click(screen.getByText("Clear all"));

    expect(onUpdate).toHaveBeenCalledWith(
      expect.objectContaining({
        yearFrom: undefined,
        yearTo: undefined,
        minRating: undefined,
        genres: undefined,
      })
    );
  });

  it("genre chip click calls onUpdate with selected genres", () => {
    const onUpdate = vi.fn();
    render(
      <LibraryFiltersPanel
        search={baseSearch}
        availableGenres={[]}
        availableOptions={emptyOptions}
        onUpdate={onUpdate}
      />
    );

    // PREDEFINED_GENRES includes "Action"
    fireEvent.click(screen.getByRole("button", { name: "Action" }));

    expect(onUpdate).toHaveBeenCalledWith({ genres: ["Action"] });
  });

  it("resolution checkbox (span) click calls onUpdate with selected resolutions", () => {
    const onUpdate = vi.fn();
    render(
      <LibraryFiltersPanel
        search={baseSearch}
        availableGenres={[]}
        availableOptions={{ ...emptyOptions, resolutions: ["1080p", "4K"] }}
        onUpdate={onUpdate}
      />
    );

    // CheckboxGroup renders span[role="checkbox"] elements; get all and use first
    const checkboxes = screen.getAllByRole("checkbox");
    // First checkbox corresponds to "1080p"
    fireEvent.click(checkboxes[0]);

    expect(onUpdate).toHaveBeenCalledWith({ resolutions: ["1080p"] });
  });
});
