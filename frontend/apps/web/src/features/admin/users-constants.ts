// Source these from the generated hooks so invalidateQueries actually
// matches what the orval-generated query helpers register. Hardcoding
// `/api/v1/admin/users` here previously made every CreateUser / EditUser /
// disable / delete mutation a no-op for the cache and forced an F5.
import {
  getUserServiceListUsersQueryKey,
  getUserServiceListInviteLinksQueryKey,
} from "@evey/api";

export const USERS_QUERY_KEY = getUserServiceListUsersQueryKey();
export const INVITES_QUERY_KEY = getUserServiceListInviteLinksQueryKey();

export const BITRATE_OPTIONS = [
  { label: "Unlimited", value: 0 },
  { label: "1 Mbps", value: 1000 },
  { label: "2 Mbps", value: 2000 },
  { label: "4 Mbps", value: 4000 },
  { label: "8 Mbps", value: 8000 },
  { label: "20 Mbps", value: 20000 },
  { label: "40 Mbps", value: 40000 },
];
