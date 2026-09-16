import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const api = vi.hoisted(() => ({ person: undefined as unknown, items: undefined as unknown, loading: false, listArgs: null as any }));
vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => ({ ...def, useParams: () => ({ personId: "p1" }) }),
  useRouter: () => ({ history: { back: vi.fn() }, navigate: vi.fn() }),
  useCanGoBack: () => false,
}));
vi.mock("@evey/api", () => ({
  useLibraryServiceGetPerson: () => ({ data: api.person }),
  useLibraryServiceListMediaItems: (a: unknown) => {
    api.listArgs = a;
    return { data: api.items, isLoading: api.loading };
  },
  personDepartmentKey: (d: string) => (d === "Acting" ? "media.person.department.acting" : null),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: Record<string, unknown> | string) => (o && typeof o === "object" ? `${k}:${Object.values(o).join(",")}` : k) }),
}));
vi.mock("@/components/ui/Badge", () => ({ Badge: ({ children }: any) => React.createElement("span", { "data-testid": "badge" }, children) }));
vi.mock("@/components/ui/Spinner", () => ({ Spinner: () => React.createElement("div", { "data-testid": "spinner" }) }));
vi.mock("@/features/library/MediaCardWithModals", () => ({
  MediaCardWithModals: ({ item }: any) => React.createElement("div", { "data-testid": "card" }, item.title),
}));

import { Route } from "./$personId.lazy";
const Page = (Route as unknown as { component: React.ComponentType }).component;

const item = (id: string, title: string, type: string, role?: string) => ({
  id, title, people: [{ id: "p1", type, role }, { id: "someone-else", type: "PERSON_TYPE_DIRECTOR" }],
});

beforeEach(() => {
  api.person = undefined;
  api.items = undefined;
  api.loading = false;
});

// The filmography is grouped by the role THIS person held on each title
// (never another credit on the same title), each card names the character,
// the known-for department is localised when it is one we know, and a
// missing portrait shows the initial - also after the image fails.
describe("person page", () => {
  it("groups the filmography by this person's role and names characters", () => {
    api.person = { name: "Jane Doe", knownFor: "Acting", placeOfBirth: "Lyon", biography: "Bio." };
    api.items = {
      items: [
        item("m1", "Film A", "PERSON_TYPE_ACTOR", "Nurse"),
        item("m2", "Film B", "PERSON_TYPE_ACTOR"),
        item("m3", "Film C", "PERSON_TYPE_DIRECTOR"),
        { id: "m4", title: "Film D", people: [] },
      ],
    };
    render(<Page />);
    expect(api.listArgs).toEqual({ personId: "p1", "pagination.pageSize": 200 });
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Jane Doe");
    expect(screen.getByText("media.person.titleCount:4")).toBeInTheDocument();
    expect(screen.getByText("media.person.department.acting")).toBeInTheDocument();
    expect(screen.getByText("Lyon")).toBeInTheDocument();
    expect(screen.getByText("Bio.")).toBeInTheDocument();

    const sections = screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent);
    expect(sections).toEqual(["media.person.actor(2)", "media.person.director(1)", "media.person.other(1)"]);
    expect(screen.getAllByTestId("badge").map((b) => b.textContent)).toEqual(["media.person.actor", "media.person.director", "media.person.other"]);
    expect(screen.getByText("media.person.asRole:Nurse")).toBeInTheDocument();
    expect(screen.getAllByText(/asRole/)).toHaveLength(1);
    expect(screen.getAllByTestId("card")).toHaveLength(4);
  });

  it("shows the initial without a portrait and swaps to it when the portrait fails", () => {
    api.person = { name: "jane", thumbUrl: "/thumb.jpg" };
    api.items = { items: [] };
    render(<Page />);
    const img = screen.getByRole("img", { name: "jane" }) as HTMLImageElement;
    const initial = screen.getByText("J") as HTMLElement;
    expect(initial.style.display).toBe("none");
    fireEvent.error(img);
    expect(img.style.display).toBe("none");
    expect(initial.style.display).toBe("flex");
    expect(screen.getByText("media.person.noFilmography")).toBeInTheDocument();
  });

  it("names an unknown person, shows the spinner while loading and no count yet", () => {
    api.loading = true;
    render(<Page />);
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("media.person.unknown");
    expect(screen.getByTestId("spinner")).toBeInTheDocument();
    expect(screen.queryByText(/titleCount/)).toBeNull();
    expect(screen.queryByText("media.person.noFilmography")).toBeNull();
  });
});
