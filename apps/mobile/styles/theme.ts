export const theme = {
  colors: {
    background: "#0d0d0e",
    backgroundAlt: "#0a0a0b",
    surface: "#16171a",
    surfaceElevated: "#1f2024",
    border: "rgba(255, 255, 255, 0.1)",
    borderSubtle: "rgba(255, 255, 255, 0.05)",
    
    // Text colors
    text: {
      primary: "#ffffff",
      secondary: "#a1a1aa",
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
