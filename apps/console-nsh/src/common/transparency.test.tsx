import { expect, open, test } from "nsh/test";

test("the checkerboard renders behind the image cards", async () => {
  const app = await open("./transparency.fixture.tsx", { platform: "linux" });
  expect(app.texts()).toContain("transparent");
  expect(app.texts()).toContain("opaque");
});
