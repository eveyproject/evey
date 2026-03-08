import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceListUsers,
  useUserServiceCreateUser,
  useUserServiceDeleteUser,
  useUserServiceDisableUser,
} from "@evey/api";
import type { V1User } from "@evey/api/src/generated/user.schemas";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Modal } from "@/components/ui/Modal";
import { Input } from "@/components/ui/Input";

export const Route = createFileRoute("/_auth/admin/users")({
  component: UsersPage,
});

const USERS_QUERY_KEY = ["/api/v1/admin/users"];

function CreateUserForm({ onSuccess }: { onSuccess: () => void }) {
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [isAdmin, setIsAdmin] = useState(false);

  const createUser = useUserServiceCreateUser({ mutation: { onSuccess } });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    createUser.mutate({ data: { username, email, password, isAdmin } });
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-4">
      <Input
        label="Username"
        value={username}
        onChange={(e) => setUsername(e.target.value)}
        required
      />
      <Input
        label="Email"
        type="email"
        value={email}
        onChange={(e) => setEmail(e.target.value)}
      />
      <Input
        label="Password"
        type="password"
        value={password}
        onChange={(e) => setPassword(e.target.value)}
        required
      />
      <label className="flex items-center gap-2 text-sm cursor-pointer" style={{ color: "var(--text)" }}>
        <input
          type="checkbox"
          checked={isAdmin}
          onChange={(e) => setIsAdmin(e.target.checked)}
        />
        Admin
      </label>
      <div className="flex justify-end pt-2">
        <Button type="submit" loading={createUser.isPending} disabled={!username || !password}>
          Create User
        </Button>
      </div>
    </form>
  );
}

function UserRow({ user }: { user: V1User }) {
  const queryClient = useQueryClient();

  const deleteUser = useUserServiceDeleteUser({
    mutation: { onSuccess: () => queryClient.invalidateQueries({ queryKey: USERS_QUERY_KEY }) },
  });

  const disableUser = useUserServiceDisableUser({
    mutation: { onSuccess: () => queryClient.invalidateQueries({ queryKey: USERS_QUERY_KEY }) },
  });

  return (
    <tr style={{ borderBottom: "1px solid var(--border)" }}>
      <td className="px-4 py-3 font-medium" style={{ color: "var(--text)" }}>
        {user.username}
      </td>
      <td className="px-4 py-3 text-sm" style={{ color: "var(--text-muted)" }}>
        {user.email ?? "—"}
      </td>
      <td className="px-4 py-3">
        {user.isAdmin ? <Badge variant="warning">Admin</Badge> : <Badge>User</Badge>}
      </td>
      <td className="px-4 py-3">
        {user.enabled !== false ? (
          <Badge variant="success">Enabled</Badge>
        ) : (
          <Badge variant="error">Disabled</Badge>
        )}
      </td>
      <td className="px-4 py-3 whitespace-nowrap">
        <div className="flex gap-2">
          <Button
            variant="secondary"
            loading={disableUser.isPending}
            onClick={() =>
              disableUser.mutate({ id: user.id!, params: { disabled: user.enabled !== false } })
            }
          >
            {user.enabled !== false ? "Disable" : "Enable"}
          </Button>
          <Button
            variant="danger"
            loading={deleteUser.isPending}
            onClick={() => {
              if (confirm(`Delete user "${user.username}"?`)) {
                deleteUser.mutate({ id: user.id! });
              }
            }}
          >
            Delete
          </Button>
        </div>
      </td>
    </tr>
  );
}

function UsersPage() {
  const [showModal, setShowModal] = useState(false);
  const queryClient = useQueryClient();
  const { data, isLoading } = useUserServiceListUsers();
  const users = data?.users ?? [];

  function handleCreateSuccess() {
    queryClient.invalidateQueries({ queryKey: USERS_QUERY_KEY });
    setShowModal(false);
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
          Users
        </h1>
        <Button onClick={() => setShowModal(true)}>Create User</Button>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : (
        <div
          className="overflow-x-auto rounded-[var(--radius-lg)]"
          style={{ border: "1px solid var(--border)" }}
        >
          <table className="w-full text-sm">
            <thead>
              <tr style={{ backgroundColor: "var(--surface)", borderBottom: "1px solid var(--border)" }}>
                {["Username", "Email", "Role", "Status", "Actions"].map((h) => (
                  <th
                    key={h}
                    className="px-4 py-2 text-left font-medium"
                    style={{ color: "var(--text-muted)" }}
                  >
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {users.length === 0 ? (
                <tr>
                  <td
                    colSpan={5}
                    className="px-4 py-8 text-center"
                    style={{ color: "var(--text-muted)" }}
                  >
                    No users found.
                  </td>
                </tr>
              ) : (
                users.map((u) => <UserRow key={u.id} user={u} />)
              )}
            </tbody>
          </table>
        </div>
      )}

      <Modal open={showModal} onClose={() => setShowModal(false)} title="Create User">
        <CreateUserForm onSuccess={handleCreateSuccess} />
      </Modal>
    </div>
  );
}
