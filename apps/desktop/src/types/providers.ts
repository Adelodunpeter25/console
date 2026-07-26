/**
 * Desktop-specific provider/model response types.
 *
 * `ProviderCatalogEntry` and `Model` are shared via `@console/types`; this
 * file holds the dynamic models endpoint wrapper.
 */

import type { Model } from "@console/types";

export interface ProviderModelsResult {
  provider: string;
  models: Model[];
}
