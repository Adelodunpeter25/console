// Composer autocomplete for slash commands, `@` file references and `$`
// chat references, ported from console-ui's `common/autocomplete.rs`
// (`detect_trigger`, `filter_items`, `slugify_chat_title`). This module owns
// only trigger detection, filtering, and item text; the host passes the
// candidate lists, keeping backend requests where they are.

import { mentionLabel } from "../utils/mentions";

/// console-core's `SlashCommandInfo`.
export interface SlashCommand {
  name: string;
  description: string;
}

/// console-core's `FileSearchResult`, the fields the popup uses.
export interface SuggestionFile {
  relativePath: string;
  absolutePath: string;
}

/// console-core's `SessionHeader`, the fields the popup uses.
export interface SuggestionChat {
  id: string;
  title: string;
}

export type AutocompleteKind = "command" | "file" | "chat";

export interface AutocompleteTrigger {
  kind: AutocompleteKind;
  /// Character range covering the trigger and query, such as `/plan` or
  /// `@src/`.
  range: { start: number; end: number };
  /// Text after `/`, `@` or `$`.
  query: string;
}

export type AutocompleteItem =
  | { kind: "command"; command: SlashCommand }
  | { kind: "file"; file: SuggestionFile }
  | { kind: "chat"; chat: SuggestionChat };

/// The popup shows at most this many rows, and filtering short-circuits at
/// the cap (the desktop's `MAX_AUTOCOMPLETE_ITEMS`).
const MAX_AUTOCOMPLETE_ITEMS = 80;

/// The path a confirmed item attaches to the prompt: a file's path, `skill:name`
/// for a command, `chat:slug` for a chat ref (`ComposerMention::path`).
export function mentionPath(item: AutocompleteItem): string {
  if (item.kind === "command") return `skill:${item.command.name}`;
  if (item.kind === "file") return item.file.relativePath || item.file.absolutePath;
  return `chat:${slugifyChatTitle(item.chat.title) || item.chat.id}`;
}

/// The text a confirmed item splices over the trigger (`AutocompleteItem::
/// insert_text`).
export function itemInsertText(item: AutocompleteItem): string {
  if (item.kind === "command") return `/${item.command.name} `;
  if (item.kind === "file") {
    // The buffer holds the file's basename (the chip label); the path rides
    // separately as a context file, like the desktop's mention chips.
    return `${mentionLabel(mentionPath(item))} `;
  }
  const slug = slugifyChatTitle(item.chat.title);
  return `$${slug || item.chat.id} `;
}

/// `slugify_chat_title`: lowercase, runs of non-letters/digits collapsed to
/// one '-', trimmed. A `$` ref typed from this slug resolves server-side
/// against the chat's title.
export function slugifyChatTitle(title: string): string {
  let out = "";
  let lastDash = true;
  for (const character of title.toLowerCase()) {
    const alphanumeric = /[a-z0-9\u00c0-\uffff]/.test(character);
    if (alphanumeric) {
      out += character;
      lastDash = false;
    } else if (!lastDash) {
      out += "-";
      lastDash = true;
    }
  }
  return out.replace(/^-+|-+$/g, "");
}

function validCommandCharacter(character: string): boolean {
  return /[a-zA-Z0-9_:-]/.test(character);
}

function validFileCharacter(character: string): boolean {
  return /[a-zA-Z0-9_.\-/]/.test(character);
}

function validRefCharacter(character: string): boolean {
  return /[a-zA-Z0-9_.\-]/.test(character);
}

/// Detect the trigger immediately before the composer's caret. Slash
/// commands and file references must begin at whitespace or the start of
/// the prompt, which keeps email addresses, URLs and ordinary prose from
/// opening the popup; `$` chat refs also allow opening punctuation. A
/// digit-only `$` query is a price, not a ref. A candidate with an invalid
/// query falls through to the later triggers, like the Rust source.
export function detectTrigger(
  value: string,
  cursor: number,
): AutocompleteTrigger | undefined {
  const at = Math.min(cursor, value.length);
  const before = value.slice(0, at);
  const boundaryBefore = (index: number): boolean =>
    index === 0 || /\s/.test(before[index - 1] ?? "");

  // Find the nearest slash-command position: `/` at a boundary (the desktop
  // searches backwards, so only the command currently being typed fires).
  const slash = before.lastIndexOf("/");
  if (slash >= 0 && boundaryBefore(slash)) {
    const query = value.slice(slash + 1, at);
    if ([...query].every(validCommandCharacter)) {
      return { kind: "command", range: { start: slash, end: at }, query };
    }
  }

  // `$` chat references: same boundary rule (start, whitespace, or opening
  // punctuation), slug/id characters only. Digit-only queries are prices,
  // not refs.
  const dollar = before.lastIndexOf("$");
  if (dollar >= 0) {
    const previous = dollar > 0 ? before[dollar - 1] : undefined;
    const query = value.slice(dollar + 1, at);
    const boundary =
      previous == null || /[\s([{<"']/.test(previous);
    const digitsOnly = query !== "" && [...query].every((c) => c >= "0" && c <= "9");
    if (boundary && [...query].every(validRefCharacter) && !digitsOnly) {
      return { kind: "chat", range: { start: dollar, end: at }, query };
    }
  }

  const atSign = before.lastIndexOf("@");
  if (atSign >= 0 && boundaryBefore(atSign)) {
    const query = value.slice(atSign + 1, at);
    if ([...query].every(validFileCharacter)) {
      return { kind: "file", range: { start: atSign, end: at }, query };
    }
  }

  return undefined;
}

/// Filter the candidates for a trigger, capped and short-circuiting like the
/// desktop: commands by name prefix or description contains, files by path
/// contains, chats scored slug-prefix, id-prefix, title-prefix, then
/// contains, ties broken by title.
export function filterItems(
  commands: SlashCommand[],
  files: SuggestionFile[],
  chats: SuggestionChat[],
  trigger: AutocompleteTrigger,
): AutocompleteItem[] {
  const query = trigger.query.toLowerCase();
  if (trigger.kind === "command") {
    if (query === "") {
      return commands
        .slice(0, MAX_AUTOCOMPLETE_ITEMS)
        .map((command) => ({ kind: "command" as const, command }));
    }
    const out: AutocompleteItem[] = [];
    for (const command of commands) {
      if (
        command.name.toLowerCase().startsWith(query) ||
        command.description.toLowerCase().includes(query)
      ) {
        out.push({ kind: "command", command });
        if (out.length >= MAX_AUTOCOMPLETE_ITEMS) break;
      }
    }
    return out;
  }
  if (trigger.kind === "chat") {
    const scored: { score: number; chat: SuggestionChat }[] = [];
    for (const chat of chats) {
      const slug = slugifyChatTitle(chat.title);
      const title = chat.title.toLowerCase();
      const id = chat.id.toLowerCase();
      const score =
        query === ""
          ? 0
          : slug.startsWith(query)
            ? 3
            : id.startsWith(query)
              ? 2
              : title.startsWith(query)
                ? 1
                : slug.includes(query) || title.includes(query)
                  ? 0
                  : -1;
      if (score >= 0) {
        scored.push({ score, chat });
        if (scored.length >= MAX_AUTOCOMPLETE_ITEMS) break;
      }
    }
    return scored
      .sort((a, b) => b.score - a.score || a.chat.title.localeCompare(b.chat.title))
      .map(({ chat }) => ({ kind: "chat" as const, chat }));
  }
  if (query === "") {
    return files
      .slice(0, MAX_AUTOCOMPLETE_ITEMS)
      .map((file) => ({ kind: "file" as const, file }));
  }
  const out: AutocompleteItem[] = [];
  for (const file of files) {
    if (file.relativePath.toLowerCase().includes(query)) {
      out.push({ kind: "file", file });
      if (out.length >= MAX_AUTOCOMPLETE_ITEMS) break;
    }
  }
  return out;
}
