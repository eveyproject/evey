import { test as base, expect } from "@playwright/test";
import { STORAGE_STATE } from "../global-setup.ts";

export const test = base.extend({
  storageState: STORAGE_STATE,
});

export { expect };
