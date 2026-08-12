use crate::error::AppResult;

/// Open a native platform message box confirming an action (Electron `showMessageBox` equivalent).
#[tauri::command]
pub async fn confirm_dialog(
    title: String,
    message: String,
) -> AppResult<bool> {
    let dialog = rfd::AsyncMessageDialog::new()
        .set_title(&title)
        .set_description(&message)
        .set_buttons(rfd::MessageButtons::OkCancel);
    let result = dialog.show().await;
    Ok(matches!(
        result,
        rfd::MessageDialogResult::Ok | rfd::MessageDialogResult::Yes
    ))
}
