//! Render models and pure geometry for the v1 visual spec
//! (docs/plan/visualize.md §3). No GPUI types here so everything is
//! unit-testable; `console-ui/src/chat/visual.rs` does the painting.

use serde::Deserialize;

use crate::types::agent::VisualArtifact;

pub const SUPPORTED_VISUAL_VERSION: u32 = 1;

#[derive(Clone, Debug, Deserialize)]
pub struct VisualSpec {
    pub version: u32,
    #[serde(default)]
    pub title: Option<String>,
    #[serde(default)]
    pub subtitle: Option<String>,
    #[serde(default)]
    pub blocks: Vec<VisualBlock>,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(tag = "type", rename_all = "lowercase")]
pub enum VisualBlock {
    Text {
        text: String,
    },
    Metric {
        label: String,
        value: String,
        #[serde(default)]
        delta: Option<String>,
        #[serde(default)]
        trend: Option<String>,
        #[serde(default)]
        caption: Option<String>,
    },
    Table {
        #[serde(default)]
        caption: Option<String>,
        columns: Vec<String>,
        rows: Vec<Vec<String>>,
    },
    Chart(VisualChart),
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct VisualChart {
    pub kind: String,
    #[serde(default)]
    pub title: Option<String>,
    #[serde(default)]
    pub x_label: Option<String>,
    #[serde(default)]
    pub y_label: Option<String>,
    pub series: Vec<VisualSeries>,
}

#[derive(Clone, Debug, Deserialize)]
pub struct VisualSeries {
    pub name: String,
    pub points: Vec<VisualPoint>,
}

#[derive(Clone, Debug, Deserialize)]
pub struct VisualPoint {
    pub x: f64,
    pub y: f64,
    #[serde(default)]
    pub label: Option<String>,
}

#[derive(Clone, Debug)]
pub enum VisualParse {
    Ok(VisualSpec),
    /// A newer spec version than this build understands.
    Unsupported,
    Invalid,
}

/// The version is checked before decoding so a newer spec never half-parses.
pub fn parse_visual(artifact: &VisualArtifact) -> VisualParse {
    if artifact.version != SUPPORTED_VISUAL_VERSION {
        return VisualParse::Unsupported;
    }
    match serde_json::from_value::<VisualSpec>(artifact.spec.clone()) {
        Ok(spec) if spec.version == SUPPORTED_VISUAL_VERSION => VisualParse::Ok(spec),
        Ok(_) => VisualParse::Unsupported,
        Err(_) => VisualParse::Invalid,
    }
}

/// Value axis: `min`/`max` plus the tick values between them.
#[derive(Clone, Debug, PartialEq)]
pub struct Axis {
    pub min: f64,
    pub max: f64,
    pub ticks: Vec<f64>,
}

pub fn nice_step(range: f64, target_ticks: usize) -> f64 {
    let raw = range / target_ticks.max(1) as f64;
    let mag = 10f64.powf(raw.log10().floor());
    let norm = raw / mag;
    let nice = if norm <= 1.0 {
        1.0
    } else if norm <= 2.0 {
        2.0
    } else if norm <= 5.0 {
        5.0
    } else {
        10.0
    };
    nice * mag
}

/// Bars and areas include zero; lines and scatter use the data range with
/// padding. A flat series is expanded so it sits mid-plot.
pub fn value_axis(lo: f64, hi: f64, include_zero: bool, target_ticks: usize) -> Axis {
    let (mut a, mut b) = (lo, hi);
    if include_zero {
        a = a.min(0.0);
        b = b.max(0.0);
    }
    if a == b {
        let pad = if a == 0.0 { 1.0 } else { a.abs() * 0.5 };
        a -= pad;
        b += pad;
    } else if !include_zero {
        let pad = (b - a) * 0.05;
        a -= pad;
        b += pad;
    }
    let step = nice_step(b - a, target_ticks);
    let nice_min = (a / step).floor() * step;
    let nice_max = (b / step).ceil() * step;
    let mut ticks = Vec::new();
    let mut t = nice_min;
    while t <= nice_max + step * 1e-9 && ticks.len() < 40 {
        ticks.push(if t.abs() < step * 1e-9 { 0.0 } else { t });
        t += step;
    }
    Axis {
        min: nice_min,
        max: nice_max,
        ticks,
    }
}

fn trim_decimals(v: f64, decimals: usize) -> String {
    let s = format!("{v:.decimals$}");
    if s.contains('.') {
        s.trim_end_matches('0').trim_end_matches('.').to_string()
    } else {
        s
    }
}

/// Compact tick text: 1.2k, 3.4M; small values keep up to two decimals.
pub fn format_compact(v: f64) -> String {
    let a = v.abs();
    if a >= 1e9 {
        format!("{}B", trim_decimals(v / 1e9, 1))
    } else if a >= 1e6 {
        format!("{}M", trim_decimals(v / 1e6, 1))
    } else if a >= 1e3 {
        format!("{}k", trim_decimals(v / 1e3, 1))
    } else {
        trim_decimals(v, 2)
    }
}

/// Rendering safety net: at most a min and a max per horizontal bucket, so a
/// dense line never draws more segments than pixels. Under the cap the
/// points pass through untouched.
pub fn min_max_buckets(points: &[VisualPoint], buckets: usize) -> Vec<VisualPoint> {
    if buckets < 1 || points.len() <= buckets * 2 {
        return points.to_vec();
    }
    let per = points.len() as f64 / buckets as f64;
    let mut out = Vec::with_capacity(buckets * 2);
    for i in 0..buckets {
        let from = (i as f64 * per) as usize;
        let to = (((i + 1) as f64 * per) as usize).min(points.len()).max(from + 1);
        let (mut lo, mut hi) = (from, from);
        for j in from..to {
            if points[j].y < points[lo].y {
                lo = j;
            }
            if points[j].y > points[hi].y {
                hi = j;
            }
        }
        let (first, second) = if lo <= hi { (lo, hi) } else { (hi, lo) };
        out.push(points[first].clone());
        if second != first {
            out.push(points[second].clone());
        }
    }
    out
}

/// One-line summary shown under a chart (GPUI has no accessibility tree, so
/// this is also the fallback for anyone who cannot read the plot).
pub fn describe_chart(chart: &VisualChart) -> String {
    let all: Vec<&VisualPoint> = chart.series.iter().flat_map(|s| s.points.iter()).collect();
    let kind = {
        let mut c = chart.kind.chars();
        c.next()
            .map(|f| f.to_uppercase().collect::<String>() + c.as_str())
            .unwrap_or_default()
    };
    let title = chart
        .title
        .as_ref()
        .map(|t| format!(", {t}"))
        .unwrap_or_default();
    let per_series = chart.series.iter().map(|s| s.points.len()).max().unwrap_or(0);
    let mut out = format!(
        "{kind} chart{title}. {} {}, {} {}.",
        chart.series.len(),
        if chart.series.len() == 1 { "series" } else { "series" },
        per_series,
        if per_series == 1 { "point" } else { "points" },
    );
    if !all.is_empty() {
        let lo = all.iter().map(|p| p.y).fold(f64::INFINITY, f64::min);
        let hi = all.iter().map(|p| p.y).fold(f64::NEG_INFINITY, f64::max);
        out.push_str(&format!(
            " Range {} to {}.",
            format_compact(lo),
            format_compact(hi)
        ));
        if let Some(last) = chart.series.last().and_then(|s| s.points.last()) {
            out.push_str(&format!(" Last {}.", format_compact(last.y)));
        }
    }
    out
}

/// Heuristic from the plan: numeric-looking cells right-align.
pub fn looks_numeric(cell: &str) -> bool {
    let t = cell
        .trim()
        .trim_start_matches(['+', '-', '$', '€', '£'])
        .trim_end_matches(['%', 'k', 'K', 'M', 'B']);
    !t.is_empty()
        && t.chars().all(|c| c.is_ascii_digit() || c == '.' || c == ',')
        && t.chars().any(|c| c.is_ascii_digit())
}
