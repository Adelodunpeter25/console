//! Settings window sections, one module per tab. `SettingsWindow` keeps a
//! tab-state struct per section and delegates rendering to it, so per-tab
//! flows (like the MCP connect sequence) live next to their state instead
//! of inside one giant render method.
pub mod mcp;
