# Mobile Markdown Preview — Specification

**Status:** Draft
**Applies to:** Mobile (`apps/mobile`)
**Owner:** Console Mobile

---

## 1. Overview & Problem

Agent responses stream as markdown (headings, lists, code fences, tables, links). Mobile currently renders markdown only in final bubbles (`components/common/markdown-renderer.tsx` via `react-native-markdown-display`) — streaming shows plain `Text`, no inline preview, no copy, no link handling while streaming.

Goals:
- Live markdown preview while streaming (incremental, no flicker).
- Consistent styling with desktop `crates/console-ui/src/markdown/` (pulldown-cmark) and existing mobile renderer.
- Performant on long threads (virtualized transcript).

Non-goals: editing markdown, WYSIWYG composer, desktop parity for tables/veil.

---

## 2. Current State

- `apps/mobile/components/common/markdown-renderer.tsx:60` — `MarkdownRenderer({content})` with `react-native-markdown-display@7.0.2` (`markdown-it@10`), custom `style` + `rules` (h1-h4, lists, `code_inline`, `fence`/`code_block` → `SyntaxHighlighter`).
- `apps/mobile/components/common/syntax-highlighter.tsx:3` — `prismjs` with 18 languages, `ScrollView horizontal`, copy via `expo-clipboard`.
- Usage: `components/chat/message-bubbles.tsx:346` (final only), `run-activity.tsx:144`, `tool-result-content.tsx:100`.
- Desktop reference: `crates/console-ui/src/markdown/parser.rs` (Block tree, IncrementalParser, mend), `render.rs` (FlatText cache, RowVeil, palette).

---

## 3. Proposed Design

### 3.1 Component

```
apps/mobile/components/common/markdown-preview.tsx
  <MarkdownPreview content={text} streaming={isStreaming} />
```

- Wraps `MarkdownRenderer` for final; for streaming, renders incremental parse with `mend` (close hanging `**`, `` ` ``, ` ``` `) similar to desktop `markdown/mend.rs`.
- Debounce: 32ms coalesce (`requestAnimationFrame` batch) to avoid re-tokenizing per chunk.
- Cache: memoize by `content` length prefix; only last block re-parsed.

### 3.2 Rendering Rules (reuse `markdown-renderer.tsx:101` overrides)

| Element | Style |
|---------|-------|
| `p` | 15/23, `theme.text` |
| `h1-h3` | 18/22 semibold, 16/20, 14/18 |
| `code_inline` | `rgba(255,255,255,0.08)`, orange-300, `JetBrains Mono` 13 |
| `fence` | `SyntaxHighlighter` + `CodeBlockHeader` with language icon + copy |
| `a` | `theme.accent`, `Linking.openURL`, long-press copy |
| `blockquote` | left border 2px `theme.border`, italic |
| `ul/ol` | `•` / `1.` marker, 6px gap |
| `hr`, `table` | 1px divider, horizontal scroll for table |

### 3.3 Streaming Behavior

- While `streaming=true`: show `MarkdownPreview` with `tail` mended (last block) + opacity veil 0.9 for last 2 lines (optional).
- Copy button disabled until `streaming=false`.
- Links with incomplete URL (`PENDING_LINK_URL`) not tappable.
- `selectable Text` remains (desktop `selection.rs` equivalent via RN `selectable`).

### 3.4 Props

```typescript
interface MarkdownPreviewProps {
  content: string;
  streaming?: boolean;
  maxBlocks?: number; // tail cap, default 80
  onLinkPress?: (url: string) => void;
}
```

---

## 4. Implementation Steps

1. **Preview wrapper** — `apps/mobile/components/common/markdown-preview.tsx` with `mendCloseHanging()` util (port of `crates/console-ui/src/markdown/mend.rs`).
2. **Streaming integration** — `components/chat/message-bubbles.tsx:339` replace plain `Text` streaming branch with `<MarkdownPreview streaming />`.
3. **Run activity** — `run-activity.tsx:144` pass `streaming` from `useRunActivityViewModel`.
4. **Perf** — memoize `MarkdownPreview`, `React.memo` + `useMemo` for `style` object.
5. **Tests** — snapshot for fence, link, list, mended `**bold` without closing.

---

## 5. Edge Cases

- Unclosed ` ``` ` — mend closes, renders as code block, not paragraph.
- Bare URL `https://…` — linkify via `linkify-it` (already in `markdown-it`).
- 10k+ char thread — virtualized via `LegendList` (existing transcript), preview only for visible bubble.
- Copy while streaming — queued until settled, 3s check feedback.

---

## 6. Acceptance

- [ ] Streaming bubble shows formatted markdown, not raw `**`/` ``` `.
- [ ] Code fences have header + copy, syntax highlighted.
- [ ] Links tappable after stream ends, not before.
- [ ] No frame drop on 2k token stream (profile).
- [ ] `bunx tsc --noEmit` + `expo export --platform android` pass.

---

## 7. References

- Mobile renderer: `apps/mobile/components/common/markdown-renderer.tsx`
- Highlighter: `apps/mobile/components/common/syntax-highlighter.tsx`
- Desktop parser/mend: `apps/desktop/crates/console-ui/src/markdown/{parser,render,mend}.rs`
- `react-native-markdown-display@7.0.2` + `markdown-it@10`
