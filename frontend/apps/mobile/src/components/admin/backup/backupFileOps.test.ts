import { describe, it, expect, vi, beforeEach } from "vitest";

// expo-file-system's object API is replaced by a recorder: the contract under
// test is which URL, headers and destination each operation uses, and that a
// restore round-trips through a temp file that is removed afterwards.
const fs = vi.hoisted(() => ({
  dirs: [] as { base: string; name: string; created: boolean }[],
  deleted: [] as string[],
  download: vi.fn(),
  upload: vi.fn(),
  dirExists: false,
}));
vi.mock("expo-file-system", () => {
  class Directory {
    base: string;
    name: string;
    exists = fs.dirExists;
    constructor(base: string, name: string) {
      this.base = base;
      this.name = name;
    }
    create() {
      fs.dirs.push({ base: this.base, name: this.name, created: true });
    }
  }
  class File {
    uri: string;
    constructor(dir: Directory, name: string) {
      this.uri = `${dir.base}/${dir.name}/${name}`;
    }
    delete() {
      fs.deleted.push(this.uri);
    }
    static downloadFileAsync = (...a: unknown[]) => fs.download(...a);
  }
  return { Directory, File, Paths: { document: "doc", cache: "cache" } };
});
vi.mock("expo-file-system/legacy", () => ({
  uploadAsync: (...a: unknown[]) => fs.upload(...a),
  FileSystemUploadType: { MULTIPART: "multipart" },
}));
vi.mock("@evey/api", () => ({ getBaseURL: () => "https://s/" }));

import { downloadBackupFile, restoreFromFile, restoreExistingBackup } from "./backupFileOps";

beforeEach(() => {
  fs.dirs.length = 0;
  fs.deleted.length = 0;
  fs.dirExists = false;
  fs.download.mockReset().mockImplementation(async (_url: string, dest: { uri: string }) => dest);
  fs.upload.mockReset().mockResolvedValue({ status: 200, body: "" });
});

// A download lands in the documents "backups" folder under the backup id,
// authenticated both by query token (redirect-safe) and header; a malformed
// backup with no id is refused before any I/O.
describe("downloadBackupFile", () => {
  it("downloads into documents/backups with the token in the header, never the URL", async () => {
    const uri = await downloadBackupFile({ id: "b 1" }, "tok");
    expect(uri).toBe("doc/backups/b 1.tar.gz");
    expect(fs.dirs).toEqual([{ base: "doc", name: "backups", created: true }]);
    expect(fs.download).toHaveBeenCalledWith(
      "https://s/api/v1/admin/backups/b%201",
      expect.objectContaining({ uri: "doc/backups/b 1.tar.gz" }),
      { headers: { Authorization: "Bearer tok" }, idempotent: true },
    );
  });

  it("skips creating an existing folder and sends no header without a token", async () => {
    fs.dirExists = true;
    await downloadBackupFile({ id: "b1" }, null);
    expect(fs.dirs).toEqual([]);
    expect(fs.download.mock.calls[0][0]).toBe("https://s/api/v1/admin/backups/b1");
    expect(fs.download.mock.calls[0][2]).toEqual({ headers: undefined, idempotent: true });
  });

  it("refuses a backup without an id before touching the filesystem", async () => {
    await expect(downloadBackupFile({ id: "" }, "tok")).rejects.toThrow("invalid backup id");
    expect(fs.download).not.toHaveBeenCalled();
  });
});

// An external archive streams straight to the multipart restore endpoint; a
// non-2xx answer is an error carrying the server's body.
describe("restoreFromFile", () => {
  it("uploads the local file as multipart with the bearer", async () => {
    await restoreFromFile("file:///picked.tar.gz", "tok");
    expect(fs.upload).toHaveBeenCalledWith("https://s/api/v1/admin/backups/restore", "file:///picked.tar.gz", {
      httpMethod: "POST", uploadType: "multipart", fieldName: "backup", mimeType: "application/gzip",
      headers: { Authorization: "Bearer tok" },
    });
  });

  it("turns a refused restore into an error with the server's message", async () => {
    fs.upload.mockResolvedValueOnce({ status: 409, body: "restore in progress" });
    await expect(restoreFromFile("f", null)).rejects.toThrow("restore in progress");
    fs.upload.mockResolvedValueOnce({ status: 500, body: "" });
    await expect(restoreFromFile("f", null)).rejects.toThrow("restore failed");
  });

  // The server refuses with http.Error(`{"message":"..."}`): the toast shows
  // the reason, not the JSON envelope.
  it("reads the message out of the server's JSON envelope", async () => {
    fs.upload.mockResolvedValueOnce({ status: 422, body: '{"message":"x"}' });
    await expect(restoreFromFile("f", null)).rejects.toThrow(/^x$/);
  });
});

// A server-side backup has no restore-by-id route: it is downloaded to a temp
// file in the cache, streamed back as multipart, and the temp copy removed
// whether the restore succeeded or not.
describe("restoreExistingBackup", () => {
  it("round-trips through a cache temp file and cleans it up", async () => {
    await restoreExistingBackup({ id: "b1" }, "tok");
    expect(fs.dirs).toEqual([{ base: "cache", name: "backup-restore", created: true }]);
    expect(fs.download.mock.calls[0][0]).toBe("https://s/api/v1/admin/backups/b1");
    expect(fs.upload.mock.calls[0][1]).toBe("cache/backup-restore/b1.tar.gz");
    expect(fs.deleted).toEqual(["cache/backup-restore/b1.tar.gz"]);
  });

  it("still removes the temp file when the restore is refused", async () => {
    fs.upload.mockResolvedValueOnce({ status: 400, body: "bad archive" });
    await expect(restoreExistingBackup({ id: "b1" }, null)).rejects.toThrow("bad archive");
    expect(fs.deleted).toEqual(["cache/backup-restore/b1.tar.gz"]);
  });

  it("reads the message out of the server's JSON envelope", async () => {
    fs.upload.mockResolvedValueOnce({ status: 422, body: '{"message":"x"}' });
    await expect(restoreExistingBackup({ id: "b1" }, null)).rejects.toThrow(/^x$/);
  });

  it("refuses a backup without an id", async () => {
    await expect(restoreExistingBackup({ id: "" }, null)).rejects.toThrow("invalid backup id");
    expect(fs.download).not.toHaveBeenCalled();
  });
});
