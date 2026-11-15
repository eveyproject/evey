import { AppState } from "react-native";
import { clientUpdateBootGuard, watchClientUpdateSettle } from "@evey/client/native/clientUpdates";
import { clientUpdateBinding } from "./clientUpdates";

// Evaluated before the app (see index.js). Throws on purpose when the
// running update keeps dying: expo-updates then falls back to the previous
// bundle.
clientUpdateBootGuard(clientUpdateBinding);

// Armed here and not in a component: a launch must settle even when no
// screen ever mounts. The listener is never removed: it lives as long as the
// process, like this module.
watchClientUpdateSettle({
  ...clientUpdateBinding,
  onLeave: (listener) => {
    AppState.addEventListener("change", (state) => {
      if (state !== "active") listener();
    });
  },
});
