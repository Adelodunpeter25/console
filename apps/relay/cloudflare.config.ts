import { bindings, defineConfig, exports } from "cf/config";
import * as entrypoint from "./src/index.ts" with { type: "cf-worker" };

export default defineConfig({
	worker: {
		name: "relay",
		compatibilityDate: "2026-10-23",
		entrypoint,
		env: {
			TUNNEL: bindings.durableObject({ worker: "relay", exportName: "Tunnel" }),
		},
		exports: {
			Tunnel: exports.durableObject({ storage: "sqlite" }),
		},
	},
});
