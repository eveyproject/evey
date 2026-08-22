// Shared with mobile and TV: see @evey/store/queryRetry for the policy and
// the incident it fixes. Kept as a thin re-export so main.tsx's existing
// import path (@/lib/queryRetry) does not need to change.
export { shouldRetryQuery } from "@evey/store";
