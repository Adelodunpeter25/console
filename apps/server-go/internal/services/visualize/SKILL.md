# Visualize

You have the `visualize` tool for this session. Use it when the user asks for a
chart, graph, dashboard, table, or a visual summary of data. The tool renders a
native card in the chat; it does not run code, fetch remote assets, or write
files.

## How to call it

`visualize` takes one object:

```json
{
  "spec": {
    "version": 1,
    "title": "Weekly signups",
    "subtitle": "Last 8 weeks",
    "blocks": []
  }
}
```

Call it once per card. If the user wants several separate visuals, make several
calls. Keep the assistant text brief: the card is the artifact, so summarize the
takeaway rather than repeating every number.

Use only data that is present in the conversation or in files/tools you actually
read. Do not invent values. If the data is incomplete, say what is missing in the
assistant message or in a `text` block.

## Rules

- `version` must be `1`.
- `blocks` must contain 1–12 blocks.
- Every block has a `type`: `text`, `metric`, `table`, or `chart`.
- Use display strings for metrics and table cells (`"$42.1k"`, `"49.8%"`), and
  numeric `x`/`y` for charts.
- Do not use HTML, Markdown, colors, fonts, URLs as images, or absolute layout.
- Do not put credentials, tokens, or other secrets in a visual. Redact them.
- If a call fails validation, read the path in the error, fix that field, and
  call again. Do not guess around the error.
- A chart with many raw rows is usually the wrong visual. Aggregate first
  (daily → weekly, per-row → per-category) so the card stays readable.

## Text block

```json
{ "type": "text", "text": "Organic remains the largest channel." }
```

- 1–1,000 characters. Plain text only; it is not rendered as Markdown.

## Metric block

```json
{
  "type": "metric",
  "label": "MRR",
  "value": "$42.1k",
  "delta": "+8.2%",
  "trend": "up",
  "caption": "vs. last month"
}
```

- `label` and `value` are required.
- `trend` is `up`, `down`, or `flat`; omit it if there is no direction.
- Use metrics for the one or two numbers the user should see first, not for
  every row of a table.

## Table block

```json
{
  "type": "table",
  "caption": "Top channels",
  "columns": ["Channel", "Users", "Share"],
  "rows": [
    ["Organic", "640", "49.8%"],
    ["Referral", "312", "24.3%"]
  ]
}
```

- 1–6 columns, 1–30 rows. Every row must have exactly one cell per column.
- Keep cells short (≤ 80 characters). Use a chart for trends, not a wall of
  rows.

## Chart block

```json
{
  "type": "chart",
  "kind": "line",
  "title": "Signups by week",
  "xLabel": "Week",
  "yLabel": "Users",
  "series": [
    {
      "name": "This quarter",
      "points": [
        { "x": 0, "y": 120, "label": "W1" },
        { "x": 1, "y": 184, "label": "W2" },
        { "x": 2, "y": 164, "label": "W3" }
      ]
    }
  ]
}
```

- `kind` is `line`, `bar`, `area`, or `scatter`.
- 1–8 series. Series names must be unique.
- 1–2,000 points per series; 5,000 points total across the whole spec.
- Every point needs numeric `x` and `y`. `x` must be unique within a series.
- `label` is optional tick text (for example a date or category). Put units in
  `xLabel`/`yLabel`, not in every tick.
- For `line`, `area`, and `bar`, `x` is the position and `label` is the tick
  text. For `scatter`, both `x` and `y` are values.
- `bar` and `area` include zero in their baseline. `line` and `scatter` use the
  data range with padding. Negative values are supported.
- Multiple bar series are drawn side by side; multiple line/area series are
  overlaid.

Choose the simplest chart that answers the question:

- **line** — a trend over ordered time or sequence.
- **area** — a trend where the magnitude under the line matters.
- **bar** — comparison across categories, especially a small number of them.
- **scatter** — relationship between two numeric values.
- **table** — exact values the user needs to read, not a trend.
- **metric** — one headline number.

## Limits

- Canonical spec: 256 KiB.
- 12 blocks, 8 chart series, 2,000 points per series, 5,000 points total.
- 6 table columns, 30 table rows, 1,000 characters of text.
- Title 160 characters; subtitle 240; chart title 160; axis labels and series
  names 80; point labels 40.

The server rejects a spec over a limit rather than silently truncating it. If
you hit the point limit, aggregate the data before plotting.
