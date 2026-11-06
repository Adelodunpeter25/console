// Transparency checkerboard backdrop for image previews, ported from
// console-ui's `primitives/transparency.rs`: the fixed light checkerboard
// behind transparent images (theme-independent by design, like Figma or
// GitHub — a theme-derived backdrop just moves the invisibility problem to
// the other theme). Opaque images cover it entirely, so it is safe to render
// unconditionally.

import { Canvas } from "nsh";

/// The checker cell size in points.
const CELL = 12.0;
const LIGHT = "#f5f5f5";
const GRAY = "#cccccc";

/// Add as the first child of a `relative` image card, before the image.
export function TransparencyGrid() {
  return (
    <Canvas
      class="absolute inset-0"
      accessibilityLabel="Transparency checkerboard"
      draw={(cx, size) => {
        cx.fillStyle = GRAY;
        cx.fillRect(0, 0, size.width, size.height);
        cx.fillStyle = LIGHT;
        const cols = Math.ceil(size.width / CELL);
        const rows = Math.ceil(size.height / CELL);
        for (let row = 0; row < rows; row++) {
          for (let col = 0; col < cols; col++) {
            if ((row + col) % 2 !== 0) continue;
            cx.fillRect(col * CELL, row * CELL, CELL, CELL);
          }
        }
      }}
    />
  );
}
