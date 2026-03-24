import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { CastCarousel } from "./MediaDetailCast";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to, params, ...rest }: { children: React.ReactNode; to: string; params?: Record<string, string>; [k: string]: unknown }) => {
    const href = params ? Object.entries(params).reduce((acc, [k, v]) => acc.replace(`$${k}`, v), to) : to;
    return <a href={href} {...rest}>{children}</a>;
  },
}));

const makePeople = (count: number) =>
  Array.from({ length: count }, (_, i) => ({
    id: `p${i}`,
    name: `Person ${i}`,
    type: "PERSON_TYPE_ACTOR" as const,
    role: i % 2 === 0 ? `Role ${i}` : undefined,
    thumbUrl: i === 0 ? "/thumb0.jpg" : undefined,
  }));

describe("CastCarousel", () => {
  it("returns null when people is empty", () => {
    const { container } = render(<CastCarousel people={[]} title="Cast" />);
    expect(container.innerHTML).toBe("");
  });

  it("renders section title", () => {
    render(<CastCarousel people={makePeople(3)} title="Cast & Crew" />);
    expect(screen.getByText("Cast & Crew")).toBeInTheDocument();
  });

  it("renders person names", () => {
    render(<CastCarousel people={makePeople(3)} title="Cast" />);
    expect(screen.getByText("Person 0")).toBeInTheDocument();
    expect(screen.getByText("Person 1")).toBeInTheDocument();
    expect(screen.getByText("Person 2")).toBeInTheDocument();
  });

  it("renders role when present", () => {
    render(<CastCarousel people={makePeople(2)} title="Cast" />);
    expect(screen.getByText("Role 0")).toBeInTheDocument();
  });

  it("renders initial when no thumb", () => {
    const people = [{ id: "p1", name: "John Doe", type: "PERSON_TYPE_ACTOR" as const }];
    render(<CastCarousel people={people} title="Cast" />);
    expect(screen.getByText("J")).toBeInTheDocument();
  });

  it("renders thumb image when present", () => {
    const people = [{ id: "p1", name: "Actor", thumbUrl: "/thumb.jpg", type: "PERSON_TYPE_ACTOR" as const }];
    render(<CastCarousel people={people} title="Cast" />);
    expect(screen.getByAltText("Actor")).toHaveAttribute("src", "/thumb.jpg");
  });

  it("hides scroll buttons when 6 or fewer people", () => {
    render(<CastCarousel people={makePeople(6)} title="Cast" />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("shows scroll buttons when more than 6 people", () => {
    render(<CastCarousel people={makePeople(8)} title="Cast" />);
    const buttons = screen.getAllByRole("button");
    expect(buttons).toHaveLength(2);
  });

  it("links to person page", () => {
    const people = [{ id: "abc", name: "Jane", type: "PERSON_TYPE_ACTOR" as const }];
    render(<CastCarousel people={people} title="Cast" />);
    expect(screen.getByText("Jane").closest("a")).toHaveAttribute("href", "/person/abc");
  });

  it("shows role label for non-actor types without role", () => {
    const people = [{ id: "d1", name: "Steven Spielberg", type: "PERSON_TYPE_DIRECTOR" as const }];
    render(<CastCarousel people={people} title="Crew" />);
    expect(screen.getByText("Steven Spielberg")).toBeInTheDocument();
    expect(screen.getByText("Director")).toBeInTheDocument();
  });
});
