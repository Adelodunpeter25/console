mod api;
mod commands;
mod config;
mod error;
mod models;

use commands::{
    abort_run, add_project, browse_directory, create_directory, create_session, delete_directory,
    delete_file, delete_session, get_auth_status, get_backend_url, get_directory_tree,
    get_login_url, get_provider_models, get_session, handle_oauth_callback, list_projects,
    list_providers, list_sessions, pick_folder, ping_server, read_file, run_agent, set_backend_url,
    update_session, write_file,
};

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .invoke_handler(tauri::generate_handler![
            ping_server,
            get_backend_url,
            set_backend_url,
            get_auth_status,
            get_login_url,
            handle_oauth_callback,
            list_sessions,
            create_session,
            get_session,
            update_session,
            delete_session,
            list_projects,
            add_project,
            list_providers,
            get_provider_models,
            browse_directory,
            pick_folder,
            get_directory_tree,
            read_file,
            write_file,
            delete_file,
            create_directory,
            delete_directory,
            run_agent,
            abort_run,
        ])
        .setup(|_app| {
            #[cfg(debug_assertions)]
            {
                let _ = _app.handle().plugin(tauri_plugin_log::Builder::new().build());
            }
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
