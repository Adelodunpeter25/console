//! Extracting a display title from fetched HTML.

use console_core::utils::extract_html_title;

#[test]
fn reads_a_plain_title() {
    let html = "<html><head><title>Console</title></head></html>";
    assert_eq!(extract_html_title(html).as_deref(), Some("Console"));
}

#[test]
fn tag_and_attribute_case_does_not_matter() {
    let html = "<HEAD><TITLE data-x=\"1\">Docs</TITLE></HEAD>";
    assert_eq!(extract_html_title(html).as_deref(), Some("Docs"));
}

#[test]
fn collapses_whitespace_and_decodes_entities() {
    let html = "<title>\n  Tom &amp; Jerry\n  &#8211; &#x41;&quot;s  </title>";
    assert_eq!(extract_html_title(html).as_deref(), Some("Tom & Jerry – A\"s"));
}

#[test]
fn unknown_entities_are_kept_literally() {
    assert_eq!(
        extract_html_title("<title>a &bogus; b</title>").as_deref(),
        Some("a &bogus; b")
    );
}

#[test]
fn missing_or_empty_titles_are_none() {
    assert_eq!(extract_html_title("<html></html>"), None);
    assert_eq!(extract_html_title("<title>   </title>"), None);
    assert_eq!(extract_html_title("<title>unterminated"), None);
}

#[test]
fn long_titles_are_truncated_with_an_ellipsis() {
    let html = format!("<title>{}</title>", "word ".repeat(40));
    let title = extract_html_title(&html).expect("title");
    assert!(title.chars().count() <= 90);
    assert!(title.ends_with('…'));
}
