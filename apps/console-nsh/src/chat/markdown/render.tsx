// The markdown renderer, ported from console-ui's `markdown/render.rs`: a
// `BlockTree` (see `./types.ts`) laid out as transcript text. The desktop's
// selection machinery, streaming fade and per-frame flatten cache are GPUI
// concerns that nsh's reactivity replaces; math shows as labelled source
// until a typesetting pass exists.

import { Image, Text, View, token } from "nsh";
import type { ColorValue } from "nsh";
import { CopyButton } from "../../common/copy_button";
import { Hint } from "../../common/tooltip";
import { Icon } from "../../common/icons";
import type { Block, BlockTree, InlineRun, TableAlign } from "./types";

/// Type scales, from render.rs's `Metrics`.
export const METRICS = {
  BODY: { textSize: 12.5, lineHeight: 20, codeTextSize: 12.5, codeLineHeight: 18.5, blockGap: 10 },
  USER_MESSAGE: { textSize: 14, lineHeight: 20, codeTextSize: 12.5, codeLineHeight: 18.5, blockGap: 10 },
  COMPACT: { textSize: 11.5, lineHeight: 17, codeTextSize: 10.5, codeLineHeight: 16, blockGap: 6 },
} as const;
export type Metrics = (typeof METRICS)[keyof typeof METRICS];

/// Heading scale relative to body text, by level (`heading_metrics`).
function headingMetrics(level: number, metrics: Metrics) {
  const [scale, weight]: [number, "400" | "600" | "700"] =
    level <= 1
      ? [1.45, "700"]
      : level === 2
        ? [1.28, "700"]
        : level <= 6
          ? [level <= 3 ? 1.14 : 1.05, "600"]
          : [1, "600"];
  const size = Math.round(metrics.textSize * scale);
  return { size, lineHeight: Math.round(size * 1.42), fontWeight: weight };
}

/// Code token colors, from the desktop's restrained three-hue palette
/// (dark-theme values; a light-theme pass can re-derive these from tokens).
type CodeTokenKind = "keyword" | "literal" | "string" | "comment" | "type";

function tokenColor(kind: CodeTokenKind): ColorValue {
  switch (kind) {
    case "keyword":
      return "#C98BC0";
    case "literal":
    case "string":
      return "#94C08A";
    case "comment":
      return token("muted-foreground");
    case "type":
      return "#8FB8D9";
  }
}

const KEYWORDS = new Set([
  "fn", "let", "var", "const", "if", "else", "match", "for", "while", "loop",
  "return", "struct", "enum", "impl", "trait", "pub", "use", "mod", "class",
  "function", "import", "export", "from", "await", "async", "type", "def",
]);

/// A minimal tokenizer for the copy button's neighbour: comments, strings and
/// keywords. The desktop uses tree-sitter; this only tints code until the
/// highlighting pass lands.
function codeTokens(code: string): { text: string; kind: CodeTokenKind | null }[] {
  const tokens: { text: string; kind: CodeTokenKind | null }[] = [];
  const pattern = /(\/\/[^\n]*|#[^\n]*|"[^"\n]*"|'[^'\n]*'|[A-Za-z_][A-Za-z0-9_]*)/g;
  let last = 0;
  for (const match of code.matchAll(pattern)) {
    if (match.index > last) tokens.push({ text: code.slice(last, match.index), kind: null });
    const text = match[0];
    let kind: CodeTokenKind | null = null;
    if (text.startsWith("//") || text.startsWith("#")) kind = "comment";
    else if (text.startsWith("\"") || text.startsWith("'")) kind = "string";
    else if (KEYWORDS.has(text)) kind = "keyword";
    tokens.push({ text, kind });
    last = match.index + text.length;
  }
  if (last < code.length) tokens.push({ text: code.slice(last), kind: null });
  return tokens;
}

/// One styled inline run.
function InlineRunView(props: { run: InlineRun }) {
  const { text, style } = props.run;
  const s = style ?? {};
  if (s.math) {
    // Inline math shows as code until a typesetting pass exists.
    return (
      <Text style={{ fontFamily: "monospace", fontSize: 11.5, color: token("muted-foreground") }}>
        {text}
      </Text>
    );
  }
  return (
    <Text
      class={
        (s.code ? "rounded bg-muted px-[2.5px] " : "") +
        (s.link ? "underline" : "")
      }
      style={{
        fontWeight: s.bold ? "700" : undefined,
        fontStyle: s.italic ? "italic" : undefined,
        textDecorationLine: s.strikethrough ? "line-through" : undefined,
        color: s.code
          ? token("foreground")
          : s.link
            ? token("primary")
            : undefined,
        fontFamily: s.code ? "monospace" : undefined,
      }}
    >
      {text}
    </Text>
  );
}

function RunsText(props: { runs: InlineRun[] }) {
  return (
    <Text>
      {props.runs.map((run, index) => (
        <InlineRunView run={run} />
      ))}
    </Text>
  );
}

/// A fenced code block card: language label and copy button in a header bar,
/// tinted code below.
let codeBlockSerial = 0;

function CodeBlockView(props: {
  language: string | null;
  code: string;
  metrics: Metrics;
}) {
  const id = `code-block-${++codeBlockSerial}`;
  const label = props.language?.toLowerCase() || null;
  return (
    <View class="w-full min-w-0 flex-col overflow-hidden rounded-lg border border-border bg-muted">
      <View class="h-7 w-full flex-row items-center border-b border-border pl-2.5 pr-0.5">
        <View class="min-w-0 flex-1 flex-row items-center gap-1.5">
          {label ? (
            <Text class="text-[10px] font-medium leading-[14px] text-muted-foreground">
              {label}
            </Text>
          ) : null}
        </View>
        <View
          class="h-6 w-6 flex-none flex-row items-center justify-center rounded"
          tooltip={<Hint>Copy code</Hint>}
        >
          <CopyButton id={id} content={props.code} />
        </View>
      </View>
      <View class="w-full min-w-0 overflow-hidden p-2.5">
        <Text
          class="min-w-0"
          style={{
            fontFamily: "monospace",
            fontSize: props.metrics.codeTextSize,
            lineHeight: props.metrics.codeLineHeight,
            color: token("foreground"),
          }}
        >
          {codeTokens(props.code).map((token, index) => (
            <Text
              style={{ color: token.kind ? tokenColor(token.kind) : undefined }}
            >
              {token.text}
            </Text>
          ))}
        </Text>
      </View>
    </View>
  );
}

/// A task-list checkbox (`checkbox` in render.rs).
function TaskCheckbox(props: { checked: boolean; metrics: Metrics }) {
  const box = Math.round(props.metrics.textSize * 0.92);
  return (
    <View
      class="flex-none items-center justify-center rounded-[3px] border"
      style={{
        width: box,
        height: box,
        borderColor: props.checked ? token("primary") : token("border"),
        backgroundColor: props.checked ? token("primary") : undefined,
      }}
    >
      {props.checked ? (
        <Icon name="Check" size={box - 4} color={token("primary-foreground")} />
      ) : null}
    </View>
  );
}

/// An inline or standalone image: scaled-down, rounded, with the alt as a
/// caption beneath so a failed or slow load still says what it was.
function ImageView(props: { url: string; alt: string; metrics: Metrics }) {
  return (
    <View class="w-full min-w-0 flex-col gap-1">
      <Image
        src={props.url}
        style={{ maxWidth: "100%", maxHeight: 320, borderRadius: 6 }}
      />
      {props.alt.trim() !== "" ? (
        <Text
          class="text-muted-foreground"
          style={{ fontSize: props.metrics.textSize - 2, lineHeight: props.metrics.lineHeight - 4 }}
        >
          {props.alt}
        </Text>
      ) : null}
    </View>
  );
}

/// A display formula: shown as labelled LaTeX source (RaTeX has no nsh port).
function MathView(props: { latex: string }) {
  return (
    <View class="w-full min-w-0 flex-row items-center gap-1.5 rounded-lg border border-border bg-muted px-2.5 py-2">
      <Text class="text-[10px] text-muted-foreground">math</Text>
      <Text class="min-w-0 flex-1" style={{ fontFamily: "monospace", fontSize: 12 }}>
        {props.latex}
      </Text>
    </View>
  );
}

/// Content-proportional column widths as fractions of the table, floored so a
/// narrow column stays readable (`column_widths`).
export function columnWidths(
  header: InlineRun[][],
  rows: InlineRun[][][],
  columns: number,
): number[] {
  const content = new Array<number>(columns).fill(0);
  const note = (index: number, cell: InlineRun[] | undefined) => {
    if (index < columns && cell) {
      content[index] = Math.max(
        content[index],
        cell.reduce((sum, run) => sum + run.text.length, 0),
      );
    }
  };
  header.forEach((cell, index) => note(index, cell));
  rows.forEach((row) => row.forEach((cell, index) => note(index, cell)));

  const sum = content.reduce((a, b) => a + b, 0);
  if (sum === 0 || columns === 0) {
    return new Array(columns).fill(1 / Math.max(1, columns));
  }
  // Normalize, then apply the readability floor; the surplus the floor takes
  // from wide columns goes to the widest one so the widths still sum to one.
  const floor = (1 / columns) * 0.55;
  const widths = content.map((value) => Math.max(floor, value / sum));
  const widest = widths.indexOf(Math.max(...widths));
  const surplus = 1 - widths.reduce((a, b) => a + b, 0);
  widths[widest] += surplus;
  return widths;
}

const ALIGN_CLASS: Record<TableAlign, string> = {
  left: "justify-start items-start text-left",
  center: "justify-center items-center text-center",
  right: "justify-end items-end text-right",
};

function CellText(props: {
  runs: InlineRun[];
  metrics: Metrics;
  weight: "400" | "600" | "700";
}) {
  return (
    <Text
      style={{
        fontSize: props.metrics.textSize - 0.5,
        lineHeight: props.metrics.lineHeight - 2,
        fontWeight: props.weight,
      }}
    >
      {props.runs.map((run, index) => (
        <InlineRunView run={run} />
      ))}
    </Text>
  );
}

function TableView(props: {
  header: InlineRun[][];
  rows: InlineRun[][][];
  align: TableAlign[];
  metrics: Metrics;
}) {
  const columns = Math.max(
    props.header.length,
    ...props.rows.map((row) => row.length),
  );
  if (columns === 0) return null;
  const widths = columnWidths(props.header, props.rows, columns);

  const rowView = (
    cells: InlineRun[][],
    weight: "400" | "600" | "700",
    divider: boolean,
    headerRow: boolean,
  ) => (
    <View
      class={
        "w-full min-w-0 flex-row items-start" +
        (divider ? " border-b border-border " : "") +
        (headerRow ? " bg-muted" : "")
      }
    >
      {cells.map((cell, index) => (
        <View
          class={"min-w-0 px-[9px] py-1.5 " + ALIGN_CLASS[props.align[index] ?? "left"]}
          style={{ width: `${(widths[index] ?? 0) * 100}%` }}
        >
          <CellText runs={cell} metrics={props.metrics} weight={weight} />
        </View>
      ))}
    </View>
  );

  return (
    <View class="w-full min-w-0 flex-col overflow-hidden rounded-lg border border-border">
      {props.header.length > 0
        ? rowView(props.header, "600", true, true)
        : null}
      {props.rows.map((row, index) => rowView(row, "400", index + 1 < props.rows.length, false))}
    </View>
  );
}

function BlockView(props: { block: Block; metrics: Metrics }) {
  const { block, metrics } = props;
  switch (block.kind) {
    case "paragraph":
      return (
        <View class="w-full min-w-0">
          <RunsText runs={block.runs} />
        </View>
      );
    case "heading": {
      const heading = headingMetrics(block.level, metrics);
      return (
        <View
          class={"w-full min-w-0" + (block.level <= 2 ? " pt-1" : "")}
          style={{ paddingTop: block.level <= 2 ? 4 : undefined }}
        >
          <Text style={{ fontSize: heading.size, lineHeight: heading.lineHeight, fontWeight: heading.fontWeight, color: token("foreground") }}>
            {block.runs.map((run, index) => (
              <InlineRunView run={run} />
            ))}
          </Text>
        </View>
      );
    }
    case "image":
      return <ImageView url={block.url} alt={block.alt} metrics={metrics} />;
    case "math":
      return <MathView latex={block.latex} />;
    case "codeBlock":
      return (
        <CodeBlockView language={block.language} code={block.code} metrics={metrics} />
      );
    case "blockQuote":
      return (
        <View class="w-full min-w-0 flex-row gap-2.5">
          <View class="w-0.5 flex-none rounded-full bg-border" />
          <View class="min-w-0 flex-auto flex-col" style={{ gap: metrics.blockGap }}>
            {block.children.map((child, index) => (
              <BlockView block={child} metrics={metrics} />
            ))}
          </View>
        </View>
      );
    case "list": {
      const markerWidth = block.orderedStart != null ? 22 : 14;
      return (
        <View class="w-full min-w-0 flex-col" style={{ gap: metrics.blockGap * 0.5 }}>
          {block.items.map((item, index) => (
            <View class="w-full min-w-0 flex-row items-start">
              <View class="flex-none" style={{ width: markerWidth }}>
                {item.task != null ? (
                  <TaskCheckbox checked={item.task} metrics={metrics} />
                ) : (
                  <Text style={{ fontSize: metrics.textSize, lineHeight: metrics.lineHeight, color: token("muted-foreground") }}>
                    {block.orderedStart != null
                      ? `${block.orderedStart + index}.`
                      : "•"}
                  </Text>
                )}
              </View>
              <View
                class="min-w-0 flex-auto flex-col"
                style={{ gap: metrics.blockGap * 0.6 }}
              >
                {item.blocks.map((child, childIndex) => (
                  <BlockView block={child} metrics={metrics} />
                ))}
              </View>
            </View>
          ))}
        </View>
      );
    }
    case "table":
      return (
        <TableView
          header={block.header}
          rows={block.rows}
          align={block.align}
          metrics={metrics}
        />
      );
    case "rule":
      return <View class="my-1 h-px w-full bg-border" />;
  }
}

/// A whole document (or one message's slice of one).
export function MarkdownBlocks(props: {
  tree: BlockTree | Block[];
  metrics?: Metrics;
}) {
  const metrics = props.metrics ?? METRICS.BODY;
  const blocks: Block[] = Array.isArray(props.tree)
    ? props.tree
    : props.tree.blocks.map((top) => top.block);
  return (
    <View class="w-full min-w-0 flex-col" style={{ gap: metrics.blockGap }}>
      {blocks.map((block, index) => (
        <BlockView block={block} metrics={metrics} />
      ))}
    </View>
  );
}
