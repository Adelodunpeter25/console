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
  let hoveredEl = null;
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

  function getSelector(el) {
    if (!el || el.nodeType !== Node.ELEMENT_NODE) return '';
    if (el.id) return `#${el.id}`;
    let path = [];
    while (el && el.nodeType === Node.ELEMENT_NODE && el !== document.documentElement && el !== document.body) {
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
      path.unshift(selector);
      el = el.parentElement;
      if (path.length >= 3) break;
    }
    return path.join(' > ');
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
    const aria = el.getAttribute('aria-label');
    if (aria && aria.trim()) return clean(aria);
    const by = el.getAttribute('aria-labelledby');
    if (by) {
      const text = by.split(/\s+/)
        .map((id) => document.getElementById(id))
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
    if (isPrivate(el)) return null;
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
      const cs = getComputedStyle(el);
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

  function updateOverlay(el) {
    if (!el || !el.isConnected) {
      hideOverlay();
      return;
    }
    const rect = el.getBoundingClientRect();
    const offscreen = rect.bottom <= 0 || rect.right <= 0
      || rect.top >= window.innerHeight || rect.left >= window.innerWidth;
    if ((rect.width === 0 && rect.height === 0) || offscreen) {
      hideOverlay();
      return;
    }

    const { overlay, label } = getOrCreateOverlay();
    overlay.style.display = 'block';
    overlay.style.top = `${rect.top}px`;
    overlay.style.left = `${rect.left}px`;
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

    // Position label inside if close to top edge
    if (rect.top < 26) {
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
    if (lastX !== null && lastY !== null) {
      const target = document.elementFromPoint(lastX, lastY);
      if (target && !isInspectorNode(target)) hoveredEl = target;
      else if (!target) hoveredEl = null;
    }
    updateOverlay(hoveredEl);
  }

  function scheduleRefresh() {
    if (!frame) frame = requestAnimationFrame(refresh);
  }

  function onPointerMove(e) {
    lastX = e.clientX;
    lastY = e.clientY;
    scheduleRefresh();
  }

  function onPointerLeave(e) {
    if (e.relatedTarget) return;
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
    // Pick on click only, and only for the primary button.
    if (e.type !== 'click' || e.button !== 0) return;

    const el = document.elementFromPoint(e.clientX, e.clientY) || hoveredEl;
    if (!el || isInspectorNode(el)) return;
    hoveredEl = el;
    updateOverlay(el);

    const rect = el.getBoundingClientRect();
    const { componentName, sourceLocation } = extractComponentMetadata(el);
    post({
      type: 'element_inspected',
      componentName: componentName || null,
      sourceLocation: sourceLocation || null,
      selector: getSelector(el),
      htmlSnippet: isPrivate(el) ? `<${el.tagName.toLowerCase()} …private…>` : (el.outerHTML ? el.outerHTML.slice(0, 1000) : ''),
      role: getRole(el),
      accessibleName: getAccessibleName(el),
      computedStyles: getComputedStyles(el),
      url: window.location.href,
      title: document.title,
      bounds: { x: rect.x, y: rect.y, width: rect.width, height: rect.height },
    });
    // Inspect mode stays on so several elements can be picked in a row.
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

  window.__consoleSetInspectMode = setInspectActive;
})();
