import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";

// Mirrors updatecheck.Status in the Go server: whether the registry publishes
// a newer image than the one running. The server only reads the registry;
// pulling and recreating the container stays the admin's.
export interface UpdateStatus {
  state: "up_to_date" | "update_available" | "unknown" | "disabled" | "pending";
  image?: string;
  currentVersion: string;
  currentCommit: string;
  latestVersion?: string;
  latestCommit?: string;
  latestCreated?: string;
  source?: string;
  reason?: string;
  checkedAt?: string;
}

export const UPDATE_STATUS_QUERY_KEY = ["/api/v1/admin/update-status"];

// What the admin runs next to their compose file.
export const UPDATE_COMMAND = "docker compose pull && docker compose up -d";

export function useUpdateStatus(enabled = true) {
  return useQuery<UpdateStatus>({
    queryKey: UPDATE_STATUS_QUERY_KEY,
    queryFn: () => eveyFetch<UpdateStatus>({ url: "/api/v1/admin/update-status", method: "GET" }),
    enabled,
    // The server checks every few hours; the page only needs its last answer.
    staleTime: 5 * 60_000,
    refetchInterval: 30 * 60_000,
  });
}

export function useCheckUpdateNow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => eveyFetch<UpdateStatus>({ url: "/api/v1/admin/update-status/check", method: "POST" }),
    onSuccess: (st) => qc.setQueryData(UPDATE_STATUS_QUERY_KEY, st),
  });
}

const shortSha = (sha?: string) => (sha ?? "").slice(0, 7);

// The label of what is published: the release tag, or the branch build's
// short commit.
export function latestLabel(st: UpdateStatus): string {
  if (st.latestVersion && st.latestVersion !== st.currentVersion) return st.latestVersion;
  return shortSha(st.latestCommit) || st.latestVersion || "";
}

// Where to read what changed: the release notes of a newer release, the
// commit range of a newer branch build. Null without a source repository.
export function changesUrl(st: UpdateStatus): string | null {
  const src = st.source?.replace(/\/+$/, "");
  if (!src || !/^https:\/\//.test(src)) return null;
  if (st.latestVersion && st.latestVersion !== st.currentVersion) return `${src}/releases`;
  if (st.latestCommit && st.currentCommit && st.currentCommit !== "unknown") {
    return `${src}/compare/${st.currentCommit}...${st.latestCommit}`;
  }
  return null;
}
