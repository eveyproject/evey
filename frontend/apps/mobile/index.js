// The update guard must run before any app code: an update that crashes at
// every launch can only be dropped from here.
import "./src/lib/clientUpdateBoot";
import "expo-router/entry";
