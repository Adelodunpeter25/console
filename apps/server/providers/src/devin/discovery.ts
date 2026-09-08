/**
 * Devin model discovery via the `GetCliModelConfigs` unary Connect RPC.
 *
 * Returns the wire uid + label of each enabled, non-internal config. Cost,
 * reasoning, family collapsing, and image-blind filtering are deferred —
 * this returns just enough to populate the model picker.
 */
import {
  DisplayOption,
  GetCliModelConfigsRequestSchema,
  GetCliModelConfigsResponseSchema,
  MetadataSchema,
} from "./proto/devin-proto.js";
import { create, toBinary } from "./proto/protobuf.js";
import { devinDiscoveryMetadata, DEVIN_STREAMING_BASE_URL } from "./metadata.js";
import { decodeUnary } from "./auth.js";

const DEVIN_GET_CLI_MODEL_CONFIGS_PATH = "/exa.api_server_pb.ApiServerService/GetCliModelConfigs";

/** Display slots we don't surface to the model picker. */
const INTERNAL_MODEL_DISPLAYS = new Set<number>([DisplayOption.QUICK_REVIEW, 6, 7, 8]);

export interface DevinDiscoveredModel {
  id: string;
  name: string;
  contextWindow?: number;
  supportsImages?: boolean;
}

export async function fetchDevinModels(
  fetchImpl: typeof fetch = fetch,
  signal?: AbortSignal,
): Promise<DevinDiscoveredModel[] | null> {
  const apiKey = process.env.DEVIN_OAUTH_TOKEN;
  const request = create(GetCliModelConfigsRequestSchema, {
    metadata: create(MetadataSchema, devinDiscoveryMetadata(apiKey)),
  });
  const body = toBinary(GetCliModelConfigsRequestSchema, request);

  const response = await fetchImpl(
    `${DEVIN_STREAMING_BASE_URL}${DEVIN_GET_CLI_MODEL_CONFIGS_PATH}`,
    {
      method: "POST",
      headers: {
        "content-type": "application/proto",
        "connect-protocol-version": "1",
        accept: "*/*",
      },
      body: body as unknown as BodyInit,
      signal,
    },
  );
  if (!response.ok) return null;

  const decoded = decodeUnary(
    GetCliModelConfigsResponseSchema,
    new Uint8Array(await response.arrayBuffer()),
  );
  if (!decoded) return null;

  const out: DevinDiscoveredModel[] = [];
  const seen = new Set<string>();
  for (const config of decoded.clientModelConfigs) {
    if (config.disabled) continue;
    const display = config.modelInfo?.displayOption ?? DisplayOption.UNSPECIFIED;
    if (INTERNAL_MODEL_DISPLAYS.has(display)) continue;
    const uid = (config.modelUid ?? "").trim();
    if (!uid || seen.has(uid)) continue;
    seen.add(uid);
    out.push({
      id: uid,
      name: (config.label ?? uid).trim() || uid,
      ...(config.maxTokens > 0 ? { contextWindow: config.maxTokens } : {}),
      ...(config.supportsImages ? { supportsImages: true } : {}),
    });
  }
  return out;
}
