# Mobile Markdown Preview — File Browser / Editor Specification

**Status:** Draft
**Applies to:** Mobile (`apps/mobile/screens/files`, `components/common`)
**Owner:** Console Mobile

---

## 1. Overview & Problem

The mobile file browser (`screens/files/files-screen.tsx:76`) lets users browse `FileTreeBrowser` and open any file. `.md` / `.mdx` files currently render as syntax-highlighted code via `renderHighlightedLine` (`screens/files/files-screen.tsx:203`, `syntax-highlighter.tsx:55` `md→markdown`, prism-markdown) — not as rendered markdown.

Goal: tap a markdown file in the Files tab and see a rendered preview, with a toggle to view raw.

Non-goals: editing markdown in place, desktop `pulldown-cmark` parity for tables/veil, streaming agent markdown (covered elsewhere).

---

## 2. Current State

- **Browser:** `screens/files/files-screen.tsx:1` orchestrates `FileTreeBrowser` (`components/files/FileTreeBrowser.tsx:60` lazy `fsService.getFsEntries`, `LegendList`), `FileTreeRows.tsx:50` `onPressFile -> onSelectFile(path,size)` -> `files-screen.tsx:76` `setSelectedFile` -> `getFilePreviewBlock` gate (`packages/types/src/fs.ts:83`) -> `useReadFile` (`hooks/queries.ts:171` `fsService.readFile -> GET /api/fs/file`).
- **Viewer:** `screens/files/files-screen.tsx:164` single `ScrollView` code view: gutter line numbers (`171`), `renderHighlightedLine(line, getLanguageFromPath(path))` (`syntax-highlighter.tsx:70`). `.md` maps to `markdown` language — still code.
- **Renderer exists but unused for files:** `components/common/markdown-renderer.tsx:60` `MarkdownRenderer({content})` (`react-native-markdown-display@7.0.2` + `markdown-it@10`, custom style/rules for h1-4, lists, `code_inline`, `fence` -> `SyntaxHighlighter` + `CodeBlockHeader` with copy). Used only in chat (`message-bubbles.tsx:346`, `run-activity.tsx:144`, `tool-result-content.tsx:100`).
- **Gating:** `getFilePreviewBlock` blocks `>512KB`, binary, lockfiles. Same check server-side (`apps/server/api/src/services/fs.service.ts:195`).

Desktop ref: `crates/console-ui/src/markdown/{parser,render}.rs` (`pulldown-cmark`, `Metrics`, `Palette`).

---

## 3. Proposed Design

### 3.1 UX

- Open `.md`/`.mdx` → **Preview** rendered by default, segmented control `Preview | Code` in header (like GitHub). Persist choice per app (`useFsStore` or `AsyncStorage` key `md-preview-mode`).
- Non-markdown files unchanged (code view).
- `Preview` is read-only, `selectable`, links tappable (`Linking.openURL`), code fences have header + copy (`SyntaxHighlighter` already does this).
- Empty/large/blocked file states reuse existing `previewBlock` panel (`files-screen.tsx:143`).

```
Header: [← back]  README.md           [Preview | Code]
Body:   Preview -> ScrollView + MarkdownRenderer
        Code    -> existing gutter + renderHighlightedLine
```

### 3.2 Component

```
apps/mobile/components/files/markdown-file-preview.tsx
  <MarkdownFilePreview path={selectedFile.path} content={fileContent} />

apps/mobile/screens/files/files-screen.tsx
  isMarkdown = isMarkdownPath(selectedFile.path) // .md/.mdx/.markdown
  if (isMarkdown) show toggle + conditional <MarkdownRenderer|CodeView>
```

- `isMarkdownPath` via `getFileTypeLanguage` or extension check (`.md`, `.mdx`, `.markdown`). Keep `file-type-mapping.ts:238` as source of truth.
- `MarkdownFilePreview` is thin wrapper over `MarkdownRenderer` + `ScrollView` + padding. No new markdown dep.
- Toggle state: `const [mode, setMode] = useState<'preview'|'code'>('preview')` reset on path change.

### 3.3 Styling (reuse `markdown-renderer.tsx:65` theme)

Reuse existing `MarkdownRenderer` styles (body, h1-4, blockquote, `code_inline` `rgba(255,255,255,0.08)`, fence via `SyntaxHighlighter`). Wrap preview in `ScrollView` with `contentContainerStyle: { padding: 16 }` + `theme.canvas` bg. No new palette.

### 3.4 Props

```typescript
// components/files/markdown-file-preview.tsx
interface MarkdownFilePreviewProps {
  path: string;    // for code fence language fallback
  content: string; // raw file text from useReadFile
}
```

Screen-level:

```typescript
const isMarkdown = useMemo(() => /\.mdx?$/i.test(selectedFile.path), [selectedFile]);
// header toggle: SegmentedControl or two Pressable pills
```

---

## 4. Implementation Steps

1. **Helper** — `utils/icons/file-type-mapping.ts` or `utils/file-helpers.ts` export `isMarkdownPath(path: string): boolean` (`.md/.mdx/.markdown`).
2. **Preview component** — `components/files/markdown-file-preview.tsx` -> `ScrollView` + `MarkdownRenderer` (memoized). Handle `content` empty.
3. **Files screen** — `screens/files/files-screen.tsx:133`:
   - compute `isMarkdown`, `viewMode` state.
   - header: when `isMarkdown` and `!previewBlock && !isLoadingFile && !fileError`, render `Preview | Code` toggle (right of `ScreenHeader` or sub-header row).
   - body: `isMarkdown && viewMode==='preview' ? <MarkdownFilePreview/> : <CurrentCodeView/>`.
   - reset `viewMode` to `preview` on `selectedFilePath` change (via `useEffect`).
4. **Tests** — render `README.md` with headings, fence, link, list; toggle switches; `bunx tsc --noEmit` + `expo export --platform android`.

---

## 5. Edge Cases

- Large markdown `>512KB` — already blocked by `getFilePreviewBlock`, shows blocked panel, no preview.
- Frontmatter `---` — `markdown-it` renders as `hr` + text; acceptable v1 (strip frontmatter optionally).
- Relative links/images `![alt](./img.png)` — render alt text; tapping link tries `Linking.openURL`, fails silently.
- Raw toggle must not re-fetch file (content cached by `useReadFile` query).
- Very long file (5k lines) — `MarkdownRenderer` is non-virtualized; cap with `ScrollView` + consider windowing later.

---

## 6. Acceptance

- [ ] Tapping `README.md` in Files shows rendered preview by default.
- [ ] Toggle `Preview | Code` switches without refetch.
- [ ] Code fences in preview have language icon + copy button.
- [ ] Links tappable, long-press copies URL (existing renderer).
- [ ] Non-markdown files show existing code view unchanged.
- [ ] `bunx tsc --noEmit` + `expo export --platform android` pass.

---

## 7. References

- Viewer: `apps/mobile/screens/files/files-screen.tsx:164` (code view to split)
- Renderer: `apps/mobile/components/common/markdown-renderer.tsx:60`
- Highlighter: `apps/mobile/components/common/syntax-highlighter.tsx:3`
- Browser: `apps/mobile/components/files/FileTreeBrowser.tsx:60`, `FileTreeRows.tsx:50`
- File gating: `packages/types/src/fs.ts:83`, `apps/server/api/src/services/fs.service.ts:195`
- File type map: `apps/mobile/utils/icons/file-type-mapping.ts:238`
