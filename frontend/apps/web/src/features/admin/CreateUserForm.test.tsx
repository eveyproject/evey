import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { CreateUserForm } from "./CreateUserForm";

const createUserMutate = vi.fn();
const setAccessMutateAsync = vi.fn();
let createUserOnSuccess: ((data: unknown) => Promise<void> | void) | undefined;
let librariesData: { libraries: Array<{ id: string; name: string }> } | undefined;

vi.mock("@evey/api", () => ({
  useUserServiceCreateUser: (opts?: { mutation?: { onSuccess?: (data: unknown) => void } }) => {
    createUserOnSuccess = opts?.mutation?.onSuccess;
    return { mutate: createUserMutate, isPending: false };
  },
  useLibraryServiceListLibraries: () => ({ data: librariesData, isLoading: false }),
  useLibraryAccessServiceSetUserLibraryAccess: () => ({
    mutateAsync: setAccessMutateAsync,
    isPending: false,
  }),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

describe("CreateUserForm", () => {
  beforeEach(() => {
    createUserMutate.mockReset();
    setAccessMutateAsync.mockReset();
    setAccessMutateAsync.mockResolvedValue(undefined);
    createUserOnSuccess = undefined;
    librariesData = undefined;
  });

  it("renders username/email/password inputs, admin checkbox, and submit button", () => {
    render(<CreateUserForm onSuccess={vi.fn()} />, { wrapper: createWrapper() });
    expect(screen.getByLabelText("Username")).toBeInTheDocument();
    expect(screen.getByLabelText("Email")).toBeInTheDocument();
    expect(screen.getByLabelText("Password")).toBeInTheDocument();
    expect(screen.getByLabelText("Admin")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create User" })).toBeInTheDocument();
  });

  it("renders a checkbox per library when listLibraries returns libs", () => {
    librariesData = {
      libraries: [
        { id: "lib-1", name: "Movies" },
        { id: "lib-2", name: "Shows" },
      ],
    };
    render(<CreateUserForm onSuccess={vi.fn()} />, { wrapper: createWrapper() });
    expect(screen.getByLabelText("Movies")).toBeInTheDocument();
    expect(screen.getByLabelText("Shows")).toBeInTheDocument();
  });

  it("submit button disabled when username or password empty", async () => {
    render(<CreateUserForm onSuccess={vi.fn()} />, { wrapper: createWrapper() });
    const btn = screen.getByRole("button", { name: "Create User" });
    expect(btn).toBeDisabled();
    await userEvent.type(screen.getByLabelText("Username"), "alice");
    expect(btn).toBeDisabled();
  });

  it("calls createUser.mutate with username/email/password/isAdmin on submit", async () => {
    render(<CreateUserForm onSuccess={vi.fn()} />, { wrapper: createWrapper() });
    await userEvent.type(screen.getByLabelText("Username"), "alice");
    await userEvent.type(screen.getByLabelText("Password"), "secret");
    await userEvent.click(screen.getByRole("button", { name: "Create User" }));
    expect(createUserMutate).toHaveBeenCalledWith({
      data: { username: "alice", email: "", password: "secret", isAdmin: false, maxProfiles: 0, maxDevicesPerProfile: 0 },
    });
  });

  it("after successful create with selected libs, calls setUserLibraryAccess with userId + libraryIds", async () => {
    librariesData = {
      libraries: [
        { id: "lib-1", name: "Movies" },
        { id: "lib-2", name: "Shows" },
      ],
    };
    const onSuccess = vi.fn();
    render(<CreateUserForm onSuccess={onSuccess} />, { wrapper: createWrapper() });
    await userEvent.type(screen.getByLabelText("Username"), "bob");
    await userEvent.type(screen.getByLabelText("Password"), "pwd");
    await userEvent.click(screen.getByLabelText("Movies"));
    await userEvent.click(screen.getByRole("button", { name: "Create User" }));
    expect(createUserMutate).toHaveBeenCalled();
    expect(createUserOnSuccess).toBeDefined();
    await createUserOnSuccess!({ id: "user-42" });
    expect(setAccessMutateAsync).toHaveBeenCalledWith({
      userId: "user-42",
      data: { libraryIds: ["lib-1"] },
    });
    expect(onSuccess).toHaveBeenCalled();
  });

  it("calls onSuccess after create when no libs selected", async () => {
    const onSuccess = vi.fn();
    render(<CreateUserForm onSuccess={onSuccess} />, { wrapper: createWrapper() });
    await userEvent.type(screen.getByLabelText("Username"), "bob");
    await userEvent.type(screen.getByLabelText("Password"), "pwd");
    await userEvent.click(screen.getByRole("button", { name: "Create User" }));
    await createUserOnSuccess!({ id: "user-1" });
    expect(setAccessMutateAsync).not.toHaveBeenCalled();
    expect(onSuccess).toHaveBeenCalled();
  });
});
