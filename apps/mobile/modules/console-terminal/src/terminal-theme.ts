export interface TerminalTheme {
  readonly background: string;
  readonly foreground: string;
  readonly mutedForeground: string;
  readonly border: string;
  readonly cursorForeground: string;
  readonly cursorBackground: string;
  readonly palette: readonly string[];
}

/**
 * Console's dark-only terminal theme. Colors mirror the tokens in
 * `apps/mobile/global.css`; the 16-color palette is the Pierre dark palette
 * carried over from the upstream t3-terminal module.
 */
export const CONSOLE_TERMINAL_THEME: TerminalTheme = {
  background: "#0a0a0b", // --color-screen
  foreground: "#ffffff", // --color-foreground
  mutedForeground: "#71717a", // --color-foreground-muted
  border: "rgba(255, 255, 255, 0.12)", // --color-border (RN styles only)
  cursorForeground: "#009fff",
  cursorBackground: "#0a0a0b",
  palette: [
    "#141415",
    "#ff2e3f",
    "#0dbe4e",
    "#ffca00",
    "#009fff",
    "#c635e4",
    "#08c0ef",
    "#c6c6c8",
    "#141415",
    "#ff2e3f",
    "#0dbe4e",
    "#ffca00",
    "#009fff",
    "#c635e4",
    "#08c0ef",
    "#c6c6c8",
  ],
};

/** Serialize a theme into ghostty's config format (newline-joined `key = value`). */
export function buildGhosttyThemeConfig(theme: TerminalTheme): string {
  const lines = [
    `background = ${theme.background}`,
    `foreground = ${theme.foreground}`,
    `cursor-color = ${theme.cursorForeground}`,
    `cursor-text = ${theme.cursorBackground}`,
  ];

  for (const [index, color] of theme.palette.entries()) {
    lines.push(`palette = ${index}=${color}`);
  }

  return `${lines.join("\n")}\n`;
}
