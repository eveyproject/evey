import { describe, it, expect } from "vitest";
import { Monitor, Smartphone, Tv } from "lucide-react";
import { deviceIcon, deviceLabel } from "./onlineDevice";

// The mobile/TV apps now send a product User-Agent ("EveyMobile (...)",
// "EveyTV (...)") instead of okhttp's anonymous default. The online panel
// must map those to the right icon and a human label - okhttp used to render
// as a desktop with a truncated "okhttp/4.12" caption.
describe("deviceIcon", () => {
  it("maps the Evey mobile app to a smartphone", () => {
    expect(deviceIcon("EveyMobile (Android 14; SM-G991B)")).toBe(Smartphone);
  });

  it("maps the Evey TV app to a TV", () => {
    expect(deviceIcon("EveyTV (Android TV 12; MiTV-AESP0)")).toBe(Tv);
  });

  it("keeps browsers on the desktop icon", () => {
    expect(deviceIcon("Mozilla/5.0 (Windows NT 10.0) Chrome/126.0")).toBe(Monitor);
  });
});

describe("deviceLabel", () => {
  it("labels the mobile app with its platform and device model", () => {
    expect(deviceLabel("EveyMobile (Android 14; SM-G991B)")).toBe("Evey Android · SM-G991B");
  });

  it("labels the mobile app on iOS", () => {
    expect(deviceLabel("EveyMobile (iOS 17.4)")).toBe("Evey iOS");
  });

  it("labels the TV app with its device model", () => {
    expect(deviceLabel("EveyTV (Android TV 12; MiTV-AESP0)")).toBe("Evey TV · MiTV-AESP0");
    expect(deviceLabel("EveyTV/0.1.7 (Android TV 12; MiTV-AESP0)")).toBe("Evey TV · MiTV-AESP0");
  });

  it("keeps the browser + OS label for web clients", () => {
    expect(deviceLabel("Mozilla/5.0 (Windows NT 10.0) Chrome/126.0 Safari/537.36")).toBe("Chrome · Windows");
  });

  it("labels a beaconing cast receiver as the Chromecast device", () => {
    expect(deviceLabel("EveyCast (Chromecast)")).toBe("Chromecast");
    expect(deviceIcon("EveyCast (Chromecast)")).toBe(Tv);
  });
});
