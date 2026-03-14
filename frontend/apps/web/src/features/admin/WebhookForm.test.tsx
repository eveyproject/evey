import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { WebhookForm } from "./WebhookForm";

const mutateMock = vi.fn();

vi.mock("@evey/api", () => ({
  useWebhookServiceCreateWebhook: () => ({ mutate: mutateMock, isPending: false }),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

describe("WebhookForm", () => {
  it("renders URL and Events inputs", () => {
    render(<WebhookForm onSuccess={vi.fn()} />, { wrapper: createWrapper() });
    expect(screen.getByLabelText("URL")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create Webhook" })).toBeInTheDocument();
  });

  it("Create Webhook button disabled when URL empty", () => {
    render(<WebhookForm onSuccess={vi.fn()} />, { wrapper: createWrapper() });
    expect(screen.getByRole("button", { name: "Create Webhook" })).toBeDisabled();
  });

  it("submits with URL and events on form submit", async () => {
    mutateMock.mockClear();
    render(<WebhookForm onSuccess={vi.fn()} />, { wrapper: createWrapper() });
    await userEvent.type(screen.getByLabelText("URL"), "https://example.com/hook");
    await userEvent.click(screen.getByRole("button", { name: "Create Webhook" }));
    expect(mutateMock).toHaveBeenCalledWith({
      data: { url: "https://example.com/hook", events: ["*"], enabled: true },
    });
  });

  it("parses comma-separated events correctly", async () => {
    mutateMock.mockClear();
    render(<WebhookForm onSuccess={vi.fn()} />, { wrapper: createWrapper() });
    await userEvent.type(screen.getByLabelText("URL"), "https://example.com/hook");
    const eventsInput = screen.getByDisplayValue("*");
    await userEvent.clear(eventsInput);
    await userEvent.type(eventsInput, "media.created, library.scanned");
    await userEvent.click(screen.getByRole("button", { name: "Create Webhook" }));
    expect(mutateMock).toHaveBeenCalledWith({
      data: { url: "https://example.com/hook", events: ["media.created", "library.scanned"], enabled: true },
    });
  });
});
