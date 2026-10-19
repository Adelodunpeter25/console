package com.console.mobile.feature.devices

/**
 * Virtual https origin the player page is served from (intercepted in the
 * WebViewClient, never hits the network). WebCodecs only exists in secure
 * contexts, and the server's own http:// address isn't one.
 */
internal const val DEVICES_PLAYER_URL = "https://appassets.androidplatform.net/devices/player.html"

/**
 * WebCodecs H.264 canvas player for sim-go stream WebSocket.
 * Identical contract to the desktop player; posts status to window.AndroidBridge.
 */
internal const val DEVICES_PLAYER_HTML = """<!doctype html>
<html>
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1, user-scalable=no" />
<style>
  html, body { margin: 0; padding: 0; height: 100%; width: 100%; background: #0a0a0b; overflow: hidden; user-select: none; -webkit-user-select: none; }
  #stage { position: relative; width: 100%; height: 100%; display: flex; align-items: center; justify-content: center; }
  canvas { max-width: 100%; max-height: 100%; width: auto; height: auto; object-fit: contain; display: block; touch-action: none; -webkit-user-drag: none; }
  #dbg { position: absolute; left: 6px; top: 6px; max-width: 94%; font: 10px/1.35 monospace; color: #7CFC00; background: rgba(0,0,0,.72); padding: 4px 6px; border-radius: 4px; pointer-events: none; white-space: pre-wrap; z-index: 5; }
  #status { display: none; position: absolute; left: 8px; bottom: 8px; font: 11px/1.4 -apple-system, sans-serif; color: rgba(255,255,255,.9); background: rgba(180,30,30,.85); padding: 4px 8px; border-radius: 6px; pointer-events: none; }
</style>
</head>
<body>
<div id="stage">
  <canvas id="screen"></canvas>
  <div id="status"></div>
  <div id="dbg"></div>
</div>
<script>
(function () {
  const statusEl = document.getElementById('status');
  const canvas = document.getElementById('screen');
  const ctx = canvas.getContext('2d');
  let cfg = null, ws = null, decoder = null;
  let streamW = 0, streamH = 0, codec = '', desc = null, needKey = true, seq = 0, gotFrame = false;
  let retry = null;

  // TEMPORARY debug readout (see dbg panel): which stage of the pipeline stalls.
  const dbgEl = document.getElementById('dbg');
  const dbg = { chunksIn: 0, keyIn: 0, decoded: 0, drawn: 0, decErr: '', cfg: '', cfgOk: '', lastFrame: '', px: '' };
  function sampleCanvas() {
    try {
      if (!canvas.width || !canvas.height) return 'canvas 0x0';
      const w = Math.min(canvas.width, 64), h = Math.min(canvas.height, 64);
      const d = ctx.getImageData(Math.floor((canvas.width - w) / 2), Math.floor((canvas.height - h) / 2), w, h).data;
      let lit = 0;
      for (let i = 0; i < d.length; i += 4) if (d[i] | d[i + 1] | d[i + 2]) lit++;
      return lit + '/' + (w * h) + ' lit';
    } catch (e) { return 'getImageData: ' + e.message; }
  }
  setInterval(() => {
    const r = canvas.getBoundingClientRect();
    const txt =
      'state=' + (decoder ? decoder.state : 'none') + ' q=' + (decoder ? decoder.decodeQueueSize : '-') + '\n' +
      'in=' + dbg.chunksIn + ' key=' + dbg.keyIn + ' decoded=' + dbg.decoded + ' drawn=' + dbg.drawn + '\n' +
      'cfg=' + dbg.cfg + ' ' + dbg.cfgOk + '\n' +
      'canvas=' + canvas.width + 'x' + canvas.height + ' css=' + Math.round(r.width) + 'x' + Math.round(r.height) + '\n' +
      'frame=' + dbg.lastFrame + ' px=' + dbg.px + '\n' +
      (dbg.decErr ? 'ERR ' + dbg.decErr : '');
    dbgEl.textContent = txt;
    report({ type: 'debug', text: txt });
  }, 500);

  const report = (msg) => {
    try {
      if (window.AndroidBridge && window.AndroidBridge.postMessage) {
        window.AndroidBridge.postMessage(JSON.stringify(msg));
      }
    } catch (e) {}
  };

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

  // A cold stream's meta can be sent before the device has reported its
  // encoder config, and then carries no codec at all. The avcC description
  // always arrives before the first key frame, so the WebCodecs codec string
  // can be derived from it (avc1.PPCCLL); without this the decoder is never
  // configured and the viewer sits at "waiting for video" forever.
  function codecFromAvcC(d) {
    // configurationVersion 1 marks an AVCDecoderConfigurationRecord.
    if (!d || d.length < 4 || d[0] !== 1) return '';
    const hex = (n) => n.toString(16).padStart(2, '0');
    return 'avc1.' + hex(d[1]) + hex(d[2]) + hex(d[3]);
  }

  function ensureDecoder() {
    if (decoder) return true;
    if (!codec || !desc) return false;
    if (typeof VideoDecoder === 'undefined') { setStatus('WebCodecs unavailable', true); return false; }
    try {
      decoder = new VideoDecoder({
        output(frame) {
          dbg.decoded++; dbg.lastFrame = frame.codedWidth + 'x' + frame.codedHeight + ' disp ' + frame.displayWidth + 'x' + frame.displayHeight;
          const targetW = frame.displayWidth || frame.codedWidth || streamW;
          const targetH = frame.displayHeight || frame.codedHeight || streamH;
          if (targetW && targetH && (canvas.width !== targetW || canvas.height !== targetH)) {
            canvas.width = targetW; canvas.height = targetH;
          }
          if (canvas.width && canvas.height) {
            ctx.drawImage(frame, 0, 0, canvas.width, canvas.height);
            dbg.drawn++;
            if (dbg.drawn === 1 || dbg.drawn % 30 === 0) dbg.px = sampleCanvas();
          }
          frame.close();
          if (!gotFrame) { gotFrame = true; setStatus('streaming'); }
        },
        error(e) {
          const errMsg = e && e.message ? e.message : String(e);
          console.error('VideoDecoder error:', errMsg);
          dbg.decErr = errMsg;
          closeDecoder();
          send({ type: 'reset' });
        },
      });
      dbg.cfg = codec + ' desc=' + desc.length + 'B'; dbg.cfgOk = '';
      decoder.configure({ codec, description: desc, optimizeForLatency: true });
      try { VideoDecoder.isConfigSupported({ codec, description: desc }).then((r) => { dbg.cfgOk = 'supported=' + r.supported; }).catch((e) => { dbg.cfgOk = 'isConfigSupported threw: ' + e.message; }); } catch (e) { dbg.cfgOk = 'no isConfigSupported'; }
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
    if (tag === 1) { desc = payload.slice(); if (!codec) codec = codecFromAvcC(desc); closeDecoder(); return; }
    if (tag !== 2 && tag !== 3) return;
    dbg.chunksIn++; if (tag === 2) dbg.keyIn++;
    if (!ensureDecoder()) return;
    if (needKey && tag !== 2) return;
    if (tag === 2) needKey = false;
    try {
      decoder.decode(new EncodedVideoChunk({ type: tag === 2 ? 'key' : 'delta', timestamp: (seq++) * 1000, data: payload }));
    } catch (e) {
      const errMsg = e && e.message ? e.message : String(e);
      console.error('decode chunk error:', errMsg);
    }
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
    if (!down || !ws) return;
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
"""
