// Tests for the markdown renderer on nsh's `TextView`: the desktop's type
// scales survive the swap, and every construct in the fixture document
// renders.
import { expect, open, test } from "nsh/test";
import { METRICS, textViewStyles } from "./render";

test("the type scales match the desktop's metrics", () => {
  expect(METRICS.BODY.textSize).toBe(12.5);
  expect(METRICS.BODY.lineHeight).toBe(20);
  expect(METRICS.USER_MESSAGE.textSize).toBe(14);
  expect(METRICS.COMPACT.textSize).toBe(11.5);
  expect(METRICS.COMPACT.blockGap).toBe(6);
});

test("textViewStyles carries the scale into TextView's parts", () => {
  const styles = textViewStyles(METRICS.COMPACT);
  const root = styles.root as { fontSize: number; lineHeight: number };
  expect(root.fontSize).toBe(11.5);
  expect(root.lineHeight).toBe(17);
  const code = styles.codeBlockCode as { fontSize: number };
  expect(code.fontSize).toBe(10.5);
  // Headings scale off the body size, like the desktop's heading_metrics.
  const h1 = styles.heading1 as { fontSize: number };
  expect(h1.fontSize).toBe(Math.round(11.5 * 1.45));
});

test("the fixture document renders every construct", async () => {
  const app = await open("./render.fixture.tsx", { platform: "linux" });
  const texts = app.texts().join("\n");
  for (const piece of [
    "bold",
    "Section",
    "First",
    "Task",
    "let x = 1; // done",
    "Quoted",
    "Name",
    "alpha",
    "12",
  ]) {
    expect(texts).toContain(piece);
  }
});

test("the scale prop switches metrics", async () => {
  const app = await open("./render.fixture.tsx", { platform: "linux" });
  expect(app.texts()).toContain("scale: body");
  await app.press("label:Toggle scale");
  expect(app.texts()).toContain("scale: compact");
});
