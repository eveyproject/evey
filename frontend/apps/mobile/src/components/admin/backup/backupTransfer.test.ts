import { describe, it, expect } from "vitest";
import { isBackupRestorable, buildBackupDownloadUrl, buildBackupRestoreUrl } from "./backupTransfer";

describe("isBackupRestorable", () => {
  it("accepts a manifest with a non-empty id", () => {
    expect(isBackupRestorable({ id: "bkp-123" })).toBe(true);
  });

  it("rejects an empty id", () => {
    expect(isBackupRestorable({ id: "" })).toBe(false);
  });

  it("rejects a missing id", () => {
    expect(isBackupRestorable({} as { id: string })).toBe(false);
  });
});

describe("buildBackupDownloadUrl", () => {
  it("appends the access token as a query param", () => {
    expect(buildBackupDownloadUrl("https://evey.example", "bkp-1", "tok-abc")).toBe(
      "https://evey.example/api/v1/admin/backups/bkp-1?token=tok-abc",
    );
  });

  it("strips a trailing slash from the base URL", () => {
    expect(buildBackupDownloadUrl("https://evey.example/", "bkp-1", "tok-abc")).toBe(
      "https://evey.example/api/v1/admin/backups/bkp-1?token=tok-abc",
    );
  });

  it("omits the token param when there is no access token", () => {
    expect(buildBackupDownloadUrl("https://evey.example", "bkp-1", null)).toBe(
      "https://evey.example/api/v1/admin/backups/bkp-1",
    );
  });

  it("encodes an id that contains URL-unsafe characters", () => {
    expect(buildBackupDownloadUrl("https://evey.example", "bkp/../1", "t")).toBe(
      "https://evey.example/api/v1/admin/backups/bkp%2F..%2F1?token=t",
    );
  });
});

describe("buildBackupRestoreUrl", () => {
  it("points at the restore endpoint", () => {
    expect(buildBackupRestoreUrl("https://evey.example")).toBe("https://evey.example/api/v1/admin/backups/restore");
  });
});
