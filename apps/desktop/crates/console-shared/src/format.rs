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

/// Simplify a CSS selector for display in UI chips and headers.
///
/// If the selector is a compound chain (e.g. `main.class > article > div.cell.pad:nth-of-type(2)`),
/// this extracts the target leaf segment, trimming excessive class lists to keep chips readable.
/// Empty input returns an empty string.
pub fn format_selector_label(selector: &str) -> String {
    let trimmed = selector.trim();
    if trimmed.is_empty() {
        return String::new();
    }

    // Extract the final segment of a descendant / child combinator chain.
    let leaf = trimmed
        .rsplit_once(['>', '+', '~', ' '])
        .map(|(_, last)| last.trim())
        .unwrap_or(trimmed);

    // If the leaf has an ID (e.g. #my-id), that alone is very informative.
    if leaf.starts_with('#') {
        return leaf.to_string();
    }

    // A leaf may look like tag.class1.class2:pseudo or .class1.class2.
    // Keep tag + at most the first class and pseudo-class.
    let (base_and_classes, pseudo) = match leaf.split_once(':') {
        Some((b, p)) => (b, Some(p)),
        None => (leaf, None),
    };

    let mut parts = base_and_classes.split('.').filter(|s| !s.is_empty());
    let mut out = String::new();

    if !base_and_classes.starts_with('.') {
        // Starts with tag name
        if let Some(tag) = parts.next() {
            out.push_str(tag);
        }
    }

    // Include at most the first class to keep the label short
    if let Some(first_class) = parts.next() {
        out.push('.');
        out.push_str(first_class);
    }

    if let Some(pseudo) = pseudo {
        out.push(':');
        out.push_str(pseudo);
    }

    if out.is_empty() {
        leaf.to_string()
    } else {
        out
    }
}
