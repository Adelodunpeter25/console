/// <reference types="vite/client" />

// Console Desktop — frontend entry point (scaffold, no UI yet).
//
// The desktop app reuses shared types from @console/types (declared as a
// dependency in package.json) and communicates with the running Console
// agent server (default: http://localhost:3000) through Tauri v2 Rust
// commands defined in src-tauri/src/commands/.
//
// Available Tauri commands (invoke via @tauri-apps/api/core invoke()):
//   ping_server, get_backend_url, set_backend_url
//   get_auth_status, get_login_url, handle_oauth_callback
//   list_sessions, create_session, get_session, update_session, delete_session
//   list_projects, add_project
//   list_providers, get_provider_models
//   browse_directory, pick_folder, get_directory_tree,
//   read_file, write_file, delete_file, create_directory, delete_directory
//   run_agent, abort_run
//
// SSE events are emitted via the "agent-event" Tauri event.

const app = document.getElementById("app");
if (app) {
  app.innerHTML =
    '<div style="font-family:monospace;padding:2rem;color:#888">' +
    "Console Desktop — scaffold ready. UI not yet implemented." +
    "</div>";
}
