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
  #screen, #still { max-width: 100%; max-height: 100%; object-fit: contain; display: block; -webkit-user-drag: none; user-select: none; -webkit-user-select: none; touch-action: none; }
  #still { display: none; }
  #status { position: absolute; left: 8px; bottom: 8px; font: 11px/1.4 -apple-system, system-ui, sans-serif; color: rgba(255,255,255,.75); background: rgba(0,0,0,.45); padding: 4px 8px; border-radius: 6px; pointer-events: none; }
</style>
</head>
<body>
<div id="stage">
  <img id="screen" draggable="false" alt="device screen" />
  <img id="still" draggable="false" alt="device screen still" />
  <div id="status">idle</div>
</div>
<script>
(function () {
  const status = document.getElementById('status');
  const screen = document.getElementById('screen');
  const still = document.getElementById('still');
  let cfg = null;
  let ws = null;
  let pollTimer = null;
  let frameCount = 0;

  const setStatus = (s) => { status.textContent = s; report({ type: 'status', status: s }); };
  const report = (msg) => { try { window.ipc.postMessage(JSON.stringify(msg)); } catch (e) {} };

  function stopAll() {
    try { if (ws) ws.close(); } catch (e) {}
    ws = null;
    if (pollTimer) { clearInterval(pollTimer); pollTimer = null; }
    try { screen.src = ''; } catch (e) {}
  }

  function startStills() {
    setStatus('stills');
    screen.style.display = 'none';
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

  function startStream() {
    setStatus('connecting');
    still.style.display = 'none';
    screen.style.display = 'block';
    screen.src = cfg.streamUrl;
    screen.onload = () => {
      setStatus('streaming');
      frameCount++;
      report({ type: 'frame', count: frameCount });
    };
    screen.onerror = () => {
      if (cfg && cfg.screenshotUrl) {
        setStatus('stream error, stills fallback');
        startStills();
      }
    };
  }

  window.__consoleDevice = {
    start: (json) => {
      stopAll();
      frameCount = 0;
      try { cfg = JSON.parse(json); } catch (e) { setStatus('bad config'); return; }
      if (!cfg || !cfg.platform) { setStatus('no device'); return; }
      if (cfg.mode === 'stills' || !cfg.streamUrl) { startStills(); return; }
      startStream();
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
        // Drag/swipe gesture (scrolling, swiping)
        report({
          type: 'swipe',
          startX: Math.max(0, Math.min(1, startX)),
          startY: Math.max(0, Math.min(1, startY)),
          endX: Math.max(0, Math.min(1, endX)),
          endY: Math.max(0, Math.min(1, endY)),
          durationMs: Math.max(150, Math.min(elapsed, 1000)),
        });
      } else {
        // Tap gesture
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

  setupGestures(screen);
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
