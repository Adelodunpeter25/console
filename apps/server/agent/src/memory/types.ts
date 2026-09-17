export type MemoryScope = "project" | "global";

export interface MemoryEntry {
  id: string;
  scope: MemoryScope;
  content: string;
  tags: string[];
  createdAt: number;
  updatedAt: number;
}

export interface MemoryStoreInput {
  content: string;
  tags?: string[];
}

export interface MemoryUpdateInput {
  content?: string;
  tags?: string[];
}

export interface MemoryListFilter {
  tags?: string[];
}

export interface RecallMatch {
  entry: MemoryEntry;
  score: number;
}

export interface RecallQuery {
  text?: string;
  tags?: string[];
}
