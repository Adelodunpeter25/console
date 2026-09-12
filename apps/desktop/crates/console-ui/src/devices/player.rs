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
  html, body { margin: 0; padding: 0; height: 100%; width: 100%; background: #0c0c0e; overflow: hidden; user-select: none; -webkit-user-select: none; }
  #stage { position: relative; width: 100%; height: 100%; display: flex; align-items: center; justify-content: center; user-select: none; -webkit-user-select: none; }
  canvas, #stream-img, #still { max-width: 100%; max-height: 100%; object-fit: contain; display: block; -webkit-user-drag: none; user-select: none; -webkit-user-select: none; touch-action: none; }
  #stream-img, #still { display: none; }
  #status { position: absolute; left: 8px; bottom: 8px; font: 11px/1.4 -apple-system, system-ui, sans-serif; color: rgba(255,255,255,.75); background: rgba(0,0,0,.45); padding: 4px 8px; border-radius: 6px; pointer-events: none; }
</style>
</head>
<body>
<div id="stage">
  <canvas id="screen"></canvas>
  <img id="stream-img" draggable="false" alt="device screen stream" />
  <img id="still" draggable="false" alt="device screen still" />
  <div id="status">idle</div>
</div>
<script>
(function () {
  const status = document.getElementById('status');
  const canvas = document.getElementById('screen');
  const streamImg = document.getElementById('stream-img');
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
    try { streamImg.src = ''; } catch (e) {}
  }

  function startStills() {
    setStatus('stills');
    canvas.style.display = 'none';
    streamImg.style.display = 'none';
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

  function startMjpegStream() {
    setStatus('connecting');
    canvas.style.display = 'none';
    still.style.display = 'none';
    streamImg.style.display = 'block';
    streamImg.src = cfg.streamUrl;
    streamImg.onload = () => {
      setStatus('streaming');
      frameCount++;
      report({ type: 'frame', count: frameCount });
    };
    streamImg.onerror = () => {
      if (cfg && cfg.screenshotUrl) {
        setStatus('stream error, stills fallback');
        startStills();
      }
    };
  }

  async function ensureDecoder() {
    if (decoder && decoder.state !== 'closed') return decoder;
    if (!('VideoDecoder' in window)) throw new Error('WebCodecs not supported');
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
      error: (e) => setStatus('decoder error: ' + (e && e.message || e)),
    });
    decoder.configure({ codec: cfg.codec || 'avc1.42E01E', optimizeForLatency: true });
    return decoder;
  }

  async function startH264Stream() {
    setStatus('connecting');
    streamImg.style.display = 'none';
    still.style.display = 'none';
    canvas.style.display = 'block';
    try {
      const dec = await ensureDecoder();
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

          const nal = buf.slice(start, nextStart);
          buf = buf.slice(nextStart);

          const nalType = nal[prefixLen] & 0x1f;
          const isKey = nalType === 5 || nalType === 7;
          try {
            dec.decode(new EncodedVideoChunk({
              type: isKey ? 'key' : 'delta',
              timestamp: performance.now() * 1000,
              data: nal.buffer,
            }));
          } catch (e) {}
        }
      }
    } catch (e) {
      setStatus('stream error, stills fallback');
      stopAll();
      startStills();
    }
  }

  window.__consoleDevice = {
    start: (json) => {
      stopAll();
      frameCount = 0;
      try { cfg = JSON.parse(json); } catch (e) { setStatus('bad config'); return; }
      if (!cfg || !cfg.platform) { setStatus('no device'); return; }
      if (cfg.mode === 'stills' || !cfg.streamUrl) { startStills(); return; }
      if (cfg.platform === 'android') startH264Stream(); else startMjpegStream();
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

  function setupGestures(el) {
    let startX = 0;
    let startY = 0;
    let startTime = 0;
    let isDown = false;

    el.addEventListener('dragstart', (ev) => ev.preventDefault());

    el.addEventListener('pointerdown', (ev) => {
      ev.preventDefault();
      const rect = el.getBoundingClientRect();
      startX = (ev.clientX - rect.left) / Math.max(1, rect.width);
      startY = (ev.clientY - rect.top) / Math.max(1, rect.height);
      startTime = performance.now();
      isDown = true;
      try { el.setPointerCapture(ev.pointerId); } catch (e) {}
    });

    el.addEventListener('pointerup', (ev) => {
      if (!isDown) return;
      isDown = false;
      const rect = el.getBoundingClientRect();
      const endX = (ev.clientX - rect.left) / Math.max(1, rect.width);
      const endY = (ev.clientY - rect.top) / Math.max(1, rect.height);
      const elapsed = Math.round(performance.now() - startTime);
      const dx = endX - startX;
      const dy = endY - startY;
      const dist = Math.hypot(dx, dy);

      if (dist > 0.03) {
        report({
          type: 'swipe',
          startX: Math.max(0, Math.min(1, startX)),
          startY: Math.max(0, Math.min(1, startY)),
          endX: Math.max(0, Math.min(1, endX)),
          endY: Math.max(0, Math.min(1, endY)),
          durationMs: Math.max(150, Math.min(elapsed, 1000)),
        });
      } else {
        report({ type: 'tap', x: endX, y: endY });
        try { window.__consoleDevice.tap(endX, endY); } catch (e) {}
      }
      try { el.releasePointerCapture(ev.pointerId); } catch (e) {}
    });

    el.addEventListener('pointercancel', (ev) => {
      isDown = false;
      try { el.releasePointerCapture(ev.pointerId); } catch (e) {}
    });
  }

  setupGestures(canvas);
  setupGestures(streamImg);
  setupGestures(still);

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
