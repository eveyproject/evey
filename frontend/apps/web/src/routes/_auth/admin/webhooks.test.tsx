import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockListWebhooks = vi.fn();
const mockDeleteWebhook = vi.fn();
const mockUpdateWebhook = vi.fn();
const mockInvalidateQueries = vi.fn();
const mockEveyFetch = vi.fn();

vi.mock("@evey/api", () => ({
  useWebhookServiceListWebhooks: (...args: unknown[]) => mockListWebhooks(...args),
  useWebhookServiceDeleteWebhook: (...args: unknown[]) => mockDeleteWebhook(...args),
  useWebhookServiceUpdateWebhook: (...args: unknown[]) => mockUpdateWebhook(...args),
  eveyFetch: (...args: unknown[]) => mockEveyFetch(...args),
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useDemoStore: (sel: (s: { demoLocked: string[] }) => unknown) => sel({ demoLocked: [] }),
}));

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  };
});

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
}));

vi.mock("@/features/admin/WebhookForm", () => ({
  WebhookForm: ({ onSuccess }: { onSuccess: () => void }) =>
    React.createElement("div", { "data-testid": "webhook-form" },
      React.createElement("button", { onClick: onSuccess }, "Submit form")
    ),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getWebhooksPage() {
  const mod = await import("./webhooks.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

const mockWebhook = {
  id: "wh-1",
  name: "My Webhook",
  url: "https://example.com/hook",
  events: ["media.created", "media.updated"],
  enabled: true,
  secret: "s3cr3t",
  createdAt: "2024-01-15T10:00:00Z",
};

describe("WebhooksPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    const mutate = vi.fn();
    mockDeleteWebhook.mockReturnValue({ mutate, isPending: false });
    mockUpdateWebhook.mockReturnValue({ mutate, isPending: false });
  });

  it("renders page heading", async () => {
    mockListWebhooks.mockReturnValue({ data: { webhooks: [] }, isLoading: false });
    const WebhooksPage = await getWebhooksPage();
    render(React.createElement(WebhooksPage), { wrapper: createWrapper() });
    expect(screen.getByText("Webhooks")).toBeInTheDocument();
  });

  it("shows empty state when no webhooks", async () => {
    mockListWebhooks.mockReturnValue({ data: { webhooks: [] }, isLoading: false });
    const WebhooksPage = await getWebhooksPage();
    render(React.createElement(WebhooksPage), { wrapper: createWrapper() });
    expect(screen.getByText(/No webhooks configured/i)).toBeInTheDocument();
  });

  it("renders webhook row with name and url", async () => {
    mockListWebhooks.mockReturnValue({ data: { webhooks: [mockWebhook] }, isLoading: false });
    const WebhooksPage = await getWebhooksPage();
    render(React.createElement(WebhooksPage), { wrapper: createWrapper() });
    expect(screen.getByText("My Webhook")).toBeInTheDocument();
    expect(screen.getByText("https://example.com/hook")).toBeInTheDocument();
  });

  it("renders event tags", async () => {
    mockListWebhooks.mockReturnValue({ data: { webhooks: [mockWebhook] }, isLoading: false });
    const WebhooksPage = await getWebhooksPage();
    render(React.createElement(WebhooksPage), { wrapper: createWrapper() });
    expect(screen.getByText("media.created")).toBeInTheDocument();
    expect(screen.getByText("media.updated")).toBeInTheDocument();
  });

  it("shows Enabled badge for enabled webhook", async () => {
    mockListWebhooks.mockReturnValue({ data: { webhooks: [mockWebhook] }, isLoading: false });
    const WebhooksPage = await getWebhooksPage();
    render(React.createElement(WebhooksPage), { wrapper: createWrapper() });
    expect(screen.getByText("Enabled")).toBeInTheDocument();
  });

  it("shows Disabled badge for disabled webhook", async () => {
    const disabled = { ...mockWebhook, enabled: false };
    mockListWebhooks.mockReturnValue({ data: { webhooks: [disabled] }, isLoading: false });
    const WebhooksPage = await getWebhooksPage();
    render(React.createElement(WebhooksPage), { wrapper: createWrapper() });
    expect(screen.getByText("Disabled")).toBeInTheDocument();
  });

  it("clicking Create Webhook opens modal", async () => {
    mockListWebhooks.mockReturnValue({ data: { webhooks: [] }, isLoading: false });
    const WebhooksPage = await getWebhooksPage();
    render(React.createElement(WebhooksPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Create Webhook/i }));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    expect(screen.getByTestId("webhook-form")).toBeInTheDocument();
  });

  it("clicking Delete opens confirm modal", async () => {
    mockListWebhooks.mockReturnValue({ data: { webhooks: [mockWebhook] }, isLoading: false });
    const WebhooksPage = await getWebhooksPage();
    render(React.createElement(WebhooksPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Delete/i }));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    expect(screen.getByText(/Delete Webhook/i)).toBeInTheDocument();
  });

  it("clicking Edit opens edit modal", async () => {
    mockListWebhooks.mockReturnValue({ data: { webhooks: [mockWebhook] }, isLoading: false });
    const WebhooksPage = await getWebhooksPage();
    render(React.createElement(WebhooksPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /^Edit$/i }));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    expect(screen.getByText(/Edit Webhook/i)).toBeInTheDocument();
  });
});
