import { describe, expect, it } from "vitest";
import { emailLinkFromLocation } from "./emailLink";

describe("emailLinkFromLocation", () => {
  it.each([
    ["/verify-email", "?token=abc_-1", { kind: "verify", token: "abc_-1" }],
    ["/reset-password/", "?token=xyz", { kind: "reset", token: "xyz" }],
    ["/reset-password", "", { kind: "reset", token: "" }],
    ["/keys", "?token=abc", null],
    ["/", "", null]
  ])("%s%s", (path, search, want) => {
    expect(emailLinkFromLocation(path, search)).toEqual(want);
  });
});
