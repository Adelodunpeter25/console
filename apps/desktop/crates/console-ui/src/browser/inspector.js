// Injected script for Console element inspection and annotation.
(function() {
  if (window.__consoleSetInspectMode) return;

  const OVERLAY_ID = '__console_inspector_overlay';
  const LABEL_ID = '__console_inspector_label';
  const STYLE_ID = '__console_inspector_style';

  // Pointer/mouse events the page must not see while inspecting, so clicking
  // an element never triggers links, buttons, focus changes or app handlers.
  const BLOCKED_EVENTS = [
    'pointerdown', 'pointerup', 'mousedown', 'mouseup',
    'click', 'dblclick', 'auxclick', 'contextmenu', 'touchstart', 'touchend',
  ];

  let active = false;
  let paused = false;
  let hoveredEl = null;
  let hoveredOffset = { x: 0, y: 0 };
  let lastX = null;
  let lastY = null;
  let frame = 0;

  function post(payload) {
    if (window.ipc && window.ipc.postMessage) {
      window.ipc.postMessage(JSON.stringify(payload));
    }
  }

  function getOrCreateOverlay() {
    let overlay = document.getElementById(OVERLAY_ID);
    let label = document.getElementById(LABEL_ID);
    if (!overlay || !overlay.isConnected) {
      overlay = document.createElement('div');
      overlay.id = OVERLAY_ID;
      overlay.style.cssText = 'position:fixed!important;pointer-events:none!important;z-index:2147483647!important;border:2px solid #007AFF!important;background-color:rgba(0,122,255,0.2)!important;border-radius:3px!important;display:none;box-sizing:border-box!important;margin:0!important;padding:0!important;transform:none!important;';

      label = document.createElement('div');
      label.id = LABEL_ID;
      label.style.cssText = 'position:absolute!important;top:-24px;left:0;background-color:#007AFF!important;color:#FFFFFF!important;font-size:11px!important;font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace!important;padding:2px 6px!important;border-radius:3px!important;white-space:nowrap!important;pointer-events:none!important;box-shadow:0 2px 4px rgba(0,0,0,0.25)!important;line-height:14px!important;height:auto!important;max-width:none!important;visibility:visible!important;opacity:1!important;';

      overlay.appendChild(label);
      (document.body || document.documentElement).appendChild(overlay);
    }
    return { overlay, label };
  }

  function hideOverlay() {
    const overlay = document.getElementById(OVERLAY_ID);
    if (overlay) overlay.style.display = 'none';
  }

  function setCursorStyle(enable) {
    let style = document.getElementById(STYLE_ID);
    if (!enable) {
      if (style) style.remove();
      return;
    }
    if (!style) {
      style = document.createElement('style');
      style.id = STYLE_ID;
      style.textContent = '*{cursor:crosshair!important;}';
      (document.head || document.documentElement).appendChild(style);
    }
  }

  function isInspectorNode(el) {
    return !!el && (el.id === OVERLAY_ID || el.id === LABEL_ID);
  }

  function segmentFor(el) {
    let selector = el.tagName.toLowerCase();
    if (el.className && typeof el.className === 'string') {
      const classes = el.className.trim().split(/\s+/).filter(c => c && !c.startsWith('__console'));
      if (classes.length > 0) {
        selector += '.' + classes.slice(0, 2).join('.');
      }
    }
    let sibling = el;
    let nth = 1;
    while ((sibling = sibling.previousElementSibling)) {
      if (sibling.tagName.toLowerCase() === el.tagName.toLowerCase()) nth++;
    }
    if (nth > 1) selector += `:nth-of-type(${nth})`;
    return selector;
  }

  function getSelector(el) {
    if (!el || el.nodeType !== Node.ELEMENT_NODE) return '';
    if (el.id) return `#${el.id}`;
    let path = [];
    let cur = el;
    let guard = 0;
    while (cur && cur.nodeType === Node.ELEMENT_NODE && guard < 12) {
      guard++;
      const doc = cur.ownerDocument || document;
      if (cur === doc.documentElement || cur === doc.body) break;
      path.unshift(segmentFor(cur));
      if (path.length >= 4) break;
      let parent = cur.parentElement;
      if (!parent) {
        const root = cur.getRootNode && cur.getRootNode();
        if (root && root.host) {
          parent = root.host;
          path.unshift(segmentFor(parent));
          cur = parent.parentElement || parent.getRootNode && parent.getRootNode().host || null;
          if (!cur || cur === doc.documentElement) break;
          continue;
        }
        break;
      }
      cur = parent;
      if (cur === doc.documentElement || cur === doc.body) break;
    }
    let inner = path.join(' > ');
    // Cross a frame boundary: prefix the innermost iframe's selector.
    try {
      const doc = el.ownerDocument || document;
      if (doc !== document) {
        const frames = Array.from(document.querySelectorAll('iframe'));
        for (const f of frames) {
          try { if (f.contentDocument === doc) { inner = segmentFor(f) + ' > ' + inner; break; } } catch (e) {}
        }
      }
    } catch (e) {}
    return inner;
  }

  function shadowDeepest(el) {
    let cur = el;
    let guard = 0;
    while (cur && cur.shadowRoot && guard < 6) {
      guard++;
      const inner = cur.shadowRoot.elementFromPoint(lastX, lastY);
      if (!inner) break;
      cur = inner;
    }
    return cur;
  }

  function pickElement(x, y) {
    if (x === null || y === null) return null;
    let best = null;
    let bestOffset = { x: 0, y: 0 };
    // Same-origin iframes first: they win over the top document hit.
    try {
      const frames = Array.from(document.querySelectorAll('iframe'));
      for (const f of frames) {
        let rect;
        try { rect = f.getBoundingClientRect(); } catch (e) { continue; }
        if (x < rect.left || x >= rect.right || y < rect.top || y >= rect.bottom) continue;
        let innerDoc;
        try { innerDoc = f.contentDocument; } catch (e) { continue; }
        if (!innerDoc) continue;
        let innerEl = null;
        try { innerEl = innerDoc.elementFromPoint(x - rect.left, y - rect.top); } catch (e) { continue; }
        if (!innerEl || isInspectorNode(innerEl)) continue;
        if (innerEl.shadowRoot) {
          try {
            const deep = innerEl.shadowRoot.elementFromPoint(x - rect.left, y - rect.top);
            if (deep) innerEl = deep;
          } catch (e) {}
        }
        best = innerEl;
        bestOffset = { x: rect.left, y: rect.top };
      }
    } catch (e) {}
    if (best) return { el: best, offset: bestOffset };
    let el = null;
    try { el = document.elementFromPoint(x, y); } catch (e) { return null; }
    if (!el || isInspectorNode(el)) return null;
    if (el.shadowRoot) {
      try {
        const deep = el.shadowRoot.elementFromPoint(x, y);
        if (deep) el = deep;
      } catch (e) {}
    }
    return { el, offset: { x: 0, y: 0 } };
  }

  function isPrivateTree(el) {
    try {
      if (!el || !el.matches) return false;
      if (el.matches('input[type="password"], [data-private]')) return true;
      if (el.closest && el.closest('input[type="password"], [data-private]')) return true;
      if (el.querySelector && el.querySelector('input[type="password"], [data-private]')) return true;
      return passedPrivateAncestor(el);
    } catch (e) { return false; }
  }

  function passedPrivateAncestor(el) {
    try {
      let cur = el.parentElement;
      while (cur) {
        if (cur.matches && cur.matches('[data-private]')) return true;
        cur = cur.parentElement;
      }
    } catch (e) {}
    return false;
  }

  function sanitizeSnippet(el) {
    if (isPrivateTree(el)) {
      return `<${el.tagName.toLowerCase()} …private…>`;
    }
    try {
      const clone = el.cloneNode(true);
      clone.querySelectorAll('input[type="password"], [data-private]').forEach(n => {
        try { n.textContent = '[private]'; n.removeAttribute('value'); } catch (e) {}
      });
      clone.querySelectorAll('input, textarea').forEach(n => {
        try { n.removeAttribute('value'); } catch (e) {}
      });
      if (clone.tagName && clone.tagName.toLowerCase() === 'input') {
        try { clone.removeAttribute('value'); } catch (e) {}
      }
      return (clone.outerHTML || '').slice(0, 1000);
    } catch (e) {
      return (el.outerHTML ? el.outerHTML.slice(0, 1000) : '');
    }
  }

  function extractComponentMetadata(el) {
    let componentName = null;
    let sourceLocation = null;

    // React Fiber traversal
    try {
      const fiberKey = Object.keys(el).find(k => k.startsWith('__reactFiber$') || k.startsWith('__reactInternalInstance$'));
      if (fiberKey) {
        let fiber = el[fiberKey];
        while (fiber) {
          if (fiber.type && (typeof fiber.type === 'function' || typeof fiber.type === 'object')) {
            const name = fiber.type.displayName || fiber.type.name;
            const source = fiber._debugSource;
            if (name && !name.startsWith('_') && name !== 'Anonymous') {
              if (!componentName) componentName = name;
              if (source) {
                sourceLocation = `${source.fileName}:${source.lineNumber}:${source.columnNumber}`;
                break;
              }
            }
          }
          fiber = fiber.return;
        }
      }
    } catch (e) {}

    // Vite / Next.js / user data locator attributes
    if (!sourceLocation) {
      try {
        const dataLoc = el.closest('[data-source-loc]')?.getAttribute('data-source-loc')
          || el.closest('[data-inspector-line]')?.getAttribute('data-inspector-line');
        if (dataLoc) sourceLocation = dataLoc;
      } catch (e) {}
    }

    return { componentName, sourceLocation };
  }

  const IMPLICIT_ROLES = {
    a: (el) => (el.hasAttribute('href') ? 'link' : null),
    button: () => 'button', nav: () => 'navigation', main: () => 'main',
    header: () => 'banner', footer: () => 'contentinfo', aside: () => 'complementary',
    form: () => 'form', ul: () => 'list', ol: () => 'list', li: () => 'listitem',
    table: () => 'table', img: (el) => (el.getAttribute('alt') === '' ? 'presentation' : 'img'),
    select: () => 'combobox', textarea: () => 'textbox', dialog: () => 'dialog',
    h1: () => 'heading', h2: () => 'heading', h3: () => 'heading',
    h4: () => 'heading', h5: () => 'heading', h6: () => 'heading',
    input: (el) => {
      const t = (el.getAttribute('type') || 'text').toLowerCase();
      if (['button', 'submit', 'reset', 'image'].includes(t)) return 'button';
      if (t === 'checkbox') return 'checkbox';
      if (t === 'radio') return 'radio';
      if (t === 'range') return 'slider';
      if (t === 'search') return 'searchbox';
      if (t === 'hidden') return null;
      return 'textbox';
    },
  };

  function getRole(el) {
    const explicit = el.getAttribute('role');
    if (explicit) return explicit.trim().split(/\s+/)[0];
    const fn = IMPLICIT_ROLES[el.tagName.toLowerCase()];
    return fn ? fn(el) : null;
  }

  function isPrivate(el) {
    return el.matches && el.matches('input[type="password"], [data-private]');
  }

  function getAccessibleName(el) {
    const clean = (t) => (t || '').replace(/\s+/g, ' ').trim().slice(0, 120);
    const doc = el.ownerDocument || document;
    const aria = el.getAttribute('aria-label');
    if (aria && aria.trim()) return clean(aria);
    const by = el.getAttribute('aria-labelledby');
    if (by) {
      const text = by.split(/\s+/)
        .map((id) => { try { return doc.getElementById(id); } catch (e) { return null; } })
        .filter(Boolean)
        .map((n) => n.textContent)
        .join(' ');
      if (clean(text)) return clean(text);
    }
    if (el.labels && el.labels.length) {
      const text = Array.from(el.labels).map((l) => l.textContent).join(' ');
      if (clean(text)) return clean(text);
    }
    const alt = el.getAttribute('alt');
    if (alt && alt.trim()) return clean(alt);
    if (isPrivate(el) || isPrivateTree(el)) return null;
    const title = el.getAttribute('title');
    const text = clean(el.innerText || el.textContent);
    return text || clean(title) || clean(el.getAttribute('placeholder')) || null;
  }

  const STYLE_PROPS = [
    'display', 'position', 'width', 'height', 'margin', 'padding',
    'color', 'background-color', 'font-family', 'font-size', 'font-weight',
    'line-height', 'border', 'border-radius', 'gap', 'flex-direction',
    'justify-content', 'align-items', 'grid-template-columns', 'opacity', 'z-index',
  ];

  function getComputedStyles(el) {
    const out = {};
    try {
      const win = (el.ownerDocument && el.ownerDocument.defaultView) || window;
      const cs = win.getComputedStyle(el);
      for (const prop of STYLE_PROPS) {
        const v = cs.getPropertyValue(prop);
        if (!v || v === 'none' || v === 'normal' || v === 'auto' || v === '0px') continue;
        out[prop] = v;
      }
    } catch (e) {}
    return out;
  }

  function classSummary(el) {
    if (!el.className || typeof el.className !== 'string') return '';
    const classes = el.className.trim().split(/\s+/).filter((c) => c && !c.startsWith('__console'));
    if (!classes.length) return '';
    const shown = classes.slice(0, 3).map((c) => '.' + c).join('');
    return classes.length > 3 ? `${shown}…` : shown;
  }

  function updateOverlay(el, offset) {
    if (!el || !el.isConnected) {
      hideOverlay();
      return;
    }
    const off = offset || { x: 0, y: 0 };
    const rect = el.getBoundingClientRect();
    const top = rect.top + off.y;
    const left = rect.left + off.x;
    const bottom = rect.bottom + off.y;
    const right = rect.right + off.x;
    const offscreen = bottom <= 0 || right <= 0
      || top >= window.innerHeight || left >= window.innerWidth;
    if ((rect.width === 0 && rect.height === 0) || offscreen) {
      hideOverlay();
      return;
    }

    const { overlay, label } = getOrCreateOverlay();
    overlay.style.display = 'block';
    overlay.style.top = `${top}px`;
    overlay.style.left = `${left}px`;
    overlay.style.width = `${rect.width}px`;
    overlay.style.height = `${rect.height}px`;

    const { componentName } = extractComponentMetadata(el);
    const tag = el.tagName.toLowerCase();
    const id = el.id ? `#${el.id}` : '';
    const dim = `${Math.round(rect.width)}×${Math.round(rect.height)}`;

    const base = `${tag}${id}${classSummary(el)}`;
    label.textContent = componentName
      ? `${base} · <${componentName} /> (${dim})`
      : `${base} (${dim})`;

    if (top < 26) {
      label.style.top = '2px';
      label.style.left = '2px';
    } else {
      label.style.top = '-24px';
      label.style.left = '0';
    }
  }

  // Re-resolve the element under the last known pointer position. Used after
  // scroll/resize/DOM changes, where the pointer stays put but content moves.
  function refresh() {
    frame = 0;
    if (!active) return;
    if (paused) return;
    if (lastX !== null && lastY !== null) {
      const pick = pickElement(lastX, lastY);
      if (pick) {
        hoveredEl = pick.el;
        hoveredOffset = pick.offset;
      } else {
        hoveredEl = null;
        hoveredOffset = { x: 0, y: 0 };
      }
    }
    updateOverlay(hoveredEl, hoveredOffset);
  }

  function scheduleRefresh() {
    if (!frame) frame = requestAnimationFrame(refresh);
  }

  function onPointerMove(e) {
    lastX = e.clientX;
    lastY = e.clientY;
    if (paused) return;
    scheduleRefresh();
  }

  function onPointerLeave(e) {
    if (e.relatedTarget) return;
    if (paused) return;
    lastX = lastY = null;
    hoveredEl = null;
    hideOverlay();
  }

  function swallow(e) {
    e.preventDefault();
    e.stopPropagation();
    e.stopImmediatePropagation();
  }

  function onBlockedEvent(e) {
    swallow(e);
    if (paused) return;
    if (e.type !== 'click' || e.button !== 0) return;

    const pick = pickElement(e.clientX, e.clientY);
    const el = pick ? pick.el : hoveredEl;
    const offset = pick ? pick.offset : hoveredOffset;
    if (!el || isInspectorNode(el)) return;
    hoveredEl = el;
    hoveredOffset = offset;
    updateOverlay(el, offset);

    const rect = el.getBoundingClientRect();
    const { componentName, sourceLocation } = extractComponentMetadata(el);
    post({
      type: 'element_inspected',
      componentName: componentName || null,
      sourceLocation: sourceLocation || null,
      selector: getSelector(el),
      htmlSnippet: sanitizeSnippet(el),
      role: getRole(el),
      accessibleName: getAccessibleName(el),
      computedStyles: getComputedStyles(el),
      url: window.location.href,
      title: document.title,
      bounds: { x: rect.x + offset.x, y: rect.y + offset.y, width: rect.width, height: rect.height },
    });
  }

  function onKeyDown(e) {
    if (e.key === 'Escape' || e.keyCode === 27) {
      swallow(e);
      setInspectActive(false);
      post({ type: 'inspect_cancelled' });
    }
  }

  function setListeners(enable) {
    const method = enable ? 'addEventListener' : 'removeEventListener';
    const capture = { capture: true, passive: false };
    for (const type of BLOCKED_EVENTS) window[method](type, onBlockedEvent, capture);
    window[method]('pointermove', onPointerMove, true);
    window[method]('mousemove', onPointerMove, true);
    window[method]('mouseout', onPointerLeave, true);
    window[method]('keydown', onKeyDown, true);
    // Scroll does not bubble, so capture it on document to catch nested scrollers.
    document[method]('scroll', scheduleRefresh, { capture: true, passive: true });
    window[method]('resize', scheduleRefresh, true);
  }

  function setInspectActive(enable) {
    enable = !!enable;
    if (enable === active) return;
    active = enable;
    if (!enable) paused = false;
    setListeners(enable);
    setCursorStyle(enable);
    if (frame) {
      cancelAnimationFrame(frame);
      frame = 0;
    }
    if (!enable) {
      hoveredEl = null;
      hideOverlay();
    }
  }

  function setInspectPaused(enable) {
    enable = !!enable;
    if (enable === paused) return;
    paused = enable;
    if (paused && frame) {
      cancelAnimationFrame(frame);
      frame = 0;
    }
  }

  window.__consoleSetInspectMode = setInspectActive;
  window.__consoleSetInspectPaused = setInspectPaused;
})();
