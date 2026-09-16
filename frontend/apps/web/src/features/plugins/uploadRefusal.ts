// The upload route answers a refused archive (a broken manifest, a hash that
// does not match) with {"message": "..."}: that reason is what the admin needs
// to fix the archive, so it wins over the generic "install failed". Anything
// without a readable message keeps the fallback.
export async function uploadRefusalMessage(res: { ok: boolean; json: () => Promise<unknown> }, fallback: string): Promise<string> {
  if (res.ok) return fallback;
  const body = (await res.json().catch(() => null)) as { message?: unknown } | null;
  if (body && typeof body.message === "string" && body.message.trim()) return body.message;
  return fallback;
}
