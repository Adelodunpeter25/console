// Polyfill FormData early to prevent ReferenceError from early-evaluating dependencies in the monorepo
if (typeof globalThis.FormData === "undefined" || typeof global.FormData === "undefined") {
  try {
    const FormDataPolyfill = require("react-native/Libraries/Network/FormData");
    const ResolvedFormData = FormDataPolyfill.default || FormDataPolyfill;
    if (typeof ResolvedFormData === "function") {
      (globalThis as any).FormData = ResolvedFormData;
      (global as any).FormData = ResolvedFormData;
    }
  } catch (e) {
    // Ignore require error
  }
}

// Guarantee globalThis.FormData and global.FormData are never undefined
if (typeof globalThis.FormData === "undefined" || typeof global.FormData === "undefined") {
  class FormDataFallback {
    private _parts: Array<[string, any]> = [];
    append(key: string, value: any) {
      this._parts.push([key, value]);
    }
    getParts() {
      return this._parts;
    }
  }
  (globalThis as any).FormData = (globalThis as any).FormData || FormDataFallback;
  (global as any).FormData = (global as any).FormData || FormDataFallback;
}


