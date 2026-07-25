import { describe, it, expect } from "vitest";
import { capitalizeStatus, requestStatusI18nKey, STATUS_FILTERS } from "./mediaRequestsHelpers";

describe("capitalizeStatus", () => {
  it("capitalizes the first letter", () => {
    expect(capitalizeStatus("pending")).toBe("Pending");
    expect(capitalizeStatus("approved")).toBe("Approved");
  });

  it("defaults to Pending when status is missing", () => {
    expect(capitalizeStatus(undefined)).toBe("Pending");
    expect(capitalizeStatus("")).toBe("Pending");
  });
});

describe("requestStatusI18nKey", () => {
  it("maps a status to the shared requests.status* i18n key", () => {
    expect(requestStatusI18nKey("approved")).toBe("requests.statusApproved");
    expect(requestStatusI18nKey("rejected")).toBe("requests.statusRejected");
    expect(requestStatusI18nKey("later")).toBe("requests.statusLater");
  });

  it("falls back to Pending for an unset status", () => {
    expect(requestStatusI18nKey(undefined)).toBe("requests.statusPending");
  });
});

describe("STATUS_FILTERS", () => {
  it("lists all queue filters in the order the web page uses", () => {
    expect(STATUS_FILTERS).toEqual(["all", "pending", "approved", "rejected", "later"]);
  });
});
