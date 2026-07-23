import { eveyFetch } from "@evey/api";

// The account JWT must never ride in a download URL (reverse-proxy access
// logs, browser history hold it as a replayable credential). These helpers
// trade the JWT - sent once, in a header - for a short-lived opaque token
// bound to ONE resource id, and put THAT in the URL instead. Server side:
// POST /api/v1/download-token/{id} + the per-handler binding guard.

export async function mintDownloadToken(resourceId: string): Promise<string> {
  const res = await eveyFetch<{ token: string }>({
    url: `/api/v1/download-token/${encodeURIComponent(resourceId)}`,
    method: "POST",
  });
  return res.token;
}

// startTokenDownload mints and then triggers the browser download through a
// transient <a download>, the same trick DownloadButton uses for season ZIPs:
// an anchor can't send headers, so the token has to be minted at click time.
export async function startTokenDownload(baseUrl: string, resourceId: string, filename?: string): Promise<void> {
  const token = await mintDownloadToken(resourceId);
  const sep = baseUrl.includes("?") ? "&" : "?";
  const a = document.createElement("a");
  a.href = `${baseUrl}${sep}token=${encodeURIComponent(token)}`;
  if (filename) a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
}
