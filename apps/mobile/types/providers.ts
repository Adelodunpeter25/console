import type { Model } from "@console/types";

/** Models endpoint wrapper returned by GET /api/providers/:id/models. */
export interface ProviderModelsResult {
  provider: string;
  models: Model[];
}
