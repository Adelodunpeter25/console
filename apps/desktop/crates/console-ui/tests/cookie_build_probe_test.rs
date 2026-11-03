//! Isolated probe: build the same property dictionary `cookies::build_cookie`
//! assembles and call `NSHTTPCookie::cookieWithProperties:` directly, to see
//! whether the Rust-side dictionary construction is what WebKit rejects.
//! Runs as one #[test] (ignore:objc probe) — no GPUI, Foundation only.

use objc2::rc::Retained;
use objc2::runtime::AnyObject;
use objc2_foundation::{
    ns_string, NSDate, NSDictionary, NSHTTPCookie, NSNumber, NSString,
};

/// Deref-coerce typed Foundation objects into the dictionary's `AnyObject`.
unsafe fn into_any<T: objc2::ClassType>(value: Retained<T>) -> Retained<AnyObject> {
    unsafe { Retained::cast_unchecked::<AnyObject>(value) }
}

#[test]
fn cookie_with_properties_probe() {
    unsafe {
        let mut keys: Vec<&NSString> = vec![
            ns_string!("NSHTTPCookieName"),
            ns_string!("NSHTTPCookieValue"),
            ns_string!("NSHTTPCookieDomain"),
            ns_string!("NSHTTPCookiePath"),
        ];
        let name = NSString::from_str("probe");
        let value = NSString::from_str("1");
        let domain = NSString::from_str(".cookie-probe-rust-77.com");
        let path = NSString::from_str("/");
        let mut objects: Vec<Retained<AnyObject>> = vec![
            into_any(name.clone()),
            into_any(value.clone()),
            into_any(domain.clone()),
            into_any(path.clone()),
        ];
        // Expiry: the piece every failing cookie carries.
        let date = NSDate::dateWithTimeIntervalSince1970(1_900_000_000.0);
        let date_any: Retained<AnyObject> = unsafe { into_any(date) };
        keys.push(ns_string!("NSHTTPCookieExpires"));
        objects.push(date_any);

        let ref_rows: Vec<&AnyObject> = objects.iter().map(|o| &**o).collect();
        let properties =
            NSDictionary::<NSString, AnyObject>::from_slices(&keys, &ref_rows);
        println!("dictionary count = {}", properties.count());

        let built = NSHTTPCookie::cookieWithProperties(&properties);
        match built {
            Some(cookie) => println!(
                "cookieWithProperties -> Some: name={} domain={} expires={:?}",
                cookie.name().to_string(),
                cookie.domain().to_string(),
                cookie.expiresDate().map(|d| d.timeIntervalSince1970() as i64),
            ),
            None => println!("cookieWithProperties -> None (property dictionary rejected)"),
        }

        // Same but WITHOUT the expires entry — the control the Swift probe
        // effectively ran.
        let mut ctl_keys = keys;
        ctl_keys.pop();
        let mut ctl_objects = objects;
        let _ = ctl_objects.pop();
        let ctl_refs: Vec<&AnyObject> =
            ctl_objects.iter().map(|o| &**o).collect();
        let control = NSDictionary::<NSString, AnyObject>::from_slices(&ctl_keys, &ctl_refs);
        println!("control dictionary count = {}", control.count());
        let built = NSHTTPCookie::cookieWithProperties(&control);
        match built {
            Some(cookie) => println!(
                "control cookieWithProperties -> Some: name={} domain={}",
                cookie.name().to_string(),
                cookie.domain().to_string()
            ),
            None => println!("control cookieWithProperties -> None"),
        }

        // Read one key back out of the dictionary: proves whether the dict
        // records keys the way Foundation's property lookup expects.
        let looked = properties.objectForKey(ns_string!("NSHTTPCookieName"));
        println!("objectForKey(NSHTTPCookieName) present = {}", looked.is_some());
        // Dump the dictionary's description: shows exactly what Foundation
        // sees as keys/values (including any invisible bytes).
        println!("properties description: {}", properties.description().to_string());
        let name_value = properties
            .objectForKey(ns_string!("NSHTTPCookieName"))
            .map(|a| unsafe { Retained::<AnyObject>::cast::<NSString>(a) });
        if let Some(n) = name_value {
            let s: &NSString = n.as_ref();
            println!("stored name length = {} content = {:?}",
                s.length(), s.to_string());
        }

        // Same construction with a NSMutableDictionary, the way native code
        // usually builds these. Rebuild fresh keys/values so the previous
        // dictionary still owns its parts.
        let ctl_keys2 = [
            ns_string!("NSHTTPCookieName"),
            ns_string!("NSHTTPCookieValue"),
            ns_string!("NSHTTPCookieDomain"),
            ns_string!("NSHTTPCookiePath"),
        ];
        let mut ctl2_objects: Vec<Retained<AnyObject>> = vec![
            into_any(NSString::from_str("probe")),
            into_any(NSString::from_str("1")),
            into_any(NSString::from_str(".cookie-probe-rust-77.com")),
            into_any(NSString::from_str("/")),
        ];
        let ctl2_refs: Vec<&AnyObject> = ctl2_objects.iter().map(|o| &**o).collect();
        let mutable = objc2_foundation::NSMutableDictionary::<NSString, AnyObject>::from_slices(
            &ctl_keys2, &ctl2_refs,
        );
        let _ = &mutable;
        match NSHTTPCookie::cookieWithProperties(&mutable) {
            Some(cookie) => println!(
                "mutable-dict cookieWithProperties -> Some: name={}",
                cookie.name().to_string()
            ),
            None => println!("mutable-dict cookieWithProperties -> None"),
        }
        let _ = NSNumber::numberWithBool(true);

        // Keys built via from_str (no trailing null a runtime bridge might
        // bake into ns_string! constants) — the suspects behind
        // "required key missing".
        let fs_keys = [
            NSString::from_str("NSHTTPCookieName"),
            NSString::from_str("NSHTTPCookieValue"),
            NSString::from_str("NSHTTPCookieDomain"),
            NSString::from_str("NSHTTPCookiePath"),
        ];
        let fs_objects: Vec<Retained<AnyObject>> = vec![
            into_any(name.clone()),
            into_any(value.clone()),
            into_any(domain.clone()),
            into_any(path.clone()),
        ];
        let fs_key_refs = fs_keys.iter().map(|k| &**k).collect::<Vec<&NSString>>();
        let fs_refs: Vec<&AnyObject> = fs_objects.iter().map(|o| &**o).collect();
        let fs_dict = NSDictionary::<NSString, AnyObject>::from_slices(&fs_key_refs, &fs_refs);
        match NSHTTPCookie::cookieWithProperties(&fs_dict) {
            Some(cookie) => println!(
                "from_str-keys cookieWithProperties -> Some: name={} domain={}",
                cookie.name().to_string(),
                cookie.domain().to_string()
            ),
            None => println!("from_str-keys cookieWithProperties -> None"),
        }

        // The real framework-exported property-key constants — the Rust
        // equivalent of Swift's `.name` etc., which is what cookieWithProperties
        // matches on (pointer identity, not string content).
        let const_keys = [
            objc2_foundation::NSHTTPCookieName,
            objc2_foundation::NSHTTPCookieValue,
            objc2_foundation::NSHTTPCookieDomain,
            objc2_foundation::NSHTTPCookiePath,
        ];
        let const_refs: Vec<&AnyObject> = fs_objects.iter().map(|o| &**o).collect();
        let const_dict = NSDictionary::<NSString, AnyObject>::from_slices(&const_keys, &const_refs);
        match NSHTTPCookie::cookieWithProperties(&const_dict) {
            Some(cookie) => println!(
                "framework-constants cookieWithProperties -> Some: name={} domain={} path={}",
                cookie.name().to_string(),
                cookie.domain().to_string(),
                cookie.path().to_string()
            ),
            None => println!("framework-constants cookieWithProperties -> None"),
        }
        // Show whether the ns_string! keys carry a trailing NUL byte.
        let ns_key = ns_string!("NSHTTPCookieName");
        println!("ns_string key length = {} (from_str length = {})",
            ns_key.length(), "NSHTTPCookieName".len());
    }
}
