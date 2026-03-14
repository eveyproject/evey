import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { AuditLogTable } from "./AuditLogTable";

const mockEntries = [
  {
    id: "1",
    createdAt: "2026-01-15T10:30:00Z",
    username: "alice",
    userId: "u1",
    action: "LOGIN",
    resourceType: "USER",
    resourceId: "u1",
    details: "Successful login",
    ipAddress: "192.168.1.1",
  },
  {
    id: "2",
    createdAt: "2026-01-15T11:00:00Z",
    username: undefined,
    userId: "u2",
    action: "LIBRARY_SCAN",
    resourceType: undefined,
    resourceId: undefined,
    details: undefined,
    ipAddress: undefined,
  },
];

describe("AuditLogTable", () => {
  it("renders empty state when no entries", () => {
    render(
      <AuditLogTable entries={[]} total={0} onPageChange={vi.fn()} page={0} pageSize={20} />
    );
    expect(screen.getByText("No entries found")).toBeInTheDocument();
  });

  it("renders table headers", () => {
    render(
      <AuditLogTable entries={[]} total={0} onPageChange={vi.fn()} page={0} pageSize={20} />
    );
    expect(screen.getByText("Time")).toBeInTheDocument();
    expect(screen.getByText("User")).toBeInTheDocument();
    expect(screen.getByText("Action")).toBeInTheDocument();
  });

  it("renders entries with data", () => {
    render(
      <AuditLogTable entries={mockEntries as never[]} total={2} onPageChange={vi.fn()} page={0} pageSize={20} />
    );
    expect(screen.getByText("alice")).toBeInTheDocument();
    expect(screen.getByText("LOGIN")).toBeInTheDocument();
    expect(screen.getByText("Successful login")).toBeInTheDocument();
    expect(screen.getByText("192.168.1.1")).toBeInTheDocument();
  });

  it("falls back to userId when username absent", () => {
    render(
      <AuditLogTable entries={mockEntries as never[]} total={2} onPageChange={vi.fn()} page={0} pageSize={20} />
    );
    expect(screen.getByText("u2")).toBeInTheDocument();
  });

  it("shows total entries count", () => {
    render(
      <AuditLogTable entries={mockEntries as never[]} total={42} onPageChange={vi.fn()} page={0} pageSize={20} />
    );
    expect(screen.getByText("42 entries")).toBeInTheDocument();
  });

  it("calls onPageChange when pagination clicked", async () => {
    const onPageChange = vi.fn();
    render(
      <AuditLogTable entries={mockEntries as never[]} total={100} onPageChange={onPageChange} page={0} pageSize={20} />
    );
    const nextBtn = screen.getByRole("button", { name: /next/i });
    await userEvent.click(nextBtn);
    expect(onPageChange).toHaveBeenCalledWith(1);
  });
});
