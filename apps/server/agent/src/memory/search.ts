/**
 * Deterministic keyword/tag scoring — no embeddings, no external deps.
 * Pure function over already-loaded entries so it's unit-testable without
 * touching SQLite.
 */
import type { MemoryEntry, RecallMatch, RecallQuery } from "./types.js";

const TAG_MATCH_WEIGHT = 3;
const TEXT_MATCH_WEIGHT = 1;
const DEFAULT_LIMIT = 10;

export function recallMemories(entries: MemoryEntry[], query: RecallQuery, limit = DEFAULT_LIMIT): RecallMatch[] {
  const wantedTags = new Set(query.tags ?? []);
  const text = query.text?.toLowerCase().trim();

  const scored: RecallMatch[] = entries.map((entry) => {
    let score = 0;
    for (const tag of entry.tags) {
      if (wantedTags.has(tag)) score += TAG_MATCH_WEIGHT;
    }
    if (text && entry.content.toLowerCase().includes(text)) {
      score += TEXT_MATCH_WEIGHT;
    }
    return { entry, score };
  });

  return scored
    .filter((match) => match.score > 0)
    .sort((a, b) => b.score - a.score)
    .slice(0, limit);
}
