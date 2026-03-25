import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { TrailerSection } from "./MediaDetailTrailer";
import type { V1Trailer } from "@evey/api";

function makeTrailer(overrides: Partial<V1Trailer> = {}): V1Trailer {
  return { key: "abc123", site: "YouTube", language: "en", official: true, ...overrides };
}

describe("TrailerSection", () => {
  it("returns null when trailers undefined", () => {
    const { container } = render(<TrailerSection />);
    expect(container.innerHTML).toBe("");
  });

  it("returns null when trailers empty", () => {
    const { container } = render(<TrailerSection trailers={[]} />);
    expect(container.innerHTML).toBe("");
  });

  it("renders trailer buttons", () => {
    render(<TrailerSection trailers={[makeTrailer()]} />);
    expect(screen.getByRole("button")).toBeInTheDocument();
  });

  it("limits display to 3 trailers", () => {
    const trailers = Array.from({ length: 5 }, (_, i) =>
      makeTrailer({ key: `k${i}` }),
    );
    render(<TrailerSection trailers={trailers} />);
    expect(screen.getAllByRole("button")).toHaveLength(3);
  });

  it("sorts user-language trailers first", () => {
    const trailers = [
      makeTrailer({ key: "de1", language: "de" }),
      makeTrailer({ key: "en1", language: "en" }),
    ];
    render(<TrailerSection trailers={trailers} />);
    const buttons = screen.getAllByRole("button");
    expect(buttons).toHaveLength(2);
  });

  it("sorts official trailers before non-official", () => {
    const trailers = [
      makeTrailer({ key: "no", language: "en", official: false }),
      makeTrailer({ key: "yes", language: "en", official: true }),
    ];
    render(<TrailerSection trailers={trailers} />);
    expect(screen.getAllByRole("button")).toHaveLength(2);
  });
});

describe("TrailerPlayer (via TrailerSection)", () => {
  it("opens YouTube embed on click", () => {
    render(<TrailerSection trailers={[makeTrailer({ key: "yt1", site: "YouTube" })]} />);
    fireEvent.click(screen.getByRole("button"));
    const iframe = document.querySelector("iframe");
    expect(iframe).not.toBeNull();
    expect(iframe!.src).toContain("https://www.youtube.com/embed/yt1");
    expect(iframe!.src).toContain("autoplay=1");
  });

  it("opens Vimeo embed on click", () => {
    render(<TrailerSection trailers={[makeTrailer({ key: "vm1", site: "Vimeo" })]} />);
    fireEvent.click(screen.getByRole("button"));
    const iframe = document.querySelector("iframe");
    expect(iframe).not.toBeNull();
    expect(iframe!.src).toContain("https://player.vimeo.com/video/vm1");
  });

  it("shows unsupported message for unknown site", () => {
    render(<TrailerSection trailers={[makeTrailer({ key: "x", site: "Dailymotion" })]} />);
    fireEvent.click(screen.getByRole("button"));
    expect(document.querySelector("iframe")).toBeNull();
  });

  it("shows CC badge when trailer language differs from user lang", () => {
    render(<TrailerSection trailers={[makeTrailer({ language: "ja" })]} />);
    expect(screen.getByText("CC")).toBeInTheDocument();
  });

  it("hides CC badge when trailer matches user language", () => {
    render(<TrailerSection trailers={[makeTrailer({ language: "en" })]} />);
    expect(screen.queryByText("CC")).not.toBeInTheDocument();
  });

  it("closes overlay on scrim click", () => {
    render(<TrailerSection trailers={[makeTrailer({ key: "yt2", site: "YouTube" })]} />);
    fireEvent.click(screen.getByRole("button"));
    expect(document.querySelector("iframe")).not.toBeNull();
    const scrim = document.querySelector(".fixed.inset-0");
    expect(scrim).not.toBeNull();
    fireEvent.click(scrim!);
    expect(document.querySelector("iframe")).toBeNull();
  });

  it("closes overlay on close button click", () => {
    render(<TrailerSection trailers={[makeTrailer({ key: "yt3", site: "YouTube" })]} />);
    fireEvent.click(screen.getByRole("button"));
    expect(document.querySelector("iframe")).not.toBeNull();
    const closeBtn = screen.getByText(/✕/);
    fireEvent.click(closeBtn);
    expect(document.querySelector("iframe")).toBeNull();
  });

  it("skips trailer with missing key (returns null)", () => {
    const { container } = render(<TrailerSection trailers={[makeTrailer({ key: undefined, site: undefined })]} />);
    expect(container.querySelector("button")).toBeNull();
  });

  it("adds subtitle params for YouTube when language differs", () => {
    render(<TrailerSection trailers={[makeTrailer({ key: "sub1", site: "YouTube", language: "fr" })]} />);
    fireEvent.click(screen.getByRole("button"));
    const iframe = document.querySelector("iframe");
    expect(iframe!.src).toContain("cc_load_policy=1");
    expect(iframe!.src).toContain("cc_lang_pref=en");
  });
});
