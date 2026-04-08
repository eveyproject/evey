export const USERS_QUERY_KEY = ["/api/v1/admin/users"];
export const INVITES_QUERY_KEY = ["/api/v1/invites"];

export interface LibraryInfo {
  id: string;
  name: string;
}

export interface LibraryAccessData {
  libraryIds: string[];
  libraries: LibraryInfo[];
}

export const BITRATE_OPTIONS = [
  { label: "Unlimited", value: 0 },
  { label: "1 Mbps", value: 1000 },
  { label: "2 Mbps", value: 2000 },
  { label: "4 Mbps", value: 4000 },
  { label: "8 Mbps", value: 8000 },
  { label: "20 Mbps", value: 20000 },
  { label: "40 Mbps", value: 40000 },
];
