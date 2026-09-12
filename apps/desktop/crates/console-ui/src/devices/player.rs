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
  canvas, #stream-img { max-width: 100%; max-height: 100%; object-fit: contain; display: block; -webkit-user-drag: none; user-select: none; -webkit-user-select: none; touch-action: none; }
  #stream-img { display: none; }
  #status { display: none; position: absolute; left: 8px; bottom: 8px; font: 11px/1.4 -apple-system, system-ui, sans-serif; color: rgba(255,255,255,.9); background: rgba(180,30,30,.85); padding: 4px 8px; border-radius: 6px; pointer-events: none; }
</style>
</head>
<body>
<div id="stage">
  <canvas id="screen"></canvas>
  <img id="stream-img" draggable="false" alt="device screen stream" />
  <div id="status"></div>
</div>
<script>
(function () {
  const status = document.getElementById('status');
  const canvas = document.getElementById('screen');
  const streamImg = document.getElementById('stream-img');
  const ctx = canvas.getContext('2d');
  let cfg = null;
  let ws = null;
  let decoder = null;
  let frameCount = 0;

  const setStatus = (s) => {
    status.textContent = s;
    if (s && s.toLowerCase().includes('error')) {
      status.style.display = 'block';
    } else {
      status.style.display = 'none';
    }
    report({ type: 'status', status: s });
  };
  const report = (msg) => { try { window.ipc.postMessage(JSON.stringify(msg)); } catch (e) {} };

  function stopAll() {
    try { if (ws) ws.close(); } catch (e) {}
    ws = null;
    try { if (decoder && decoder.state !== 'closed') decoder.close(); } catch (e) {}
    decoder = null;
    try { streamImg.src = ''; } catch (e) {}
  }

  function startMjpegStream() {
    setStatus('connecting');
    canvas.style.display = 'none';
    streamImg.style.display = 'block';
    streamImg.src = cfg.streamUrl;
    streamImg.onload = () => {
      setStatus('streaming');
      frameCount++;
      report({ type: 'frame', count: frameCount });
    };
    streamImg.onerror = () => {
      setStatus('stream error');
    };
  }

  function buildAvcc(spsNal, ppsNal) {
    const spsLen = spsNal.length;
    const ppsLen = ppsNal.length;
    const avcc = new Uint8Array(11 + spsLen + ppsLen);
    avcc[0] = 1;
    avcc[1] = spsNal[1];
    avcc[2] = spsNal[2];
    avcc[3] = spsNal[3];
    avcc[4] = 0xff;
    avcc[5] = 0xe1;
    avcc[6] = (spsLen >> 8) & 0xff;
    avcc[7] = spsLen & 0xff;
    avcc.set(spsNal, 8);
    const ppsOffset = 8 + spsLen;
    avcc[ppsOffset] = 1;
    avcc[ppsOffset + 1] = (ppsLen >> 8) & 0xff;
    avcc[ppsOffset + 2] = ppsLen & 0xff;
    avcc.set(ppsNal, ppsOffset + 3);
    return avcc;
  }

  async function startH264Stream() {
    setStatus('connecting');
    streamImg.style.display = 'none';
    canvas.style.display = 'block';

    let sps = null;
    let pps = null;
    let configured = false;

    if (!('VideoDecoder' in window)) {
      setStatus('WebCodecs not supported in webview');
      return;
    }

    decoder = new VideoDecoder({
      output: (frame) => {
        if (canvas.width !== frame.displayWidth || canvas.height !== frame.displayHeight) {
          canvas.width = frame.displayWidth;
          canvas.height = frame.displayHeight;
        }
        ctx.drawImage(frame, 0, 0);
        frame.close();
        frameCount++;
        if (frameCount === 1) setStatus('streaming');
        report({ type: 'frame', count: frameCount });
      },
      error: (e) => setStatus('decoder error: ' + (e && e.message || e)),
    });

    try {
      const res = await fetch(cfg.streamUrl, { cache: 'no-store' });
      if (!res.ok || !res.body) throw new Error('http ' + res.status);
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
          for (let i = 0; i <= buf.length - 4; i++) {
            if (buf[i] === 0 && buf[i+1] === 0 && buf[i+2] === 0 && buf[i+3] === 1) {
              start = i;
              prefixLen = 4;
              break;
            } else if (buf[i] === 0 && buf[i+1] === 0 && buf[i+2] === 1) {
              start = i;
              prefixLen = 3;
              break;
            }
          }
          if (start === -1) break;

          let nextStart = -1;
          for (let i = start + prefixLen; i <= buf.length - 4; i++) {
            if ((buf[i] === 0 && buf[i+1] === 0 && buf[i+2] === 0 && buf[i+3] === 1) ||
                (buf[i] === 0 && buf[i+1] === 0 && buf[i+2] === 1)) {
              nextStart = i;
              break;
            }
          }

          if (nextStart === -1) {
            if (start > 0) buf = buf.slice(start);
            break;
          }

          const rawNal = buf.slice(start + prefixLen, nextStart);
          buf = buf.slice(nextStart);

          const nalType = rawNal[0] & 0x1f;
          if (nalType === 7) {
            sps = rawNal;
            continue;
          }
          if (nalType === 8) {
            pps = rawNal;
            continue;
          }

          if (!configured) {
            if (sps && pps) {
              const profile = sps[1].toString(16).padStart(2, '0');
              const compat = sps[2].toString(16).padStart(2, '0');
              const level = sps[3].toString(16).padStart(2, '0');
              const codec = `avc1.${profile}${compat}${level}`;
              const avcc = buildAvcc(sps, pps);
              decoder.configure({
                codec: codec,
                description: avcc.buffer,
                optimizeForLatency: true,
              });
              configured = true;
            } else {
              continue;
            }
          }

          if (!configured || decoder.state !== 'configured') continue;

          const isKey = nalType === 5;
          const frameData = new Uint8Array(4 + rawNal.length);
          frameData[0] = (rawNal.length >> 24) & 0xff;
          frameData[1] = (rawNal.length >> 16) & 0xff;
          frameData[2] = (rawNal.length >> 8) & 0xff;
          frameData[3] = rawNal.length & 0xff;
          frameData.set(rawNal, 4);

          try {
            decoder.decode(new EncodedVideoChunk({
              type: isKey ? 'key' : 'delta',
              timestamp: performance.now() * 1000,
              data: frameData.buffer,
            }));
          } catch (e) {}
        }
      }
    } catch (e) {
      setStatus('stream error: ' + (e && e.message || e));
    }
  }

  window.__consoleDevice = {
    start: (json) => {
      stopAll();
      frameCount = 0;
      try { cfg = JSON.parse(json); } catch (e) { setStatus('bad config'); return; }
      if (!cfg || !cfg.platform) { setStatus('no device'); return; }
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
