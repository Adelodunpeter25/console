// Agent snapshot, click and type script. The request placeholder below is
// replaced in Rust with {op, ref, text, submit, selector}. Refs live on `window`, so a full page
// load drops them; in-page navigation is caught by the isConnected check.
(() => {
  const req = __REQUEST__;
  const MAX = 200;
  const STORE = '__consoleRefs';

  const clean = (t, n = 80) => {
    t = (t || '').replace(/\s+/g, ' ').trim();
    return t.length > n ? t.slice(0, n - 1) + '…' : t;
  };

  const IMPLICIT = {
    a: (el) => (el.hasAttribute('href') ? 'link' : null),
    button: () => 'button', summary: () => 'button', select: () => 'combobox',
    textarea: () => 'textbox',
    input: (el) => {
      const t = (el.getAttribute('type') || 'text').toLowerCase();
      if (['button', 'submit', 'reset', 'image'].includes(t)) return 'button';
      if (t === 'checkbox') return 'checkbox';
      if (t === 'radio') return 'radio';
      if (t === 'range') return 'slider';
      if (t === 'search') return 'searchbox';
      return 'textbox';
    },
  };
  const getRole = (el) => {
    const explicit = el.getAttribute('role');
    if (explicit) return explicit.trim().split(/\s+/)[0];
    const fn = IMPLICIT[el.tagName.toLowerCase()];
    const role = fn ? fn(el) : null;
    if (role) return role;
    return el.isContentEditable ? 'textbox' : el.tagName.toLowerCase();
  };

  const isPrivate = (el) => el.matches('input[type="password"], [data-private]');

  const getName = (el) => {
    const aria = el.getAttribute('aria-label');
    if (aria && aria.trim()) return clean(aria);
    const by = el.getAttribute('aria-labelledby');
    if (by) {
      const text = by.split(/\s+/).map((id) => document.getElementById(id))
        .filter(Boolean).map((n) => n.textContent).join(' ');
      if (clean(text)) return clean(text);
    }
    if (el.labels && el.labels.length) {
      const text = Array.from(el.labels).map((l) => l.textContent).join(' ');
      if (clean(text)) return clean(text);
    }
    const alt = el.getAttribute('alt');
    if (alt && alt.trim()) return clean(alt);
    if (isPrivate(el)) return clean(el.getAttribute('placeholder')) || null;
    const isField = el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT';
    const text = isField && el.tagName !== 'INPUT' ? '' : clean(el.innerText || el.textContent);
    return text || clean(el.getAttribute('title')) || clean(el.getAttribute('placeholder'))
      || (el.tagName === 'INPUT' && ['button', 'submit', 'reset'].includes(el.type) ? clean(el.value) : '')
      || null;
  };

  const SEL = 'a[href],button,input:not([type=hidden]),select,textarea,summary,'
    + '[contenteditable=""],[contenteditable="true"],'
    + '[role=button],[role=link],[role=checkbox],[role=radio],[role=tab],[role=menuitem],'
    + '[role=option],[role=switch],[role=combobox],[role=searchbox],[role=textbox],[role=slider]';

  const isVisible = (el) => {
    const r = el.getBoundingClientRect();
    if (r.width === 0 && r.height === 0) return false;
    const cs = getComputedStyle(el);
    return cs.visibility !== 'hidden' && cs.display !== 'none';
  };

  const describe = (id, el) => {
    const name = getName(el);
    let line = `[ref=${id}] ${getRole(el)}` + (name ? ` "${name}"` : '');
    const extra = [];
    if (el.tagName === 'INPUT' && el.type && !['button', 'submit', 'reset', 'image'].includes(el.type)) {
      extra.push('type=' + el.type);
    }
    if ('value' in el && el.tagName !== 'BUTTON' && !isPrivate(el)
        && !['button', 'submit', 'reset', 'checkbox', 'radio'].includes(el.type) && el.value) {
      extra.push('value="' + clean(el.value, 40) + '"');
    }
    if (el.tagName === 'A') extra.push('href=' + clean(el.getAttribute('href'), 60));
    if (el.checked) extra.push('checked');
    if (el.disabled) extra.push('disabled');
    return extra.length ? line + ' (' + extra.join(', ') + ')' : line;
  };

  const snapshot = () => {
    let roots = [document];
    if (req.selector) {
      try {
        roots = Array.from(document.querySelectorAll(req.selector));
      } catch (e) {
        return 'Invalid selector: ' + e.message;
      }
      if (!roots.length) return 'No match for selector: ' + req.selector;
    }
    // Scoped roots may nest or be interactive themselves; keep each element once.
    const found = new Set();
    for (const root of roots) {
      if (root !== document && root.matches(SEL)) found.add(root);
      root.querySelectorAll(SEL).forEach((el) => found.add(el));
    }
    const all = Array.from(found).filter(isVisible);
    const els = new Map();
    const lines = all.slice(0, MAX).map((el, i) => {
      const id = 'e' + (i + 1);
      els.set(id, el);
      return describe(id, el);
    });
    window[STORE] = { els };
    const head = `Interactive elements: ${all.length}` + (all.length > MAX ? ` (showing first ${MAX}; pass selector to scope)` : '');
    return [head, ...lines].join('\n');
  };

  if (req.op === 'snapshot') return snapshot();

  const store = window[STORE];
  const el = store && store.els.get(req.ref);
  if (!req.ref) return 'A ref is required. Take a snapshot first.';
  if (!el || !el.isConnected) {
    return `Stale or unknown ref "${req.ref}" (refs expire when the page changes). Fresh snapshot:\n` + snapshot();
  }
  const label = `${req.ref} ${getRole(el)}` + (getName(el) ? ` "${getName(el)}"` : '');
  if (el.disabled) return `Element ${label} is disabled.`;

  el.scrollIntoView({ block: 'center', inline: 'center' });

  if (req.op === 'click') {
    const r = el.getBoundingClientRect();
    const o = { bubbles: true, cancelable: true, view: window, button: 0,
      clientX: r.x + r.width / 2, clientY: r.y + r.height / 2 };
    el.dispatchEvent(new PointerEvent('pointerdown', o));
    el.dispatchEvent(new MouseEvent('mousedown', o));
    if (el.focus) el.focus();
    el.dispatchEvent(new PointerEvent('pointerup', o));
    el.dispatchEvent(new MouseEvent('mouseup', o));
    el.click();
    return `Clicked ${label}`;
  }

  if (req.op === 'type') {
    const text = req.text || '';
    if (el.tagName === 'SELECT') {
      const opt = Array.from(el.options).find((o) => o.value === text || clean(o.text, 200) === text);
      if (!opt) return `No option "${text}" in ${label}. Options: ` + Array.from(el.options).map((o) => clean(o.text, 40)).join(' | ');
      el.value = opt.value;
      el.dispatchEvent(new Event('input', { bubbles: true }));
      el.dispatchEvent(new Event('change', { bubbles: true }));
      return `Selected "${text}" in ${label}`;
    }
    const nonText = ['checkbox', 'radio', 'button', 'submit', 'reset', 'image', 'file', 'range'];
    if (el.tagName === 'INPUT' && nonText.includes(el.type)) return `${label} is not a text field; use click.`;
    if (!el.isContentEditable && !('value' in el)) return `${label} is not a text field.`;
    el.focus();
    if (el.isContentEditable) {
      document.execCommand('selectAll');
      if (!document.execCommand('insertText', false, text)) el.textContent = text;
    } else {
      const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
      Object.getOwnPropertyDescriptor(proto, 'value').set.call(el, text);
      el.dispatchEvent(new Event('input', { bubbles: true }));
      el.dispatchEvent(new Event('change', { bubbles: true }));
    }
    let out = `Typed into ${label}`;
    if (req.submit) {
      const k = { key: 'Enter', code: 'Enter', keyCode: 13, which: 13, bubbles: true, cancelable: true };
      const down = new KeyboardEvent('keydown', k);
      el.dispatchEvent(down);
      el.dispatchEvent(new KeyboardEvent('keyup', k));
      if (!down.defaultPrevented && el.form) {
        if (el.form.requestSubmit) el.form.requestSubmit(); else el.form.submit();
      }
      out += ' and pressed Enter';
    }
    return out;
  }

  return 'Unknown op: ' + req.op;
})()
