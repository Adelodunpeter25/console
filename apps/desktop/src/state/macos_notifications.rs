//! Native macOS notifications via `UNUserNotificationCenter` (objc2).
//!
//! Replaces the old `osascript` path. Gives us what AppleScript never could:
//! - per-session identifiers so repeat events **replace** instead of stacking
//! - `removeDeliveredNotifications` so banners clear on focus / session open
//! - click-to-session via the notification center delegate (`userInfo.sessionId`)
//! - native sound + grouping via thread identifiers
//!
//! macOS-only. Non-mac callers link against the stub functions below.

#[cfg(target_os = "macos")]
mod imp {
    use block2::StackBlock;
    use objc2::{define_class, ClassType};
    use objc2::runtime::{Bool, NSObject, NSObjectProtocol};
    use objc2_foundation::{NSArray, NSDictionary, NSError, NSString};
    use objc2_user_notifications::{
        UNMutableNotificationContent, UNNotificationRequest, UNNotificationResponse,
        UNNotificationSound, UNAuthorizationOptions, UNUserNotificationCenter,
        UNUserNotificationCenterDelegate,
    };
    use std::sync::OnceLock;
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
                    if let Some(tx) = CLICK_TX.get() {
                        let _ = tx.send(session_id.clone());
                    }
                    // Drop the delivered banner for the opened session.
                    let center = UNUserNotificationCenter::currentNotificationCenter();
                    let ident = notification_ident(&session_id);
                    let ns_ident = NSString::from_str(&ident);
                    let ids = NSArray::from_slice(&[&*ns_ident]);
                    center.removeDeliveredNotificationsWithIdentifiers(&ids);
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
                completion_handler.call((
                    UNNotificationPresentationOptions::Banner
                        | UNNotificationPresentationOptions::List
                        | UNNotificationPresentationOptions::Sound,
                ));
            }
        }
    );

    static DELEGATE: OnceLock<objc2::rc::Retained<ClickDelegate>> = OnceLock::new();
    static CLICK_TX: OnceLock<UnboundedSender<String>> = OnceLock::new();

    pub(crate) fn notification_ident(session_id: &str) -> String {
        format!("console-session-{session_id}")
    }

    fn session_id_from_response(response: &UNNotificationResponse) -> Option<String> {
        // Primary: identifier encodes the session (`console-session-<id>`).
        let ident = response.notification().request().identifier().to_string();
        if let Some(sid) = ident.strip_prefix("console-session-") {
            if !sid.is_empty() {
                return Some(sid.to_string());
            }
        }
        // Fallback: userInfo.sessionId.
        let info = response.notification().request().content().userInfo();
        let key = NSString::from_str("sessionId");
        // userInfo is NSDictionary<NSString, AnyObject>-shaped; read via objectForKey.
        let obj: Option<objc2::rc::Retained<NSObject>> = unsafe {
            let dict: &NSDictionary<NSString, NSObject> = &*(&*info as *const _ as *const NSDictionary<NSString, NSObject>);
            dict.objectForKey(&key)
        };
        obj.map(|o| {
            let s: &NSString = unsafe { &*(&*o as *const _ as *const NSString) };
            s.to_string()
        })
    }

    /// Install the delegate + request authorization (once). Returns the click receiver.
    pub(crate) fn ensure_initialized() -> UnboundedReceiver<String> {
        if let Some(tx) = CLICK_TX.get() {
            let _ = tx;
            // Already initialized — hand out a fresh receiver wired to the same sender?
            // Only called once at startup; create a throwaway pair to keep types simple.
            let (_tx2, rx) = tokio::sync::mpsc::unbounded_channel();
            return rx;
        }
        let (tx, rx) = tokio::sync::mpsc::unbounded_channel::<String>();
        let _ = CLICK_TX.set(tx);

        let center = UNUserNotificationCenter::currentNotificationCenter();
        let delegate: objc2::rc::Retained<ClickDelegate> =
            unsafe { objc2::msg_send![ClickDelegate::class(), new] };
        center.setDelegate(Some(objc2::runtime::ProtocolObject::from_ref(&*delegate)));
        let _ = DELEGATE.set(delegate);

        let options =
            UNAuthorizationOptions::Alert | UNAuthorizationOptions::Badge | UNAuthorizationOptions::Sound;
        let block = StackBlock::new(move |granted: Bool, _err: *mut NSError| {
            if !granted.as_bool() {
                log::warn!("macOS notifications not authorized; banners will not appear");
            }
        });
        center.requestAuthorizationWithOptions_completionHandler(options, &block);
        rx
    }

    pub(crate) fn notify_session(session_id: &str, title: &str, body: &str) {
        let center = UNUserNotificationCenter::currentNotificationCenter();
        let content = UNMutableNotificationContent::new();
        content.setTitle(&NSString::from_str(title));
        content.setBody(&NSString::from_str(body));
        content.setSound(Some(&UNNotificationSound::defaultSound()));
        content.setThreadIdentifier(&NSString::from_str("console-sessions"));
        let key = NSString::from_str("sessionId");
        let val = NSString::from_str(session_id);
        let info: objc2::rc::Retained<NSDictionary<NSString, NSString>> =
            NSDictionary::from_slices(&[&*key], &[&*val]);
        unsafe {
            let dict: &objc2_foundation::NSDictionary = &*(&*info as *const _ as *const objc2_foundation::NSDictionary);
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

    pub(crate) fn clear_for_session(session_id: &str) {
        let center = UNUserNotificationCenter::currentNotificationCenter();
        let ns_ident = NSString::from_str(&notification_ident(session_id));
        let ids = NSArray::from_slice(&[&*ns_ident]);
        center.removeDeliveredNotificationsWithIdentifiers(&ids);
        center.removePendingNotificationRequestsWithIdentifiers(&ids);
    }

}


#[cfg(target_os = "macos")]
pub(crate) use imp::{clear_for_session, ensure_initialized, notify_session};

#[cfg(not(target_os = "macos"))]
mod stub {
    use tokio::sync::mpsc::UnboundedReceiver;
    pub(crate) fn ensure_initialized() -> UnboundedReceiver<String> {
        let (_tx, rx) = tokio::sync::mpsc::unbounded_channel();
        rx
    }
    pub(crate) fn notify_session(_session_id: &str, _title: &str, _body: &str) {}
    pub(crate) fn clear_for_session(_session_id: &str) {}
}

#[cfg(not(target_os = "macos"))]
pub(crate) use stub::{clear_for_session, ensure_initialized, notify_session};
