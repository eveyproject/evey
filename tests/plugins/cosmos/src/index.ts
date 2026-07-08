// Composition root: wire adapters into the application. This is the only
// file that knows concrete implementations.

import { CosmosApp } from "./application/CosmosApp";
import { EveyBridge } from "./adapters/evey/EveyBridge";
import { GlRenderer } from "./adapters/render/GlRenderer";

new CosmosApp({
  renderer: new GlRenderer(),
  bridge: new EveyBridge(),
}).start();
