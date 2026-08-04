#![allow(dead_code)]

mod api;
mod commands;
mod config;
mod error;
mod models;

use commands::{
    abort_run, add_project, answer_question, approve_permission, browse_directory,
    create_directory, create_session, delete_directory, delete_file, delete_session,
    get_approval_modes, get_auth_status, get_backend_url, get_directory_tree, get_login_url,
    get_provider_models, get_session, handle_oauth_callback, list_projects, list_providers,
    list_sessions, list_slash_commands, pick_folder, pick_images, ping_server, read_file,
    run_agent, search_files, set_backend_url, update_session, write_file,
};

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_store::Builder::default().build())
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
            get_approval_modes,
            browse_directory,
            pick_folder,
            pick_images,
            get_directory_tree,
            read_file,
            write_file,
            delete_file,
            create_directory,
            delete_directory,
            list_slash_commands,
            search_files,
            run_agent,
            abort_run,
            answer_question,
            approve_permission,
        ])
        .setup(|app| {
            // Load the persisted backend URL before the frontend initialises
            // so `get_backend_url` returns the saved value on first read.
            crate::config::load_config(app.handle());

            #[cfg(debug_assertions)]
            {
                let _ = app.handle().plugin(tauri_plugin_log::Builder::new().build());
            }
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
