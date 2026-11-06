// The chat mention/comment/paste text helpers, ported from console-ui's
// `chat/message_bubble.rs` (their logic mirrors `console_shared::composer`
// and `console_shared::chat_refs`). These are the TS-side subsets; when the
// Rust core exposes them over the bridge, callers should move to those and
// this file shrinks away.

export interface InlineFileMention {
  range: { start: number; end: number };
  path: string;
}

// ─── Mention helpers (console-shared subsets) ────────────────────────────────

const CHAT_PATH_PREFIX = "chat:";
const SKILL_PATH_PREFIX = "skill:";

/// The chip's friendly label for a mention path: `$slug`, `/skill`, or the
/// file's basename.
export function mentionLabel(path: string): string {
  if (path.startsWith(CHAT_PATH_PREFIX)) {
    return `$${path.slice(CHAT_PATH_PREFIX.length)}`;
  }
  if (path.startsWith(SKILL_PATH_PREFIX)) {
    return `/${path.slice(SKILL_PATH_PREFIX.length)}`;
  }
  return baseName(path);
}

export function baseName(path: string): string {
  const parts = path.split(/[\\/]/);
  return parts[parts.length - 1] ?? path;
}

/// A `$slug` chat reference found in text; `range` covers the `$` and every
/// ref character after it.
export interface ChatRef {
  range: { start: number; end: number };
  slug: string;
}

/// Find `$ref` tokens the way the server's `ParseDollarRefs` does: a `$` at
/// the start or after whitespace or opening punctuation, then ASCII letters,
/// digits, `-` or `_`. Edge dashes/underscores are trimmed from the slug,
/// and a digit-led token only counts when it looks like an id (contains `-`,
/// at least 8 long), so prices stay plain text. Repeats are all returned.
export function parseChatRefs(text: string): ChatRef[] {
  const refs: ChatRef[] = [];
  for (let i = 0; i < text.length; i++) {
    if (text[i] !== "$") continue;
    const before = text.slice(0, i).at(-1);
    const boundary =
      before == null || /\s/.test(before) || "([{\"'<".includes(before);
    if (!boundary) continue;
    const rest = text.slice(i + 1);
    const raw = /^[A-Za-z0-9_-]*/.exec(rest)![0];
    const slug = raw.replace(/^[-_]+|[-_]+$/g, "");
    const digitLed = /^[0-9]/.test(slug);
    if (slug === "" || (digitLed && !(slug.includes("-") && slug.length >= 8))) continue;
    refs.push({ range: { start: i, end: i + 1 + slug.length }, slug });
  }
  return refs;
}

/// One staged file line comment recovered from a prompt.
export interface FileCommentRef {
  path: string;
  lines: string;
  text: string;
}

const COMMENT_OPEN = '<file_comment path="';
const COMMENT_CLOSE = "</file_comment>";

/// Split `content` into the user's own text and the `<file_comment>` blocks
/// in it (`console_shared::composer::file_comments::split_file_comments`).
export function splitFileComments(content: string): [string, FileCommentRef[]] {
  const comments: FileCommentRef[] = [];
  let text = "";
  let rest = content;
  for (;;) {
    const start = rest.indexOf(COMMENT_OPEN);
    if (start < 0) break;
    text += rest.slice(0, start);
    const afterOpen = rest.slice(start + COMMENT_OPEN.length);
    const pathEnd = afterOpen.indexOf('" lines="');
    if (pathEnd < 0) break;
    const path = afterOpen.slice(0, pathEnd).replace(/&quot;/g, '"');
    const afterLines = afterOpen.slice(pathEnd + '" lines="'.length);
    const gtEnd = afterLines.indexOf('">');
    if (gtEnd < 0) break;
    const lines = afterLines.slice(0, gtEnd);
    const afterTag = afterLines.slice(gtEnd + '">'.length);
    const closeEnd = afterTag.indexOf(COMMENT_CLOSE);
    if (closeEnd < 0) break;
    comments.push({
      path,
      lines,
      text: afterTag.slice(0, closeEnd).replace(/^\n+|\n+$/g, ""),
    });
    rest = afterTag.slice(closeEnd + COMMENT_CLOSE.length);
  }
  text += rest;
  return [text.trim(), comments];
}

/// Find the next `pasted <N> lines` label (the friendly text a large-paste
/// mention writes into the prompt) bounded by whitespace. Returns its
/// byte range within `remaining`.
export function findPasteLabel(remaining: string): [number, number] | undefined {
  const PREFIX = "pasted ";
  let from = 0;
  for (;;) {
    const rel = remaining.slice(from).indexOf(PREFIX);
    if (rel < 0) return undefined;
    const start = from + rel;
    const beforeOk =
      start === 0 || /\s/.test(remaining[start - 1]);
    let end = start + PREFIX.length;
    const digitsMatch = /^[0-9,]*/.exec(remaining.slice(end))![0];
    end += digitsMatch.length;
    if (
      beforeOk &&
      digitsMatch.length > 0 &&
      remaining.slice(end).startsWith(" lines") &&
      (end + " lines".length >= remaining.length ||
        /\s/.test(remaining[end + " lines".length]))
    ) {
      return [start, end + " lines".length];
    }
    from = start + PREFIX.length;
  }
}

/// A pasted upload's context file lives under the server's pastes dir
/// (`.../pastes/<session>/pasted-<ts>-<rand>.log|.txt`).
function isPastePath(path: string): boolean {
  if (path.includes("/pastes/")) return true;
  const name = baseName(path);
  return name.startsWith("pasted-");
}

/// Keep the message as one flat string and return the ranges that should be
/// painted as file mentions. Separate flex children cannot wrap text around
/// a pill like the composer does (`message_text_with_mentions`).
export function messageTextWithMentions(
  content: string,
  contextFiles: string[],
): [string, InlineFileMention[]] {
  // Build (label, path) pairs. Pastes are kept apart: their friendly
  // composer text is `pasted <N> lines`, which never matches the basename,
  // so they are detected by pattern and mapped in order.
  const pills: [string, string][] = [];
  const pastePaths: string[] = [];
  for (const path of contextFiles) {
    if (isPastePath(path)) pastePaths.push(path);
    else pills.push([mentionLabel(path), path]);
  }
  let nextPaste = 0;

  let text = "";
  const mentions: InlineFileMention[] = [];
  let remaining = content;

  while (remaining.length > 0) {
    // Find the earliest label-or-path occurrence among all pills.
    let earliest: [number, number, number] | undefined;
    let earliestPaste = nextPaste < pastePaths.length ? findPasteLabel(remaining) : undefined;
    for (const [pillIdx, [label, path]] of pills.entries()) {
      for (const needle of [label, path]) {
        if (needle === "") continue;
        const pos = remaining.indexOf(needle);
        if (pos < 0) continue;
        // Must be surrounded by whitespace or string boundaries.
        const beforeOk = pos === 0 || /\s/.test(remaining[pos - 1]);
        const afterPos = pos + needle.length;
        const afterOk =
          afterPos >= remaining.length || /\s/.test(remaining[afterPos]);
        if (beforeOk && afterOk) {
          // Prefer the earliest match; break ties with the longer needle so
          // a full path wins over its bare filename.
          const replace =
            earliest == undefined ||
            pos < earliest[0] ||
            (pos === earliest[0] && afterPos - pos > earliest[1] - earliest[0]);
          if (replace) earliest = [pos, afterPos, pillIdx];
          break;
        }
      }
    }

    // A paste label only wins if it starts before the earliest pill.
    if (
      earliestPaste != null &&
      (earliest == undefined || earliestPaste[0] < earliest[0])
    ) {
      const [pstart, pend] = earliestPaste;
      text = pushPrefix(remaining.slice(0, pstart), text);
      text += remaining.slice(pstart, pend);
      mentions.push({
        range: { start: text.length - (pend - pstart), end: text.length },
        path: pastePaths[nextPaste],
      });
      nextPaste += 1;
      remaining = remaining.slice(pend);
      if (remaining.startsWith("  ")) {
        remaining = remaining.replace(/^ +/, "");
        if (remaining.length > 0) text += " ";
      }
      continue;
    }

    if (earliest == undefined) {
      text += remaining;
      break;
    }
    const [start, end, pillIdx] = earliest;
    text = pushPrefix(remaining.slice(0, start), text);
    const [label, path] = pills[pillIdx];
    mentions.push({
      range: { start: text.length, end: text.length + label.length },
      path,
    });
    text += label;
    remaining = remaining.slice(end);
    if (remaining.startsWith("  ")) {
      remaining = remaining.replace(/^ +/, "");
      if (remaining.length > 0) text += " ";
    }
  }

  addChatRefMentions(text, mentions);
  return [text, mentions];
}

/// The prefix before a mention: collapse trailing spaces to one (unless the
/// text is empty or ends in a newline), or keep as-is mid-word.
function pushPrefix(prefix: string, text: string): string {
  if (/^\s*$/.test(prefix)) {
    // Mention at start of message: don't add leading whitespace.
    if (text === "") return text;
    return text + " ";
  }
  if (/\s$/.test(prefix)) {
    let next = text + prefix.replace(/ +$/, "");
    if (next !== "" && !next.endsWith("\n")) next += " ";
    return next;
  }
  return text + prefix;
}

/// Chat refs sent before they were recorded as context paths have no
/// metadata, so find them in the text with the shared `$ref` rules.
function addChatRefMentions(text: string, mentions: InlineFileMention[]): void {
  const found = parseChatRefs(text)
    .filter((ref) =>
      mentions.every(
        (m) => m.range.end <= ref.range.start || m.range.start >= ref.range.end,
      ),
    )
    .map((ref) => ({
      range: ref.range,
      path: `${CHAT_PATH_PREFIX}${ref.slug}`,
    }));
  mentions.push(...found);
}
