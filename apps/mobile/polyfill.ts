// Polyfill FormData early to prevent ReferenceError from early-evaluating dependencies in the monorepo
if (typeof globalThis.FormData === "undefined" || typeof global.FormData === "undefined") {
  try {
    const FormDataPolyfill = require("react-native/Libraries/Network/FormData");
    const ResolvedFormData = FormDataPolyfill.default || FormDataPolyfill;
    (globalThis as any).FormData = ResolvedFormData;
    (global as any).FormData = ResolvedFormData;
  } catch (e) {
    // Fallback if needed
  }
}

