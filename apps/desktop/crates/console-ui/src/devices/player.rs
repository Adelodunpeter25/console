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
  html, body { margin: 0; padding: 0; height: 100%; width: 100%; background: #0c0c0e; overflow: hidden; }
  #stage { position: relative; width: 100%; height: 100%; display: flex; align-items: center; justify-content: center; }
  #screen, #still { max-width: 100%; max-height: 100%; object-fit: contain; display: block; }
  #still { display: none; }
  #status { position: absolute; left: 8px; bottom: 8px; font: 11px/1.4 -apple-system, system-ui, sans-serif; color: rgba(255,255,255,.75); background: rgba(0,0,0,.45); padding: 4px 8px; border-radius: 6px; pointer-events: none; }
</style>
</head>
<body>
<div id="stage">
  <img id="screen" alt="device screen" />
  <img id="still" alt="device screen still" />
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

  screen.addEventListener('pointerdown', (ev) => {
    const rect = screen.getBoundingClientRect();
    const x = (ev.clientX - rect.left) / Math.max(1, rect.width);
    const y = (ev.clientY - rect.top) / Math.max(1, rect.height);
    report({ type: 'tap', x, y });
    try { window.__consoleDevice.tap(x, y); } catch (e) {}
  });

  still.addEventListener('pointerdown', (ev) => {
    const rect = still.getBoundingClientRect();
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
