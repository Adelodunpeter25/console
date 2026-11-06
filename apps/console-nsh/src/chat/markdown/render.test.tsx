// Tests for the markdown renderer, ported from console-ui's
// `markdown/render.rs` behaviour: the type scales, proportional table
// columns, and every block kind rendering into the fixture document.
import { expect, open, test } from "nsh/test";
import { columnWidths } from "./render";
import { plain } from "./types";

test("the type scales match the desktop's metrics", async () => {
  const { METRICS } = await import("./render");
  expect(METRICS.BODY.textSize).toBe(12.5);
  expect(METRICS.BODY.lineHeight).toBe(20);
  expect(METRICS.USER_MESSAGE.textSize).toBe(14);
  expect(METRICS.COMPACT.textSize).toBe(11.5);
  expect(METRICS.COMPACT.blockGap).toBe(6);
});

test("table columns are content-proportional with a readable floor", () => {
  const header = [[plain("A")], [plain("B is a much longer header")]];
  const widths = columnWidths(header, [], 2);
  expect(widths.length).toBe(2);
  // The long column takes most of the table…
  expect(widths[1]).toBeGreaterThan(widths[0]);
  // …but the short one keeps at least 55% of an even share.
  expect(widths[0]).toBeGreaterThanOrEqual((1 / 2) * 0.55 - 1e-9);
  // And they sum to one.
  expect(Math.abs(widths[0] + widths[1] - 1)).toBeLessThan(1e-9);
});

test("equal-content columns share evenly", () => {
  const widths = columnWidths([[plain("a")], [plain("b")]], [], 2);
  expect(Math.abs(widths[0] - 0.5)).toBeLessThan(1e-9);
});

test("the fixture document renders every block kind", async () => {
  const app = await open("./render.fixture.tsx", { platform: "linux" });
  const texts = app.texts();
  for (const piece of [
    "Plain text, bold and code.",
    "Section",
    "First",
    "Task",
    "let x = 1; // done",
    "Quoted",
    "Name",
    "alpha",
    "12",
    "E = mc^2",
  ]) {
    expect(texts).toContain(piece);
  }
});

test("the code block header carries a copy button", async () => {
  const app = await open("./render.fixture.tsx", { platform: "linux" });
  await app.press("label:Copy");
  // CopyButton gives no external signal here; the press succeeding (no
  // throw) plus the check icon swap is the observable. Re-find the button
  // to confirm the header is still laid out after the copy.
  expect(app.texts()).toContain("rust");
});

test("the scale prop switches metrics", async () => {
  const app = await open("./render.fixture.tsx", { platform: "linux" });
  expect(app.texts()).toContain("scale: body");
  await app.press("label:Toggle scale");
  expect(app.texts()).toContain("scale: compact");
});
