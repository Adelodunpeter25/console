//! Embedded device-stream player for the Devices inspector tab.
//!
//! Speaks the sim-go stream wire (served by the Go server at
//! `GET /api/devices/:id/stream?platform=` as a websocket), identical for iOS
//! and Android:
//!
//! - server → player text: `{"type":"meta","width","height","name","codec"}`
//! - server → player binary: tag byte + payload; `1` avcC description,
//!   `2` key frame, `3` delta (AVCC H.264). Unknown tags are ignored.
//! - player → server text: `{"type":"touch","action":"down|move|up","x","y"}`,
//!   `scroll`, `key` (Android keycode), `text`, `button`, `reset`; x/y in
//!   stream pixels.
//!
//! No screenshot fallback: a dead stream reports its state instead of a
//! frozen picture. Status changes are posted to the host over IPC.

pub const PLAYER_HTML: &str = r#"<!doctype html>
<html>
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<style>
  html, body { margin: 0; padding: 0; height: 100%; width: 100%; background: #0c0c0e; overflow: hidden; user-select: none; -webkit-user-select: none; }
  #stage { position: relative; width: 100%; height: 100%; display: flex; align-items: center; justify-content: center; }
  canvas { max-width: 100%; max-height: 100%; object-fit: contain; display: block; touch-action: none; -webkit-user-drag: none; }
  #status { display: none; position: absolute; left: 8px; bottom: 8px; font: 11px/1.4 -apple-system, system-ui, sans-serif; color: rgba(255,255,255,.9); background: rgba(180,30,30,.85); padding: 4px 8px; border-radius: 6px; pointer-events: none; }
</style>
</head>
<body>
<div id="stage">
  <canvas id="screen"></canvas>
  <div id="status"></div>
</div>
<script>
(function () {
  const statusEl = document.getElementById('status');
  const canvas = document.getElementById('screen');
  const ctx = canvas.getContext('2d');
  let cfg = null, ws = null, decoder = null;
  let streamW = 0, streamH = 0, codec = '', desc = null, needKey = true, seq = 0, gotFrame = false;
  let retry = null;

  const report = (msg) => { try { window.ipc.postMessage(JSON.stringify(msg)); } catch (e) {} };
  const setStatus = (s, isError) => {
    statusEl.textContent = s;
    statusEl.style.display = isError ? 'block' : 'none';
    report({ type: 'status', status: s });
  };

  function closeDecoder() {
    try { if (decoder && decoder.state !== 'closed') decoder.close(); } catch (e) {}
    decoder = null; needKey = true;
  }

  function stopAll() {
    if (retry) { clearTimeout(retry); retry = null; }
    if (ws) { ws.onclose = null; try { ws.close(); } catch (e) {} ws = null; }
    closeDecoder();
    codec = ''; desc = null; streamW = 0; streamH = 0; seq = 0; gotFrame = false;
  }

  function ensureDecoder() {
    if (decoder) return true;
    if (!codec || !desc) return false;
    if (typeof VideoDecoder === 'undefined') { setStatus('WebCodecs unavailable', true); return false; }
    try {
      decoder = new VideoDecoder({
        output(frame) {
          if (canvas.width !== frame.displayWidth || canvas.height !== frame.displayHeight) {
            canvas.width = frame.displayWidth; canvas.height = frame.displayHeight;
          }
          ctx.drawImage(frame, 0, 0);
          frame.close();
          if (!gotFrame) { gotFrame = true; setStatus('streaming'); }
        },
        error(e) {
          // Usually a missed description/key frame: reset, don't reconnect.
          closeDecoder();
          send({ type: 'reset' });
        },
      });
      decoder.configure({ codec, description: desc, optimizeForLatency: true });
      needKey = true;
      return true;
    } catch (e) {
      setStatus('decoder error: ' + (e && e.message || e), true);
      decoder = null;
      return false;
    }
  }

  function onBinary(buf) {
    const tag = buf[0], payload = buf.subarray(1);
    if (tag === 1) { desc = payload.slice(); closeDecoder(); return; }
    if (tag !== 2 && tag !== 3) return;
    if (!ensureDecoder()) return;
    // The first chunk after configure() must be a key frame.
    if (needKey && tag !== 2) return;
    if (tag === 2) needKey = false;
    try {
      decoder.decode(new EncodedVideoChunk({ type: tag === 2 ? 'key' : 'delta', timestamp: (seq++) * 1000, data: payload }));
    } catch (e) {}
  }

  function connect() {
    if (!cfg) return;
    setStatus('connecting');
    const sock = new WebSocket(cfg.streamUrl);
    sock.binaryType = 'arraybuffer';
    ws = sock;
    sock.onopen = () => setStatus('waiting for video');
    sock.onmessage = (ev) => {
      if (typeof ev.data === 'string') {
        let m; try { m = JSON.parse(ev.data); } catch (e) { return; }
        if (m.type === 'meta') {
          streamW = m.width; streamH = m.height;
          if (m.codec && m.codec !== codec) { codec = m.codec; closeDecoder(); }
        }
        return;
      }
      onBinary(new Uint8Array(ev.data));
    };
    sock.onclose = () => {
      if (ws !== sock) return;
      ws = null; closeDecoder(); gotFrame = false;
      setStatus('disconnected', true);
      // Session ended (device shut down / slow viewer): re-attach shortly.
      if (cfg) retry = setTimeout(() => { retry = null; connect(); }, 2000);
    };
  }

  function send(o) { if (ws && ws.readyState === 1) ws.send(JSON.stringify(o)); }

  function toStream(e) {
    const r = canvas.getBoundingClientRect();
    const x = Math.round((e.clientX - r.left) / Math.max(1, r.width) * streamW);
    const y = Math.round((e.clientY - r.top) / Math.max(1, r.height) * streamH);
    return { x: Math.max(0, Math.min(streamW, x)), y: Math.max(0, Math.min(streamH, y)) };
  }

  let down = false;
  canvas.addEventListener('dragstart', (e) => e.preventDefault());
  canvas.addEventListener('pointerdown', (e) => {
    if (!ws || !streamW) return;
    e.preventDefault();
    try { canvas.setPointerCapture(e.pointerId); } catch (_) {}
    down = true;
    const p = toStream(e);
    send({ type: 'touch', action: 'down', x: p.x, y: p.y });
  });
  canvas.addEventListener('pointermove', (e) => {
    if (!down) return;
    const p = toStream(e);
    send({ type: 'touch', action: 'move', x: p.x, y: p.y });
  });
  const release = (e) => {
    if (!down) return;
    down = false;
    const p = toStream(e);
    send({ type: 'touch', action: 'up', x: p.x, y: p.y });
    try { canvas.releasePointerCapture(e.pointerId); } catch (_) {}
  };
  canvas.addEventListener('pointerup', release);
  canvas.addEventListener('pointercancel', release);
  canvas.addEventListener('wheel', (e) => {
    e.preventDefault();
    if (!ws || !streamW) return;
    const p = toStream(e);
    send({ type: 'scroll', x: p.x, y: p.y, dx: 0, dy: -Math.sign(e.deltaY) * 16 });
  }, { passive: false });
  window.addEventListener('keydown', (e) => {
    if (!ws) return;
    const map = { Enter: 66, Backspace: 67, Escape: 4, Home: 3 };
    if (e.key in map) { e.preventDefault(); send({ type: 'key', code: map[e.key] }); }
    else if (e.key.length === 1 && !e.metaKey && !e.ctrlKey) { e.preventDefault(); send({ type: 'text', text: e.key }); }
  });

  window.__consoleDevice = {
    start: (json) => {
      stopAll();
      try { cfg = JSON.parse(json); } catch (e) { cfg = null; setStatus('bad config', true); return; }
      if (!cfg || !cfg.streamUrl) { cfg = null; setStatus('no device', true); return; }
      connect();
    },
    stop: () => { cfg = null; stopAll(); setStatus('idle'); },
    button: (name) => send({ type: 'button', button: name }),
  };

  setStatus('ready');
})();
</script>
</body>
</html>
"#;

/// Config passed to `window.__consoleDevice.start(...)`.
#[derive(Clone, Debug)]
pub struct PlayerConfig {
    /// Fully-qualified `ws://` / `wss://` stream URL.
    pub stream_url: String,
}

impl PlayerConfig {
    /// Build the stream URL from the HTTP backend origin.
    pub fn for_device(base: &str, id: &str, platform: &str) -> Self {
        let base = base.trim_end_matches('/');
        let ws_base = if let Some(rest) = base.strip_prefix("https://") {
            format!("wss://{rest}")
        } else if let Some(rest) = base.strip_prefix("http://") {
            format!("ws://{rest}")
        } else {
            base.to_string()
        };
        Self {
            stream_url: format!(
                "{ws_base}/api/devices/{}/stream?platform={}",
                urlencoding_component(id),
                urlencoding_component(platform)
            ),
        }
    }

    pub fn start_script(&self) -> String {
        let config = serde_json::json!({ "streamUrl": self.stream_url }).to_string();
        format!(
            "window.__consoleDevice && window.__consoleDevice.start({})",
            serde_json::to_string(&config).unwrap_or_else(|_| "\"{}\"".to_string())
        )
    }

    pub fn stop_script() -> &'static str {
        "window.__consoleDevice && window.__consoleDevice.stop()"
    }

    /// Press a hardware button over the live socket (home, back, power, ...).
    pub fn button_script(name: &str) -> String {
        format!(
            "window.__consoleDevice && window.__consoleDevice.button({})",
            serde_json::to_string(name).unwrap_or_else(|_| "\"\"".to_string())
        )
    }
}

fn urlencoding_component(value: &str) -> String {
    let mut out = String::with_capacity(value.len());
    for byte in value.bytes() {
        match byte {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                out.push(byte as char)
            }
            _ => out.push_str(&format!("%{byte:02X}")),
        }
    }
    out
}
