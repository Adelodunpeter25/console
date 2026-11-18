# Render & Preview

You have `render` and `preview` for showing interactive HTML pages inline in the chat: charts, dashboards, diagrams, mockups, and interactive prototypes.

## Workflow

1. Build a self-contained HTML document (`<!doctype html>...`) with inline `<style>` and `<script>`.
2. Call `preview({html})` (or `preview({path})`) to inspect a screenshot, measured `contentHeight`, and any console errors. You can also pass `click` or `type` to verify interactive controls work.
3. Fix any layout or script issues, then call `render({html, title, height})` (or `render({path, title, height})`) using the measured `contentHeight`.
4. Keep your written reply brief — the rendered page is the artifact.

If `preview` reports that no desktop app is connected, publish directly with `render`.

## Rules

- Pass **either** `html` (inline, ≤ 512 KiB) **or** `path` (existing `.html` file in the workspace, ≤ 4 MiB).
- Document must start with `<!doctype html>` or `<html>`.
- **Self-contained**: inline CSS and JS. Do not rely on relative asset paths; use data URIs or absolute `/api/fs/file/raw?path=...` URLs for workspace images.
- **No viewport height/width units** (`vh`, `vw`, `dvh`, `svh`, `lvh`): the card auto-fits its height to the page, so viewport units cause infinite growth. Use fixed pixel heights for charts/canvases and let content set the height.
- **Use Console theme CSS variables** on `:root` instead of hardcoded colors so the page follows dark/light mode:
  `--background`, `--foreground`, `--muted`, `--muted-foreground`, `--card`, `--card-foreground`, `--secondary`, `--border`, `--ring`, `--primary`, `--accent`, `--destructive`, `--warning`, `--success`, `--info`, `--code-background`, `--code-foreground`, `--chart-1`..`--chart-8`, `--radius`, `--font-sans`, `--font-mono`.
- Leave `html` and `body` with no outer card border or extra side padding — the base sheet already sets background, text color, font, and an 8px gutter.
- Never put credentials, tokens, or secrets in a page.
