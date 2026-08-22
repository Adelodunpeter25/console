/**
 * JS mirror of the design tokens in ../global.css (@theme block).
 * global.css is the single source of truth — when changing a color,
 * update BOTH files (or better, add a CSS token and consume it via
 * className utilities). Values here must never diverge from it.
 */
export const theme = {
  colors: {
    /** --color-screen */
    background: "#0a0a0b",
    /** --color-screen (alias kept for existing call sites) */
    backgroundAlt: "#0a0a0b",
    /** --color-surface */
    surface: "#16171a",
    /** --color-surface-elevated */
    surfaceElevated: "#1f2024",
    /** --color-border */
    border: "rgba(255, 255, 255, 0.12)",
    /** --color-border-subtle */
    borderSubtle: "rgba(255, 255, 255, 0.06)",

    // Text colors
    text: {
      /** --color-foreground */
      primary: "#ffffff",
      /** --color-foreground-secondary */
      secondary: "#a1a1aa",
      /** --color-foreground-muted */
      muted: "#71717a",
      dark: "#000000",
    },

    // Status colors
    status: {
      running: "#fb923c", // orange-400
      runningBg: "rgba(251, 146, 60, 0.1)",
      ready: "#34d399", // emerald-400
      readyBg: "rgba(52, 211, 153, 0.1)",
      attention: "#f87171", // red-400
      attentionBg: "rgba(248, 113, 113, 0.1)",
      idle: "#a1a1aa",
      idleBg: "rgba(161, 161, 170, 0.1)",
    },

    // Destructive — matches --color-destructive
    danger: "#f87171",
    dangerPressed: "#dc2626",
  },

  fonts: {
    mono: "JetBrainsMono",
    monoMedium: "JetBrainsMono-Medium",
    monoSemiBold: "JetBrainsMono-SemiBold",
    monoBold: "JetBrainsMono-Bold",
  },

  roundness: {
    sm: 8,
    md: 12,
    lg: 16,
    full: 9999,
  },

  spacing: {
    xs: 4,
    sm: 8,
    md: 12,
    lg: 16,
    xl: 24,
    xxl: 32,
  },
};
