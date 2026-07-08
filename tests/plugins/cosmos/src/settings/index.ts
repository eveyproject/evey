// Composition root for the settings.user.appearance module (a separate
// iframe from the home widget, sharing the same plugin per-profile pref
// namespace).
import { SettingsApp } from "./SettingsApp";
import { EveyBridge } from "../adapters/evey/EveyBridge";

new SettingsApp(new EveyBridge()).start();
