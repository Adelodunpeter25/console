/**
 * Devin JWT auth via `GetUserJwt` unary Connect RPC. The returned JWT rides
 * inside `Metadata.userJwt` on the chat/assignment requests. The response
 * may also carry a `customApiServerUrl` that overrides the streaming base.
 */
import { GetUserJwtRequestSchema, GetUserJwtResponseSchema, MetadataSchema } from "./proto/devin-proto.js";
import { create, fromBinary, toBinary, type MessageCodec, type ProtoMessage } from "./proto/protobuf.js";
import { devinCliMetadata, DEVIN_STREAMING_BASE_URL } from "./metadata.js";
import { gunzipSync } from "node:zlib";

const DEVIN_AUTH_PATH = "/exa.auth_pb.AuthService/GetUserJwt";

export interface DevinAuthMetadata {
  userJwt: string;
  /** Overrides the streaming base URL when the backend returns one. */
  baseUrl?: string;
}

/**
 * Decode a unary Connect response. Backend edges variously return bare
 * protobuf or a gzipped protobuf body; attempt direct first, fall back.
 */
export function decodeUnary<T extends ProtoMessage>(
  schema: MessageCodec<T>,
  payload: Uint8Array,
): T | null {
  try {
    return fromBinary(schema, payload);
  } catch {
    try {
      return fromBinary(schema, gunzipSync(payload));
    } catch {
      return null;
    }
  }
}

export async function fetchDevinAuthMetadata(
  apiKey: string | undefined,
  fetchImpl: typeof fetch = fetch,
  signal?: AbortSignal,
): Promise<DevinAuthMetadata> {
  const request = create(GetUserJwtRequestSchema, {
    metadata: create(MetadataSchema, devinCliMetadata(apiKey)),
  });
  const body = toBinary(GetUserJwtRequestSchema, request);

  const response = await fetchImpl(`${DEVIN_STREAMING_BASE_URL}${DEVIN_AUTH_PATH}`, {
    method: "POST",
    headers: {
      "content-type": "application/proto",
      "connect-protocol-version": "1",
      accept: "*/*",
    },
    body: body as unknown as BodyInit,
    signal,
  });

  if (!response.ok) {
    const text = await response.text().catch(() => "");
    throw new Error(`Devin auth error ${response.status}: ${text}`);
  }

  const decoded = decodeUnary(
    GetUserJwtResponseSchema,
    new Uint8Array(await response.arrayBuffer()),
  );
  if (!decoded?.userJwt) {
    throw new Error("Devin auth error: GetUserJwt returned an empty user JWT");
  }
  const custom = (decoded.customApiServerUrl ?? "").trim();
  return {
    userJwt: decoded.userJwt,
    ...(custom ? { baseUrl: custom.replace(/\/+$/, "") } : {}),
  };
}
