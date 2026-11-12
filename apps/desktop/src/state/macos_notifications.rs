//! Native macOS notifications via `UNUserNotificationCenter` (objc2).
//!
//! Replaces the old `osascript` path. Gives us what AppleScript never could:
//! - per-session identifiers so repeat events **replace** instead of stacking
//! - `removeDeliveredNotifications` so banners clear on focus / session open
//! - click-to-session via the notification center delegate (`userInfo.sessionId`)
//! - native sound + grouping via thread identifiers
//! - Dock tile badge counting unread and needs-attention chats, set from the
//!   session headers so it survives restarts and matches the sidebar
//!
//! macOS-only. Non-mac callers link against the stub functions below.

#[cfg(target_os = "macos")]
mod imp {
    use block2::StackBlock;
    use console_core::types::notification::{
        NOTIFICATION_THREAD_ID, dock_badge_label, notification_ident, parse_session_id_from_ident,
    };
    use objc2::runtime::{Bool, NSObject, NSObjectProtocol};
    use objc2::{ClassType, define_class};
    use objc2_app_kit::NSApplication;
    use objc2_foundation::{MainThreadMarker, NSArray, NSDictionary, NSError, NSString};
    use objc2_user_notifications::{
        UNAuthorizationOptions, UNMutableNotificationContent, UNNotificationRequest,
        UNNotificationResponse, UNNotificationSound, UNUserNotificationCenter,
        UNUserNotificationCenterDelegate,
    };
    use std::sync::{Mutex, OnceLock};
    use tokio::sync::mpsc::{UnboundedReceiver, UnboundedSender};

    define_class!(
        #[unsafe(super(NSObject))]
        #[name = "ConsoleNotificationDelegate"]
        struct ClickDelegate;

        unsafe impl NSObjectProtocol for ClickDelegate {}

        unsafe impl UNUserNotificationCenterDelegate for ClickDelegate {
            #[unsafe(method(userNotificationCenter:didReceiveNotificationResponse:withCompletionHandler:))]
            unsafe fn user_notification_center_did_receive_response(
                &self,
                _center: &UNUserNotificationCenter,
                response: &UNNotificationResponse,
                completion_handler: &block2::DynBlock<dyn Fn()>,
            ) {
                if let Some(session_id) = session_id_from_response(response) {
                    if let Some(txs) = CLICK_TXS.get() {
                        let mut guard = txs.lock().unwrap_or_else(|e| e.into_inner());
                        // Drop dead receivers so repeat inits don't leak senders.
                        guard.retain(|tx| tx.send(session_id.clone()).is_ok());
                    }
                    // Same path as opening the session in-app: drop the banner
                    // and its dock badge count.
                    clear_for_session(&session_id);
                }
                completion_handler.call(());
            }

            #[unsafe(method(userNotificationCenter:willPresentNotification:withCompletionHandler:))]
            unsafe fn user_notification_center_will_present(
                &self,
                _center: &UNUserNotificationCenter,
                _notification: &objc2_user_notifications::UNNotification,
                completion_handler: &block2::DynBlock<
                    dyn Fn(objc2_user_notifications::UNNotificationPresentationOptions),
                >,
            ) {
                use objc2_user_notifications::UNNotificationPresentationOptions;
                completion_handler.call((UNNotificationPresentationOptions::Banner
                    | UNNotificationPresentationOptions::List
                    | UNNotificationPresentationOptions::Sound,));
            }
        }
    );

    static DELEGATE: OnceLock<objc2::rc::Retained<ClickDelegate>> = OnceLock::new();
    static CLICK_TXS: OnceLock<Mutex<Vec<UnboundedSender<String>>>> = OnceLock::new();
    /// Show `count` on the Dock tile (nothing when zero). The count comes from
    /// the session headers (`console_core::types::notification::badge_count`),
    /// not from which banners were posted, so it survives a restart and agrees
    /// with the sidebar. AppKit requires the main thread; off-main callers skip
    /// silently (every caller runs inside `cx.update` or the center delegate,
    /// both main-thread).
    pub(crate) fn set_dock_badge(count: usize) {
        let Some(mtm) = MainThreadMarker::new() else {
            log::debug!("Skipping dock badge update off the main thread");
            return;
        };
        let tile = NSApplication::sharedApplication(mtm).dockTile();
        match dock_badge_label(count) {
            Some(label) => tile.setBadgeLabel(Some(&NSString::from_str(&label))),
            None => tile.setBadgeLabel(None),
        }
    }

    fn session_id_from_response(response: &UNNotificationResponse) -> Option<String> {
        // Primary: identifier encodes the session (`console-session-<id>`).
        let ident = response.notification().request().identifier().to_string();
        if let Some(sid) = parse_session_id_from_ident(&ident) {
            return Some(sid);
        }
        // Fallback: userInfo.sessionId.
        let info = response.notification().request().content().userInfo();
        let key = NSString::from_str("sessionId");
        // userInfo is NSDictionary<NSString, AnyObject>-shaped; read via objectForKey.
        let obj: Option<objc2::rc::Retained<NSObject>> = unsafe {
            let dict: &NSDictionary<NSString, NSObject> =
                &*(&*info as *const _ as *const NSDictionary<NSString, NSObject>);
            dict.objectForKey(&key)
        };
        obj.map(|o| {
            let s: &NSString = unsafe { &*(&*o as *const _ as *const NSString) };
            s.to_string()
        })
    }

    /// Install the delegate + request authorization (once). Each call gets its
    /// own live receiver; clicks fan out to all of them.
    pub(crate) fn ensure_initialized() -> UnboundedReceiver<String> {
        let txs = CLICK_TXS.get_or_init(|| Mutex::new(Vec::new()));
        let (tx, rx) = tokio::sync::mpsc::unbounded_channel::<String>();
        txs.lock().unwrap_or_else(|e| e.into_inner()).push(tx);

        // One-time native setup.
        if DELEGATE.get().is_none() {
            let center = UNUserNotificationCenter::currentNotificationCenter();
            let delegate: objc2::rc::Retained<ClickDelegate> =
                unsafe { objc2::msg_send![ClickDelegate::class(), new] };
            center.setDelegate(Some(objc2::runtime::ProtocolObject::from_ref(&*delegate)));
            let _ = DELEGATE.set(delegate);

            let options = UNAuthorizationOptions::Alert
                | UNAuthorizationOptions::Badge
                | UNAuthorizationOptions::Sound;
            let block = StackBlock::new(move |granted: Bool, _err: *mut NSError| {
                if !granted.as_bool() {
                    log::warn!("macOS notifications not authorized; banners will not appear");
                }
            });
            center.requestAuthorizationWithOptions_completionHandler(options, &block);
        }
        rx
    }

    pub(crate) fn notify_session(session_id: &str, title: &str, subtitle: &str, body: &str) {
        let center = UNUserNotificationCenter::currentNotificationCenter();
        let content = UNMutableNotificationContent::new();
        content.setTitle(&NSString::from_str(title));
        if !subtitle.is_empty() {
            content.setSubtitle(&NSString::from_str(subtitle));
        }
        content.setBody(&NSString::from_str(body));
        content.setSound(Some(&UNNotificationSound::defaultSound()));
        content.setThreadIdentifier(&NSString::from_str(NOTIFICATION_THREAD_ID));
        let key = NSString::from_str("sessionId");
        let val = NSString::from_str(session_id);
        let info: objc2::rc::Retained<NSDictionary<NSString, NSString>> =
            NSDictionary::from_slices(&[&*key], &[&*val]);
        unsafe {
            let dict: &objc2_foundation::NSDictionary =
                &*(&*info as *const _ as *const objc2_foundation::NSDictionary);
            content.setUserInfo(dict);
        }
        let ident = notification_ident(session_id);
        let request = UNNotificationRequest::requestWithIdentifier_content_trigger(
            &NSString::from_str(&ident),
            &content,
            None,
        );
        // Same identifier replaces the previous banner for this session.
        center.addNotificationRequest_withCompletionHandler(&request, None);
    }

    /// Remove a chat's delivered banner. The Dock badge is not touched: it
    /// follows the chat's unread state, which the caller updates separately.
    pub(crate) fn clear_for_session(session_id: &str) {
        let center = UNUserNotificationCenter::currentNotificationCenter();
        let ns_ident = NSString::from_str(&notification_ident(session_id));
        let ids = NSArray::from_slice(&[&*ns_ident]);
        center.removeDeliveredNotificationsWithIdentifiers(&ids);
        center.removePendingNotificationRequestsWithIdentifiers(&ids);
    }

    /// Remove every delivered banner. Like `clear_for_session`, this leaves the
    /// Dock badge alone: focusing the window is not reading the chats.
    pub(crate) fn clear_all() {
        let center = UNUserNotificationCenter::currentNotificationCenter();
        center.removeAllDeliveredNotifications();
        center.removeAllPendingNotificationRequests();
    }
}

#[cfg(target_os = "macos")]
pub(crate) use imp::{
    clear_all, clear_for_session, ensure_initialized, notify_session, set_dock_badge,
};

#[cfg(not(target_os = "macos"))]
mod stub {
    use tokio::sync::mpsc::UnboundedReceiver;
    pub(crate) fn ensure_initialized() -> UnboundedReceiver<String> {
        let (_tx, rx) = tokio::sync::mpsc::unbounded_channel();
        rx
    }
    pub(crate) fn notify_session(_session_id: &str, _title: &str, _subtitle: &str, _body: &str) {}
    pub(crate) fn clear_for_session(_session_id: &str) {}
    pub(crate) fn clear_all() {}
    pub(crate) fn set_dock_badge(_count: usize) {}
}

#[cfg(not(target_os = "macos"))]
pub(crate) use stub::{
    clear_all, clear_for_session, ensure_initialized, notify_session, set_dock_badge,
};
