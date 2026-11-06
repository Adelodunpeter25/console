import { expect, open, test } from "nsh/test";

test("sends a message and streams a scripted reply from Rust", async () => {
  const app = await open("./App.tsx");

  await app.type("id:composer", "hello spike");
  await app.keyboard.press("mod+return");

  await app.waitUntil(
    () => app.texts().join("\n").includes("End of the scripted reply"),
    { timeout: 30_000 },
  );
  expect(app.texts().join("\n")).toContain("hello spike");
});

test("return in the composer sends as well", async () => {
  const app = await open("./App.tsx");

  await app.type("id:composer", "second way");
  await app.submit("id:composer");

  await app.waitUntil(
    () => app.texts().join("\n").includes("End of the scripted reply"),
    { timeout: 30_000 },
  );
  expect(app.texts().join("\n")).toContain("second way");
});
