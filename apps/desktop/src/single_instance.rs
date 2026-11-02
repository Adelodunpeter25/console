//! Windows single-instance guard.
//!
//! Console is a thin client, so a second copy is never useful: the copies
//! fight over the OAuth loopback port and multiply GPUI teardown noise. The
//! first process keeps a per-session named mutex for its lifetime. A later
//! launch finds the existing window, restores it, and exits before Tokio or
//! GPUI start.
//!
//! Builds from before this guard do not create the mutex, so winning it is
//! not enough. The winner also looks for an existing `Console` window using
//! GPUI's window class. If the owning process is `console.exe`, that is an
//! older install; it is focused and the user is told to close or uninstall it
//! instead of silently starting a second process tree.

#[cfg(windows)]
use windows_sys::Win32::Foundation::HANDLE;

/// Keeps the single-instance mutex alive for the process lifetime on Windows.
pub struct InstanceGuard {
    #[cfg(windows)]
    handle: HANDLE,
}

impl Drop for InstanceGuard {
    fn drop(&mut self) {
        #[cfg(windows)]
        unsafe {
            if !self.handle.is_null() {
                windows_sys::Win32::Foundation::CloseHandle(self.handle);
            }
        }
    }
}

/// Returns `Some(guard)` for the first instance. `None` means another Console
/// is already running; the caller should return from `main`.
pub fn acquire() -> Option<InstanceGuard> {
    #[cfg(windows)]
    {
        imp::acquire()
    }

    #[cfg(not(windows))]
    {
        Some(InstanceGuard {})
    }
}

#[cfg(windows)]
mod imp {
    use super::InstanceGuard;
    use std::time::{Duration, Instant};
    use windows_sys::Win32::Foundation::{CloseHandle, ERROR_ALREADY_EXISTS, GetLastError, HWND};
    use windows_sys::Win32::System::Threading::{
        CreateMutexW, OpenProcess, PROCESS_NAME_WIN32, PROCESS_QUERY_LIMITED_INFORMATION,
        QueryFullProcessImageNameW,
    };
    use windows_sys::Win32::UI::WindowsAndMessaging::{
        FindWindowW, GetWindowThreadProcessId, MB_ICONINFORMATION, MB_OK, MB_SETFOREGROUND,
        MESSAGEBOX_RESULT, MessageBoxW, SW_RESTORE, SetForegroundWindow, ShowWindow,
    };

    const MUTEX_NAME: &str = "Local\\Console.Desktop.SingleInstance";
    // GPUI's registered window class (gpui_windows::window::WINDOW_CLASS_NAME).
    // The title is the one main_window_title() uses on Windows.
    const WINDOW_CLASS: &str = "Zed::Window";
    const WINDOW_TITLE: &str = "Console";

    fn wide(value: &str) -> Vec<u16> {
        value.encode_utf16().chain(std::iter::once(0)).collect()
    }

    pub fn acquire() -> Option<InstanceGuard> {
        unsafe {
            let mutex_name = wide(MUTEX_NAME);
            let handle = CreateMutexW(std::ptr::null(), 0, mutex_name.as_ptr());
            if handle.is_null() {
                log::warn!(
                    "Could not create the single-instance mutex ({}); continuing without it",
                    std::io::Error::last_os_error()
                );
                return Some(InstanceGuard { handle });
            }

            if GetLastError() == ERROR_ALREADY_EXISTS {
                CloseHandle(handle);
                // A build with this guard owns the mutex. It may still be
                // creating its window, so give it a moment to appear.
                if let Some(window) = find_console_window(Duration::from_secs(2)) {
                    focus(window);
                }
                return None;
            }

            // We won the mutex, so no modern build is running. An older build
            // has no mutex but does create the same window class/title.
            // Confirm the owner is console.exe: Zed itself uses "Zed::Window",
            // and a Zed project named Console would otherwise match.
            if let Some(window) = find_console_window(Duration::from_millis(750)) {
                let path = window_process_path(window);
                if path.as_deref().is_some_and(is_console_exe) {
                    focus(window);
                    show_old_instance_message(path.as_deref());
                    CloseHandle(handle);
                    return None;
                }
            }

            Some(InstanceGuard { handle })
        }
    }

    fn is_console_exe(path: &str) -> bool {
        std::path::Path::new(path)
            .file_name()
            .and_then(|name| name.to_str())
            .is_some_and(|name| name.eq_ignore_ascii_case("console.exe"))
    }

    fn find_console_window(timeout: Duration) -> Option<HWND> {
        let class = wide(WINDOW_CLASS);
        let title = wide(WINDOW_TITLE);
        let deadline = Instant::now() + timeout;
        loop {
            let window = unsafe { FindWindowW(class.as_ptr(), title.as_ptr()) };
            if !window.is_null() {
                return Some(window);
            }
            if Instant::now() >= deadline {
                return None;
            }
            std::thread::sleep(Duration::from_millis(100));
        }
    }

    fn focus(window: HWND) {
        unsafe {
            ShowWindow(window, SW_RESTORE);
            SetForegroundWindow(window);
        }
    }

    fn window_process_path(window: HWND) -> Option<String> {
        unsafe {
            let mut pid = 0u32;
            GetWindowThreadProcessId(window, &mut pid);
            if pid == 0 {
                return None;
            }
            let process = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, 0, pid);
            if process.is_null() {
                return None;
            }
            let mut buffer = [0u16; 1024];
            let mut length = buffer.len() as u32;
            let ok = QueryFullProcessImageNameW(
                process,
                PROCESS_NAME_WIN32,
                buffer.as_mut_ptr(),
                &mut length,
            );
            CloseHandle(process);
            if ok == 0 {
                return None;
            }
            Some(String::from_utf16_lossy(&buffer[..length as usize]))
        }
    }

    fn show_old_instance_message(path: Option<&str>) {
        let mut text = String::from("Console is already running");
        match path {
            Some(path) => {
                text.push_str(" from:\n\n");
                text.push_str(path);
                text.push_str("\n\n");
            }
            None => text.push_str(".\n\n"),
        }
        text.push_str(
            "Close that window before launching this version. If it is an older \
             per-machine install, uninstall it from Settings > Apps first.",
        );

        let text = wide(&text);
        let caption = wide("Console");
        let _: MESSAGEBOX_RESULT = unsafe {
            MessageBoxW(
                std::ptr::null_mut(),
                text.as_ptr(),
                caption.as_ptr(),
                MB_OK | MB_ICONINFORMATION | MB_SETFOREGROUND,
            )
        };
    }
}
