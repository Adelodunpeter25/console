// Safely polyfill FormData early for monorepo environments if missing
try {
  const g = typeof globalThis !== "undefined" ? globalThis : global;
  if (typeof (g as any).FormData === "undefined") {
    class FormDataFallback {
      private _parts: Array<[string, any]> = [];
      append(key: string, value: any) {
        this._parts.push([key, value]);
      }
      getParts() {
        return this._parts;
      }
    }
    try {
      Object.defineProperty(g, "FormData", {
        value: FormDataFallback,
        writable: true,
        configurable: true,
      });
    } catch (e) {
      (g as any).FormData = FormDataFallback;
    }
  }
} catch (e) {
  // Ignore polyfill error if already natively provided by React Native
}
