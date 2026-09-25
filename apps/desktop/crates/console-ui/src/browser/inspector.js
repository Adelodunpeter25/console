// Injected script for Console element inspection and annotation.
(function() {
  function getOrCreateOverlay() {
    let overlay = document.getElementById('__console_inspector_overlay');
    let label = document.getElementById('__console_inspector_label');
    if (!overlay) {
      overlay = document.createElement('div');
      overlay.id = '__console_inspector_overlay';
      overlay.style.cssText = 'position:fixed!important;pointer-events:none!important;z-index:2147483647!important;border:2px solid #007AFF!important;background-color:rgba(0,122,255,0.2)!important;border-radius:3px!important;display:none;box-sizing:border-box!important;margin:0!important;padding:0!important;transform:none!important;';

      label = document.createElement('div');
      label.id = '__console_inspector_label';
      label.style.cssText = 'position:absolute!important;top:-24px;left:0;background-color:#007AFF!important;color:#FFFFFF!important;font-size:11px!important;font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace!important;padding:2px 6px!important;border-radius:3px!important;white-space:nowrap!important;pointer-events:none!important;box-shadow:0 2px 4px rgba(0,0,0,0.25)!important;line-height:14px!important;height:auto!important;max-width:none!important;visibility:visible!important;opacity:1!important;';

      overlay.appendChild(label);
      (document.body || document.documentElement).appendChild(overlay);
    }
    return { overlay, label };
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
      const dataLoc = el.closest('[data-source-loc]')?.getAttribute('data-source-loc')
        || el.closest('[data-inspector-line]')?.getAttribute('data-inspector-line');
      if (dataLoc) sourceLocation = dataLoc;
    }

    return { componentName, sourceLocation };
  }

  function updateOverlay(el) {
    const { overlay, label } = getOrCreateOverlay();
    if (!el || !overlay) return;
    const rect = el.getBoundingClientRect();
    if (rect.width === 0 && rect.height === 0) return;

    overlay.style.display = 'block';
    overlay.style.top = `${rect.top}px`;
    overlay.style.left = `${rect.left}px`;
    overlay.style.width = `${rect.width}px`;
    overlay.style.height = `${rect.height}px`;

    const { componentName } = extractComponentMetadata(el);
    const tag = el.tagName.toLowerCase();
    const id = el.id ? `#${el.id}` : '';
    const dim = `${Math.round(rect.width)}×${Math.round(rect.height)}`;

    if (label) {
      label.textContent = componentName 
        ? `<${componentName} /> (${dim})`
        : `${tag}${id} (${dim})`;
      
      // Position label inside if close to top edge
      if (rect.top < 26) {
        label.style.top = '2px';
        label.style.left = '2px';
      } else {
        label.style.top = '-24px';
        label.style.left = '0';
      }
    }
  }

  let hoveredEl = null;

  function onMouseMove(e) {
    const { overlay, label } = getOrCreateOverlay();
    const target = document.elementFromPoint(e.clientX, e.clientY);
    if (!target || target === overlay || target === label || (overlay && overlay.contains(target))) return;
    hoveredEl = target;
    updateOverlay(hoveredEl);
  }

  function onClick(e) {
    e.preventDefault();
    e.stopPropagation();

    if (!hoveredEl) {
      hoveredEl = document.elementFromPoint(e.clientX, e.clientY);
    }
    if (!hoveredEl) return;

    const el = hoveredEl;
    const rect = el.getBoundingClientRect();
    const { componentName, sourceLocation } = extractComponentMetadata(el);
    const selector = getSelector(el);
    const htmlSnippet = el.outerHTML ? el.outerHTML.slice(0, 1000) : '';

    const payload = {
      type: 'element_inspected',
      componentName: componentName || null,
      sourceLocation: sourceLocation || null,
      selector: selector,
      htmlSnippet: htmlSnippet,
      url: window.location.href,
      title: document.title,
      bounds: {
        x: rect.x,
        y: rect.y,
        width: rect.width,
        height: rect.height
      }
    };

    if (window.ipc && window.ipc.postMessage) {
      window.ipc.postMessage(JSON.stringify(payload));
    }

    setInspectActive(false);
  }

  function onKeyDown(e) {
    if (e.key === 'Escape' || e.keyCode === 27) {
      setInspectActive(false);
      if (window.ipc && window.ipc.postMessage) {
        window.ipc.postMessage(JSON.stringify({ type: 'inspect_cancelled' }));
      }
    }
  }

  function setInspectActive(enable) {
    const { overlay } = getOrCreateOverlay();
    document.removeEventListener('mousemove', onMouseMove, true);
    document.removeEventListener('click', onClick, true);
    document.removeEventListener('keydown', onKeyDown, true);
    window.removeEventListener('mousemove', onMouseMove, true);
    window.removeEventListener('click', onClick, true);
    window.removeEventListener('keydown', onKeyDown, true);

    if (!enable) {
      if (overlay) overlay.style.display = 'none';
      hoveredEl = null;
      if (document.body) {
        document.body.style.cursor = '';
      }
    } else {
      if (document.body) {
        document.body.style.cursor = 'crosshair';
      }
      window.addEventListener('mousemove', onMouseMove, true);
      window.addEventListener('click', onClick, true);
      window.addEventListener('keydown', onKeyDown, true);
    }
  }

  window.__consoleSetInspectMode = setInspectActive;
})();
