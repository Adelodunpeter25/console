//! Display formatting for model metadata, identical on both clients.

/// Human-size context window: 1_000_000 -> "1M", 272_000 -> "272k".
pub fn format_context_window(context_window: u32) -> String {
    if context_window >= 1_000_000 {
        format!("{}M", context_window / 1_000_000)
    } else {
        format!("{}k", context_window / 1000)
    }
}

/// "claude-sonnet-4_5" -> "Claude Sonnet 4 5": split on `-`/`_`, capitalise
/// the first letter of each part.
pub fn format_model_name(model_id: &str) -> String {
    model_id
        .split(['-', '_'])
        .map(|part| {
            let mut chars = part.chars();
            match chars.next() {
                Some(first) => format!("{}{}", first.to_uppercase(), chars.as_str()),
                None => String::new(),
            }
        })
        .collect::<Vec<_>>()
        .join(" ")
}
