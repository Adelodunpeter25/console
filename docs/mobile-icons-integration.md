# Mobile Icons Integration Plan (Providers & File Types)

This plan outlines how to integrate the desktop app's SVG icon library (`providers/` and `file-types/`) into **Console Mobile** using `react-native-svg`.

---

## 1. Overview of Desktop Assets

### A. Provider Icons (`assets/icons/providers/`)
* **Available**: `antigravity.svg`, `codebuff.svg`, `gemini.svg`, `openai.svg`, `opencode.svg`
* **Format**: Pure monochrome / dual-tone SVG vectors with `viewBox="0 0 24 24"`.

### B. File-Type Icons (`assets/icons/file-types/`)
* **Available**: ~100 optimized Material Icon Theme SVGs.
* **Coverage**:
  * **Languages**: TypeScript, JavaScript, Python, Rust, Go, C/C++, Java, Kotlin, Swift, Ruby, PHP, Zig, Lua, Dart, Elixir, Scala, Haskell, Clojure, Solidity, etc.
  * **Frameworks & Tooling**: React, Next.js, Vue, Nuxt, Svelte, Angular, Nest, Vite, Webpack, TailwindCSS, Biome, ESLint, Prettier, Prisma, Docker, Kubernetes, Terraform, etc.
  * **File Extensions & Names**: `.json`, `.yaml`, `.md` / `README`, `Dockerfile`, `.lock`, `.env`, `.svg`, `.png`, `.pdf`, `.zip`, `.sh` / `.zsh`, etc.

---

## 2. Target Use Cases in Mobile

1. **Provider Selector & Model Switcher**:
   * Model picker bottom sheet (`components/chat/composer.tsx`).
   * Provider settings screen (`screens/settings/provider-settings-screen.tsx`).
   * Run Activity & Thinking headers (badge showing which AI provider generated the turn).

2. **Chat Tool Runs & Diff Views**:
   * File badges in `ToolCallRow` & `ToolResultContent` when executing `readFile`, `editFile`, `writeFile`, `batchWrite`.
   * Header icon in `DiffView` (`components/chat/diff-view.tsx`).

3. **Markdown Code Blocks**:
   * Code block header in `MarkdownRenderer` (e.g. showing TypeScript icon next to `typescript`).

4. **File Search & Project Browser**:
   * `@` File mention picker and `useFileSearch` modal list items.
   * `AddProjectScreen` and directory tree browser.

---

## 3. Architecture & Rendering Strategy

### Why `SvgXml` (`react-native-svg`)
`react-native-svg` (`SvgXml`) is already installed in `apps/mobile`. Rather than configuring complex asset transformers for 105 separate SVG files, an **in-memory SVG XML registry**:
* Eliminates asset bundler runtime overhead.
* Guarantees 100% deterministic SVG rendering on Android, iOS, and Web.
* Allows dynamic `color`, `size`, and theme tinting.

---

## 4. Implementation Structure

```
apps/mobile/
├── assets/
│   └── icons/
│       ├── providers/
│       │   ├── antigravity.svg.ts
│       │   ├── codebuff.svg.ts
│       │   ├── gemini.svg.ts
│       │   ├── openai.svg.ts
│       │   ├── opencode.svg.ts
│       │   └── index.ts
│       └── file-types/
│           ├── registry.ts        # Map of extension/filename -> SVG XML
│           └── mapping.ts         # Extension & filename resolver rules
└── components/
    └── icons/
        ├── provider-icon.tsx      # <ProviderIcon provider="antigravity" size={16} />
        ├── file-icon.tsx          # <FileIcon filename="App.tsx" size={16} />
        └── index.ts
```

---

## 5. File Extension & Filename Mapping Logic

```ts
/**
 * Resolves a filename or path to the corresponding Material icon SVG string.
 */
export function getFileIconXml(pathOrName: string): string {
  const filename = pathOrName.split("/").pop()?.toLowerCase() ?? "";

  // 1. Exact Filename Matches
  if (filename === "dockerfile") return FILE_ICONS.docker;
  if (filename === "package.json") return FILE_ICONS.nodejs;
  if (filename === "bun.lock" || filename === "bun.lockb") return FILE_ICONS.bun;
  if (filename === "pnpm-lock.yaml") return FILE_ICONS.pnpm;
  if (filename === "cargo.toml" || filename === "cargo.lock") return FILE_ICONS.rust;
  if (filename === "go.mod" || filename === "go.sum") return FILE_ICONS.go;
  if (filename.startsWith("readme")) return FILE_ICONS.readme;
  if (filename.startsWith(".env")) return FILE_ICONS.settings;

  // 2. Extension Matches
  const ext = filename.split(".").pop() ?? "";
  switch (ext) {
    case "ts":
    case "d.ts":
      return FILE_ICONS.typescript;
    case "tsx":
      return FILE_ICONS.react;
    case "js":
    case "mjs":
    case "cjs":
      return FILE_ICONS.javascript;
    case "jsx":
      return FILE_ICONS.react;
    case "rs":
      return FILE_ICONS.rust;
    case "py":
      return FILE_ICONS.python;
    case "go":
      return FILE_ICONS.go;
    case "json":
      return FILE_ICONS.json;
    case "yaml":
    case "yml":
      return FILE_ICONS.yaml;
    case "md":
    case "mdx":
      return FILE_ICONS.markdown;
    case "sh":
    case "bash":
    case "zsh":
      return FILE_ICONS.console;
    case "sql":
      return FILE_ICONS.database;
    case "css":
    case "scss":
    case "sass":
      return FILE_ICONS.css;
    case "html":
      return FILE_ICONS.html;
    default:
      return FILE_ICONS.file; // Generic fallback
  }
}
```

---

## 6. Components API

### `ProviderIcon`
```tsx
import React from "react";
import { SvgXml } from "react-native-svg";
import { getProviderIconXml } from "../../assets/icons/providers";

interface ProviderIconProps {
  provider: string;
  size?: number;
  color?: string;
}

export function ProviderIcon({ provider, size = 16, color }: ProviderIconProps) {
  const xml = getProviderIconXml(provider);
  if (!xml) return null;
  return <SvgXml xml={xml} width={size} height={size} color={color} />;
}
```

### `FileIcon`
```tsx
import React from "react";
import { SvgXml } from "react-native-svg";
import { getFileIconXml } from "../../assets/icons/file-types";

interface FileIconProps {
  filename?: string;
  size?: number;
}

export function FileIcon({ filename = "", size = 16 }: FileIconProps) {
  const xml = getFileIconXml(filename);
  return <SvgXml xml={xml} width={size} height={size} />;
}
```

---

## 7. Next Steps for Implementation
1. Copy and convert SVG assets from `console-rs/assets/icons/` to mobile XML constants.
2. Implement `<ProviderIcon />` and `<FileIcon />`.
3. Integrate `<FileIcon />` into `CodeBlockHeader`, `DiffView`, `ToolResultContent`, and `AddProjectScreen`.
4. Integrate `<ProviderIcon />` into `Composer` model selector and `RunActivity` header.
