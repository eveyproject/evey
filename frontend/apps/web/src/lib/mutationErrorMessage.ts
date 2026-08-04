import i18n from "i18next";
import { classifyError } from "@/utils/errorClassification";

/**
 * What to say when a write failed and the surface that started it said nothing.
 * Kept coarse on purpose: the point is that the person learns the action did
 * not happen, and whether trying again can help.
 */
export function mutationErrorMessage(error: unknown): string {
  return classifyError(error) === "offline"
    ? i18n.t("common.offline.description")
    : i18n.t("common.error");
}
