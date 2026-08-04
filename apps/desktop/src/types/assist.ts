import type { FileSearchResponse, SlashCommandInfo } from "@console/types";

export type { FileSearchResponse, SlashCommandInfo };

/** An image picked via the native dialog, base64-encoded for inline attachment. */
export interface PickedImage {
  name: string;
  data: string;
  mimeType: string;
}
