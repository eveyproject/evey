import * as FileSystem from "expo-file-system";
import { eveyFetch } from "@evey/api";
import { getServerUrl } from "./server-config";

const docDir = (FileSystem as any).documentDirectory ?? (FileSystem as any).default?.documentDirectory ?? "";
const DOWNLOAD_DIR = `${docDir}downloads/`;

export interface DownloadedMedia {
  mediaItemId: string;
  title: string;
  filePath: string;
  fileSize: number;
  downloadedAt: string;
}

const INDEX_FILE = `${DOWNLOAD_DIR}index.json`;

async function ensureDir() {
  const info = await FileSystem.getInfoAsync(DOWNLOAD_DIR);
  if (!info.exists) {
    await FileSystem.makeDirectoryAsync(DOWNLOAD_DIR, { intermediates: true });
  }
}

async function readIndex(): Promise<DownloadedMedia[]> {
  try {
    const raw = await FileSystem.readAsStringAsync(INDEX_FILE);
    return JSON.parse(raw);
  } catch {
    return [];
  }
}

async function writeIndex(items: DownloadedMedia[]) {
  await ensureDir();
  await FileSystem.writeAsStringAsync(INDEX_FILE, JSON.stringify(items));
}

export async function getDownloads(): Promise<DownloadedMedia[]> {
  return readIndex();
}

export async function startDownload(
  mediaItemId: string,
  title: string,
  onProgress?: (progress: number) => void,
): Promise<string> {
  await ensureDir();
  const serverUrl = await getServerUrl();
  if (!serverUrl) throw new Error("Not authenticated");

  const { token } = await eveyFetch<{ token: string; mediaId: string }>({
    url: `/api/v1/media/${mediaItemId}/stream-token`,
    method: "POST",
  });
  const streamUrl = `${serverUrl}/api/v1/media/${mediaItemId}/stream?token=${token}`;
  const destPath = `${DOWNLOAD_DIR}${mediaItemId}.mp4`;

  const downloadResumable = FileSystem.createDownloadResumable(
    streamUrl,
    destPath,
    {},
    (downloadProgress) => {
      const pct = downloadProgress.totalBytesWritten / downloadProgress.totalBytesExpectedToWrite;
      onProgress?.(pct);
    },
  );

  const result = await downloadResumable.downloadAsync();
  if (!result) throw new Error("Download failed");

  const index = await readIndex();
  index.push({
    mediaItemId,
    title,
    filePath: destPath,
    fileSize: result.headers?.["content-length"] ? parseInt(result.headers["content-length"]) : 0,
    downloadedAt: new Date().toISOString(),
  });
  await writeIndex(index);

  return destPath;
}

export async function deleteDownload(mediaItemId: string): Promise<void> {
  const index = await readIndex();
  const item = index.find((i) => i.mediaItemId === mediaItemId);
  if (item) {
    try { await FileSystem.deleteAsync(item.filePath, { idempotent: true }); } catch {}
  }
  await writeIndex(index.filter((i) => i.mediaItemId !== mediaItemId));
}

export async function getDownloadPath(mediaItemId: string): Promise<string | null> {
  const index = await readIndex();
  const item = index.find((i) => i.mediaItemId === mediaItemId);
  if (!item) return null;
  const info = await FileSystem.getInfoAsync(item.filePath);
  return info.exists ? item.filePath : null;
}

export async function getTotalDownloadSize(): Promise<number> {
  const index = await readIndex();
  let total = 0;
  for (const item of index) {
    try {
      const info = await FileSystem.getInfoAsync(item.filePath);
      if (info.exists && info.size) total += info.size;
    } catch {}
  }
  return total;
}
