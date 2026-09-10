import type { Model } from "@console/types";
import { resolveRoleModel, type ConsoleModelRole } from "./model-roles.js";

/** Resolves configured role models, falling back to the session's active model. */
export async function resolveModelRole(role: ConsoleModelRole, fallback: Model): Promise<Model> {
  return resolveRoleModel(role, fallback);
}
