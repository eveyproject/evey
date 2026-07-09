// Composition root: wire adapters into the application. This is the only
// file that knows concrete implementations.

import { LuminaApp } from "./application/LuminaApp";
import { EveyBridge } from "./adapters/evey/EveyBridge";
import { GlRenderer } from "./adapters/render/GlRenderer";

new LuminaApp({
  renderer: new GlRenderer(),
  bridge: new EveyBridge(),
}).start();
