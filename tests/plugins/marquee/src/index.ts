// Composition root: wire adapters into the application. This is the only
// file that knows concrete implementations.

import { MarqueeApp } from "./application/MarqueeApp";
import { EveyBridge } from "./adapters/evey/EveyBridge";
import { GlRenderer } from "./adapters/render/GlRenderer";

new MarqueeApp({
  renderer: new GlRenderer(),
  bridge: new EveyBridge(),
}).start();
