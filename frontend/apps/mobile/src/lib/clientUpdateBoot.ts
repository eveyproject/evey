import { clientUpdateBootGuard } from "@evey/client/native/clientUpdates";
import { clientUpdateBinding } from "./clientUpdates";

// Evaluated before the app (see index.js). Throws on purpose when the
// running update keeps dying: expo-updates then falls back to the previous
// bundle.
clientUpdateBootGuard(clientUpdateBinding);
