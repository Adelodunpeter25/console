// Wrapper for agent-supplied scripts. Placeholders are substituted in Rust
// (see agent_script.rs). Awaits the script's value, serializes it, and posts
// it back over the webview IPC channel tagged with the request id.
(async () => {
  const requestId = __REQUEST_ID__;
  const source = __SOURCE__;
  const max = __MAX_CHARS__;

  const post = (payload) => {
    try {
      window.ipc.postMessage(
        JSON.stringify(Object.assign({ type: 'script_result', requestId }, payload))
      );
    } catch (_) {}
  };

  const serialize = (value) => {
    if (value === undefined) return 'undefined';
    if (typeof value === 'string') return value;
    if (typeof Node !== 'undefined' && value instanceof Node) {
      return value.outerHTML || value.textContent || String(value);
    }
    const seen = new WeakSet();
    try {
      const json = JSON.stringify(
        value,
        (_key, item) => {
          if (typeof item === 'function') return '[Function]';
          if (typeof item === 'bigint') return item.toString();
          if (typeof Node !== 'undefined' && item instanceof Node) return '[' + item.nodeName + ']';
          if (item && typeof item === 'object') {
            if (seen.has(item)) return '[Circular]';
            seen.add(item);
          }
          return item;
        },
        2
      );
      return json === undefined ? String(value) : json;
    } catch (_) {
      return String(value);
    }
  };

  try {
    let value = (0, eval)(source);
    if (typeof value === 'function') value = value();
    value = await value;
    let text = serialize(value);
    if (text.length > max) {
      text = text.slice(0, max) + '\n...[truncated: ' + text.length + ' chars total]';
    }
    post({ ok: true, value: text });
  } catch (error) {
    const message =
      error && error.message ? (error.name || 'Error') + ': ' + error.message : String(error);
    post({ ok: false, error: message });
  }
})();
