//! Embedded device-stream player for the Devices inspector tab.
//!
//! The server vendors `expo-device-hub` for all screen streaming (see the
//! device-simulator service spec): this page owns no capture code. It loads
//! `player.html` over `with_html` and drives it with `evaluate_script`:
//!
//! - iOS (`serve-sim`): an HTTP AVCC video body decoded with WebCodecs,
//!   input on a separate binary WebSocket (tags: touch `0x03`, button
//!   `0x04`, key `0x06`).
//! - Android (`serve-emu`): one WebSocket at `ws?device=<id>&frame-meta=1`
//!   carrying SEMU-framed H.264 with JSON gestures upstream.
//!
//! When the hub proxy is not reachable the player falls back to the
//! `GET /api/devices/:id/screenshot` still (polled via `fetch`).

pub const PLAYER_HTML: &str = r#"<!doctype html>
<html>
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<style>
  html, body { margin: 0; padding: 0; height: 100%; background: #0c0c0e; overflow: hidden; }
  #stage { position: relative; width: 100%; height: 100%; display: flex; align-items: center; justify-content: center; }
  canvas, img#still { max-width: 100%; max-height: 100%; object-fit: contain; }
  #still { display: none; }
  #status { position: absolute; left: 8px; bottom: 8px; font: 11px/1.4 -apple-system, system-ui, sans-serif; color: rgba(255,255,255,.75); background: rgba(0,0,0,.45); padding: 4px 8px; border-radius: 6px; pointer-events: none; }
</style>
</head>
<body>
<div id="stage">
  <canvas id="screen"></canvas>
  <img id="still" alt="device screen" />
  <div id="status">idle</div>
</div>
<script>
(function () {
  const status = document.getElementById('status');
  const canvas = document.getElementById('screen');
  const still = document.getElementById('still');
  const ctx = canvas.getContext('2d');
  let cfg = null;
  let ws = null;
  let decoder = null;
  let pollTimer = null;
  let frameCount = 0;

  const setStatus = (s) => { status.textContent = s; report({ type: 'status', status: s }); };
  const report = (msg) => { try { window.ipc.postMessage(JSON.stringify(msg)); } catch (e) {} };

  function stopAll() {
    try { if (ws) ws.close(); } catch (e) {}
    ws = null;
    try { if (decoder && decoder.state !== 'closed') decoder.close(); } catch (e) {}
    decoder = null;
    if (pollTimer) { clearInterval(pollTimer); pollTimer = null; }
  }

  function showStill(url) {
    canvas.style.display = 'none';
    still.style.display = 'block';
    still.src = url;
  }

  function startStills() {
    setStatus('stills');
    canvas.style.display = 'none';
    still.style.display = 'block';
    const tick = async () => {
      try {
        const res = await fetch(cfg.screenshotUrl, { cache: 'no-store' });
        if (!res.ok) throw new Error('http ' + res.status);
        const blob = await res.blob();
        still.src = URL.createObjectURL(blob);
        frameCount++;
        report({ type: 'frame', count: frameCount });
      } catch (e) {
        setStatus('stills error: ' + (e && e.message || e));
      }
    };
    tick();
    pollTimer = setInterval(tick, 1500);
  }

  const SEMU_MAGIC = 0x53454d55;
  const SEMU_HEADER = 16;

  function parseSemu(buf) {
    if (buf.byteLength > SEMU_HEADER) {
      const view = new DataView(buf, 0, SEMU_HEADER);
      if (view.getUint32(0, false) === SEMU_MAGIC && view.getUint8(4) === 1) {
        return { data: buf.slice(SEMU_HEADER), key: (view.getUint8(5) & 1) !== 0 };
      }
    }
    return { data: buf, key: null };
  }

  async function ensureDecoder(codec) {
    if (decoder) return decoder;
    if (!('VideoDecoder' in window)) throw new Error('no WebCodecs');
    decoder = new VideoDecoder({
      output: (frame) => {
        if (canvas.width !== frame.displayWidth || canvas.height !== frame.displayHeight) {
          canvas.width = frame.displayWidth;
          canvas.height = frame.displayHeight;
        }
        ctx.drawImage(frame, 0, 0);
        frame.close();
        frameCount++;
        report({ type: 'frame', count: frameCount });
      },
      error: (e) => setStatus('decode error: ' + (e && e.message || e)),
    });
    decoder.configure({ codec: codec || 'avc1.42E01E' });
    return decoder;
  }

  function feed(chunk, key) {
    ensureDecoder(cfg.codec).then((d) => {
      try {
        d.decode(new EncodedVideoChunk({ type: key === false ? 'delta' : 'key', timestamp: performance.now() * 1000, data: chunk }));
      } catch (e) { setStatus('feed error: ' + (e && e.message || e)); }
    }).catch((e) => {
      setStatus('decoder unavailable, stills fallback');
      stopAll();
      startStills();
    });
  }

  function startAndroid() {
    setStatus('connecting');
    still.style.display = 'none';
    canvas.style.display = 'block';
    ws = new WebSocket(cfg.streamUrl);
    ws.binaryType = 'arraybuffer';
    ws.onopen = () => setStatus('streaming');
    ws.onmessage = (ev) => {
      if (typeof ev.data === 'string') return; // session messages
      const { data, key } = parseSemu(ev.data);
      feed(data, key);
    };
    ws.onerror = () => setStatus('stream error, stills fallback');
    ws.onclose = () => { if (cfg) { setStatus('stream closed, stills fallback'); stopAll(); startStills(); } };
  }

  async function startIos() {
    setStatus('connecting');
    still.style.display = 'none';
    canvas.style.display = 'block';
    // Video stream: HTTP streaming body (AVCC or Annex B H.264).
    try {
      const res = await fetch(cfg.streamUrl, { cache: 'no-store' });
      if (!res.ok || !res.body) throw new Error('http ' + res.status);
      setStatus('streaming');
      const reader = res.body.getReader();
      let buf = new Uint8Array(0);
      const append = (chunk) => {
        const next = new Uint8Array(buf.length + chunk.length);
        next.set(buf);
        next.set(chunk, buf.length);
        buf = next;
      };
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        append(value);
        while (buf.length > 4) {
          const len = (buf[0] << 24) | (buf[1] << 16) | (buf[2] << 8) | buf[3];
          if (len > 0 && len <= buf.length - 4 && (buf[0] !== 0 || buf[1] !== 0 || buf[2] > 1)) {
            const unit = buf.slice(4, 4 + len).buffer;
            buf = buf.slice(4 + len);
            feed(unit, null);
            continue;
          }
          let start = -1;
          let prefixLen = 0;
          for (let i = 0; i < buf.length - 3; i++) {
            if (buf[i] === 0 && buf[i+1] === 0 && buf[i+2] === 1) {
              start = i;
              prefixLen = 3;
              break;
            }
            if (buf[i] === 0 && buf[i+1] === 0 && buf[i+2] === 0 && buf[i+3] === 1) {
              start = i;
              prefixLen = 4;
              break;
            }
          }
          if (start === -1) break;
          let nextStart = -1;
          for (let i = start + prefixLen; i < buf.length - 3; i++) {
            if ((buf[i] === 0 && buf[i+1] === 0 && buf[i+2] === 1) ||
                (buf[i] === 0 && buf[i+1] === 0 && buf[i+2] === 0 && buf[i+3] === 1)) {
              nextStart = i;
              break;
            }
          }
          if (nextStart === -1) {
            if (start > 0) buf = buf.slice(start);
            break;
          }
          const unit = buf.slice(start, nextStart).buffer;
          buf = buf.slice(nextStart);
          feed(unit, null);
        }
      }
    } catch (e) {
      setStatus('stream error, stills fallback');
      stopAll();
      startStills();
    }
    // Control socket for input events if configured.
    if (cfg && cfg.controlUrl) {
      try {
        ws = new WebSocket(cfg.controlUrl);
        ws.binaryType = 'arraybuffer';
      } catch (e) {}
    }
  }

  window.__consoleDevice = {
    start: (json) => {
      stopAll();
      frameCount = 0;
      try { cfg = JSON.parse(json); } catch (e) { setStatus('bad config'); return; }
      if (!cfg || !cfg.platform) { setStatus('no device'); return; }
      if (cfg.mode === 'stills' || !cfg.streamUrl) { startStills(); return; }
      if (cfg.platform === 'android' && cfg.streamUrl.startsWith('ws')) startAndroid(); else startIos();
    },
    stop: () => { cfg = null; stopAll(); setStatus('idle'); },
    tap: (x, y) => {
      if (!ws || ws.readyState !== 1 || !cfg) return;
      if (cfg.platform === 'android') {
        ws.send(JSON.stringify({ type: 'tap', x, y }));
      } else {
        const payload = JSON.stringify({ x, y });
        const bytes = new TextEncoder().encode(payload);
        const out = new Uint8Array(1 + bytes.length);
        out[0] = 0x03; out.set(bytes, 1);
        ws.send(out.buffer);
      }
    },
    key: (key) => {
      if (!ws || ws.readyState !== 1 || !cfg) return;
      if (cfg.platform === 'android') {
        ws.send(JSON.stringify({ type: 'key', key }));
      } else {
        const payload = JSON.stringify({ key });
        const bytes = new TextEncoder().encode(payload);
        const out = new Uint8Array(1 + bytes.length);
        out[0] = 0x06; out.set(bytes, 1);
        ws.send(out.buffer);
      }
    },
  };

  canvas.addEventListener('pointerdown', (ev) => {
    const rect = canvas.getBoundingClientRect();
    const x = (ev.clientX - rect.left) / Math.max(1, rect.width);
    const y = (ev.clientY - rect.top) / Math.max(1, rect.height);
    report({ type: 'tap', x, y });
    try { window.__consoleDevice.tap(x, y); } catch (e) {}
  });

  setStatus('ready');
})();
</script>
</body>
</html>
"#;

/// JSON config passed to `window.__consoleDevice.start(...)`.
#[derive(Clone, Debug)]
pub struct PlayerConfig {
    pub platform: String,
    pub stream_url: Option<String>,
    pub control_url: Option<String>,
    pub screenshot_url: String,
    pub codec: Option<String>,
}

impl PlayerConfig {
    pub fn start_script(&self) -> String {
        let escaped = serde_json::to_string(&serde_json::json!({
            "platform": self.platform,
            "mode": if self.stream_url.is_some() { "stream" } else { "stills" },
            "streamUrl": self.stream_url,
            "controlUrl": self.control_url,
            "screenshotUrl": self.screenshot_url,
            "codec": self.codec,
        }))
        .unwrap_or_else(|_| "{}".to_string());
        format!(
            "window.__consoleDevice && window.__consoleDevice.start({})",
            serde_json::to_string(&escaped).unwrap_or_else(|_| "\"{}\"".to_string())
        )
    }

    pub fn stop_script() -> &'static str {
        "window.__consoleDevice && window.__consoleDevice.stop()"
    }
}
