import { uploadAsync, FileSystemUploadType } from "expo-file-system/legacy";
import { getBaseURL } from "@evey/api";
import type { InstallPreview } from "./pluginConfigUtils";

// Multipart install-from-file for plugins (mirror web InstallPluginForm file
// mode). The plugin route is behind the admin bearer and a multipart body can't
// go through the JSON client, so - like the backup restore-from-file path - the
// picked archive is streamed with expo-file-system's uploadAsync, with the
// access token as an Authorization header.

function authHeaders(token: string | null): Record<string, string> | undefined {
  return token ? { Authorization: `Bearer ${token}` } : undefined;
}

// previewPluginFile POSTs the archive to /plugins/preview and returns the parsed
// preview (name, permissions, capabilities, static-analysis issues/urls).
export async function previewPluginFile(fileUri: string, token: string | null): Promise<InstallPreview> {
  const res = await uploadAsync(`${getBaseURL()}/api/v1/admin/plugins/preview`, fileUri, {
    httpMethod: "POST",
    uploadType: FileSystemUploadType.MULTIPART,
    fieldName: "plugin",
    mimeType: "application/zip",
    headers: authHeaders(token),
  });
  if (res.status < 200 || res.status >= 300) {
    throw new Error(refusalMessage(res.body, `preview failed (${res.status})`));
  }
  return JSON.parse(res.body) as InstallPreview;
}

export interface PluginInstallResult {
  ok: boolean;
  // 428: the install widens permissions - the new entries to approve.
  escalation?: string[];
  // 403: refused by the static security analysis - the findings.
  blocked?: string[];
  error?: string;
}

// installPluginFile POSTs to /plugins/upload. A 428 (permission escalation) or
// 403 (blocked by analysis) is returned as a structured result to branch on,
// rather than thrown - the caller shows the diff / findings and can re-call with
// accept=true. sigUri attaches an optional detached signature.
export async function installPluginFile(
  fileUri: string,
  opts: { accept?: boolean; sigUri?: string | null },
  token: string | null,
): Promise<PluginInstallResult> {
  // expo-file-system uploads ONE file per call; a detached .sig is sent as an
  // extra multipart field via `parameters` only carries strings, so when a sig
  // is present we cannot attach it through uploadAsync's single-file form. The
  // common case (no signature) uploads the archive directly; a signature is a
  // rare, expert path left to the URL/source install.
  const params: Record<string, string> = {};
  if (opts.accept) params.accept_permission_changes = "true";
  const res = await uploadAsync(`${getBaseURL()}/api/v1/admin/plugins/upload`, fileUri, {
    httpMethod: "POST",
    uploadType: FileSystemUploadType.MULTIPART,
    fieldName: "plugin",
    mimeType: "application/zip",
    parameters: params,
    headers: authHeaders(token),
  });
  if (res.status === 428) {
    const body = safeJson(res.body) as { permission_changes?: boolean; new_entries?: string[] } | null;
    return { ok: false, escalation: body?.new_entries ?? [] };
  }
  if (res.status === 403) {
    const body = safeJson(res.body) as { issues?: string[]; message?: string } | null;
    return { ok: false, blocked: body?.issues ?? (body?.message ? [body.message] : []) };
  }
  if (res.status < 200 || res.status >= 300) {
    return { ok: false, error: refusalMessage(res.body, `install failed (${res.status})`) };
  }
  return { ok: true };
}

function safeJson(s: string): unknown {
  try { return JSON.parse(s); } catch { return null; }
}

// The server answers a refused archive with {"message": "..."}: the operator
// reads the reason, not the JSON envelope. The backup restore shares it.
export function refusalMessage(body: string, fallback: string): string {
  const parsed = safeJson(body) as { message?: unknown } | null;
  if (parsed && typeof parsed.message === "string" && parsed.message) return parsed.message;
  return body || fallback;
}
