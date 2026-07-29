import { describe, it, expect } from "vitest";
import fallbackEn from "./fallback.en.json";

// The embedded floor exists so a user whose server is unreachable or not yet
// configured never reads key names on the first screens of the app. These are
// exactly the keys those screens render without their own inline fallback.
describe("embedded i18n fallback", () => {
  const required = [
    "auth.login.signIn",
    "auth.login.username",
    "auth.login.password",
    "auth.login.loginFailed",
    "auth.login.invalidCredentials",
    "auth.login.forgotPassword",
  ];

  it("covers the pre-auth screens' keys", () => {
    for (const key of required) {
      expect(fallbackEn, `missing ${key}`).toHaveProperty(key);
      expect(String((fallbackEn as Record<string, string>)[key]).length).toBeGreaterThan(0);
    }
  });

  it("holds real words, never key names", () => {
    for (const [key, value] of Object.entries(fallbackEn as Record<string, string>)) {
      expect(value, `${key} looks like a key`).not.toMatch(/^[a-z]+(\.[a-zA-Z]+)+$/);
    }
  });
});
