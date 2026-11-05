// Engine-agnostic page driver: snapshot, refs, and interactions.
//
// Installed once per document at load time (`driver::init_script`), so calls
// are a plain function invocation rather than `eval` — pages with a strict
// Content-Security-Policy keep working. One call is made per request; the
// result is posted back over the same IPC channel as other agent scripts.
//
// Refs are per document: e1, e2, ... assigned once per element and never
// recycled. A navigation destroys the JS context, so a new document starts at
// e1 again. Within a document, re-snapshotting keeps existing refs stable and
// only numbers elements that are new.
(() => {
  if (window.__consoleDriverApi) return;

  const MAX_LINES = 200;
  const MAX_NODES = 20000;
  const MAX_TEXT = 120;
  const MAX_RESULT_CHARS = 30000;

  const state = {
    refs: new Map(), // ref -> { el: WeakRef, role, name, nth }
    byEl: new WeakMap(), // Element -> ref
    next: 1,
    lastSig: '', // last extract's page signature, for waitForNew
  };

  const norm = (value) => (value || '').replace(/\s+/g, ' ').trim();
  const clip = (value, limit) => {
    const text = norm(value);
    return text.length > limit ? text.slice(0, limit - 1) + '…' : text;
  };
  const quote = (value) => {
    try {
      return JSON.stringify(value);
    } catch (_) {
      return '"' + String(value).replace(/"/g, '\\"') + '"';
    }
  };

  const serialize = (value) => {
    if (value === undefined) return 'undefined';
    if (typeof value === 'string') return value;
    if (typeof Node !== 'undefined' && value instanceof Node) {
      return value.outerHTML || value.textContent || String(value);
    }
    const seen = new WeakSet();
    try {
      const json = JSON.stringify(
        value,
        (_key, item) => {
          if (typeof item === 'function') return '[Function]';
          if (typeof item === 'bigint') return item.toString();
          if (typeof Node !== 'undefined' && item instanceof Node) return '[' + item.nodeName + ']';
          if (item && typeof item === 'object') {
            if (seen.has(item)) return '[Circular]';
            seen.add(item);
          }
          return item;
        },
        2
      );
      return json === undefined ? String(value) : json;
    } catch (_) {
      return String(value);
    }
  };

  const cap = (text) => {
    if (text.length > MAX_RESULT_CHARS) {
      return text.slice(0, MAX_RESULT_CHARS)
        + '\n...[truncated: ' + text.length + ' chars total]';
    }
    return text;
  };

  const SKIP_TAGS = new Set([
    'SCRIPT', 'STYLE', 'NOSCRIPT', 'TEMPLATE', 'HEAD', 'META', 'LINK', 'TITLE', 'BASE',
  ]);

  const INPUT_ROLES = {
    button: 'button', submit: 'button', reset: 'button', image: 'button', file: 'button',
    checkbox: 'checkbox', radio: 'radio', range: 'slider', search: 'searchbox',
    number: 'spinbutton', email: 'textbox', tel: 'textbox', url: 'textbox',
    text: 'textbox', password: 'textbox', hidden: null,
  };

  const TAG_ROLES = {
    A: 'link', BUTTON: 'button', SELECT: 'combobox', TEXTAREA: 'textbox',
    SUMMARY: 'button', DETAILS: 'group', OPTION: 'option', OPTGROUP: 'group',
    NAV: 'navigation', MAIN: 'main', HEADER: 'banner', FOOTER: 'contentinfo',
    ASIDE: 'complementary', FORM: 'form', DIALOG: 'dialog', SEARCH: 'search',
    UL: 'list', OL: 'list', LI: 'listitem', TABLE: 'table', TR: 'row',
    TD: 'cell', TH: 'columnheader', ARTICLE: 'article', H1: 'heading', H2: 'heading',
    H3: 'heading', H4: 'heading', H5: 'heading', H6: 'heading', IMG: 'img',
    CANVAS: 'img', SVG: 'img', IFRAME: 'Iframe', FRAME: 'Iframe',
  };

  const INTERACTIVE = new Set([
    'button', 'link', 'textbox', 'searchbox', 'checkbox', 'radio', 'combobox',
    'listbox', 'menuitem', 'menuitemcheckbox', 'menuitemradio', 'option', 'slider',
    'spinbutton', 'switch', 'tab', 'treeitem', 'Iframe',
  ]);
  const CONTENT = new Set([
    'heading', 'cell', 'gridcell', 'columnheader', 'rowheader', 'listitem',
    'article', 'region', 'main', 'navigation', 'banner', 'contentinfo',
    'complementary', 'form', 'search', 'dialog', 'alert', 'status', 'img',
  ]);
  const LANDMARKS = new Set([
    'main', 'navigation', 'banner', 'contentinfo', 'complementary', 'region',
    'form', 'search', 'dialog',
  ]);

  const tagOf = (el) => (el.tagName || '').toUpperCase();

  const roleOf = (el) => {
    const explicit = el.getAttribute && el.getAttribute('role');
    if (explicit && norm(explicit)) return norm(explicit).split(/\s+/)[0];
    const tag = tagOf(el);
    if (tag === 'INPUT') {
      const type = (el.getAttribute('type') || 'text').toLowerCase();
      return Object.prototype.hasOwnProperty.call(INPUT_ROLES, type)
        ? INPUT_ROLES[type]
        : 'textbox';
    }
    if (tag === 'A') return el.hasAttribute('href') ? 'link' : null;
    if (tag === 'IMG') return norm(el.getAttribute('alt')) ? 'img' : null;
    if (tag === 'SVG' || tag === 'CANVAS') {
      return el.getAttribute('role') === 'img' || el.getAttribute('aria-label') ? 'img' : null;
    }
    if (el.isContentEditable) return 'textbox';
    return TAG_ROLES[tag] || null;
  };

  const isRendered = (el) => {
    if (!el || !el.isConnected) return false;
    if (el.hidden) return false;
    const tag = tagOf(el);
    if (tag === 'BODY' || tag === 'HTML') return true;
    if (tag === 'OPTION' || tag === 'OPTGROUP') return true;
    let style;
    try {
      const win = (el.ownerDocument && el.ownerDocument.defaultView) || window;
      style = win.getComputedStyle(el);
    } catch (_) {
      return false;
    }
    if (style.display === 'none' || style.visibility === 'hidden' || style.visibility === 'collapse') {
      return false;
    }
    return style.opacity !== '0';
  };

  const hasBox = (el) => {
    const tag = tagOf(el);
    if (tag === 'BODY' || tag === 'HTML' || tag === 'OPTION' || tag === 'OPTGROUP') return true;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 || rect.height > 0;
  };

  const isVisible = (el) => isRendered(el) && hasBox(el);

  // An accessibility snapshot should not advertise subtrees the accessibility
  // tree removes. `aria-hidden` is the common case on sites that keep
  // duplicate desktop/mobile or template markup in the DOM; `inert` is the
  // modern equivalent for non-interactive regions.
  const isA11yHidden = (el) => {
    if (!el || el.nodeType !== 1) return false;
    if (el.getAttribute && el.getAttribute('aria-hidden') === 'true') return true;
    if (el.hasAttribute && (el.hasAttribute('inert') || el.inert)) return true;
    return false;
  };

  const isA11yHiddenDeep = (el) => {
    for (let node = el; node && node.nodeType === 1; node = node.parentElement) {
      if (isA11yHidden(node)) return true;
    }
    return false;
  };

  const textOf = (el) => {
    if (!el) return '';
    const tag = tagOf(el);
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return '';
    try {
      return norm(el.innerText);
    } catch (_) {
      return norm(el.textContent);
    }
  };

  // Structured extraction keeps line breaks so a table row or result card
  // stays readable, while collapsing the horizontal whitespace Amazon and
  // friends leave between inline elements.
  const queryText = (el) => {
    const tag = tagOf(el);
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return '';
    let text = '';
    try {
      text = (el.innerText || el.textContent || '').trim();
    } catch (_) {
      text = '';
    }
    text = text.replace(/[ \t\f\v]+/g, ' ').replace(/\n{3,}/g, '\n\n');
    return text.length > 400 ? text.slice(0, 399) + '…' : text;
  };

  const accessibleName = (el, role) => {
    if (!el.getAttribute) return '';
    const aria = el.getAttribute('aria-label');
    if (aria && norm(aria)) return clip(aria, 100);
    const labelledBy = el.getAttribute('aria-labelledby');
    if (labelledBy) {
      const doc = el.ownerDocument || document;
      const text = labelledBy
        .split(/\s+/)
        .map((id) => doc.getElementById(id))
        .filter(Boolean)
        .map((node) => node.textContent)
        .join(' ');
      if (norm(text)) return clip(text, 100);
    }
    if (el.labels && el.labels.length) {
      const text = Array.from(el.labels).map((label) => label.textContent).join(' ');
      if (norm(text)) return clip(text, 100);
    }
    const tag = tagOf(el);
    if (tag === 'IMG' || role === 'img') {
      const alt = el.getAttribute('alt');
      if (alt && norm(alt)) return clip(alt, 100);
    }
    if (tag === 'INPUT' && ['button', 'submit', 'reset'].includes((el.type || '').toLowerCase())) {
      if (el.value) return clip(el.value, 100);
    }
    if (INTERACTIVE.has(role) || CONTENT.has(role) || LANDMARKS.has(role)) {
      const text = textOf(el);
      if (text) return clip(text, 100);
    }
    const title = el.getAttribute('title');
    if (title && norm(title)) return clip(title, 100);
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') {
      const placeholder = el.getAttribute('placeholder');
      if (placeholder && norm(placeholder)) return clip(placeholder, 100);
    }
    return '';
  };

  const refFor = (el, role, name, nth) => {
    let ref = state.byEl.get(el);
    if (!ref) {
      ref = 'e' + state.next++;
      state.byEl.set(el, ref);
    }
    state.refs.set(ref, {
      el: typeof WeakRef === 'function' ? new WeakRef(el) : { deref: () => el },
      role,
      name,
      nth,
    });
    return ref;
  };

  const deref = (entry) => {
    try {
      return entry && entry.el ? entry.el.deref() : null;
    } catch (_) {
      return null;
    }
  };

  const isStale = (el) => !el || !el.isConnected;

  const attrsOf = (el, role) => {
    const attrs = [];
    if (role === 'link') {
      const href = el.getAttribute('href');
      if (href && norm(href)) attrs.push('url=' + clip(href, 80));
    }
    if (role === 'heading') {
      const level = Number(tagOf(el).slice(1)) || Number(el.getAttribute('aria-level')) || null;
      if (level) attrs.push('level=' + level);
    }
    if (role === 'checkbox' || role === 'radio' || role === 'switch') {
      if (el.indeterminate) attrs.push('checked=mixed');
      else if (typeof el.checked === 'boolean') attrs.push('checked=' + el.checked);
      else {
        const aria = el.getAttribute('aria-checked');
        if (aria) attrs.push('checked=' + aria);
      }
    }
    const expanded = el.getAttribute('aria-expanded');
    if (expanded) attrs.push('expanded=' + expanded);
    const selected = el.getAttribute('aria-selected');
    if (selected === 'true') attrs.push('selected');
    if (el.disabled || el.getAttribute('aria-disabled') === 'true') attrs.push('disabled');
    if (el.required || el.getAttribute('aria-required') === 'true') attrs.push('required');
    return attrs;
  };

  const valueOf = (el) => {
    const tag = tagOf(el);
    const type = (el.type || '').toLowerCase();
    if (tag === 'INPUT' && type === 'password') return '';
    if (tag === 'INPUT' && ['button', 'submit', 'reset', 'image', 'file'].includes(type)) return '';
    if (tag === 'SELECT') {
      const option = el.options ? el.options[el.selectedIndex] : null;
      return clip(option ? option.text : el.value || '', 80);
    }
    if (tag === 'INPUT' || tag === 'TEXTAREA') return clip(el.value || '', 80);
    if (el.isContentEditable) return clip(el.innerText || '', 80);
    return '';
  };

  // --- snapshot -----------------------------------------------------------

  const rootsForSelector = (selector) => {
    const roots = [];
    const visit = (root) => {
      let matches = [];
      try {
        matches = Array.from(root.querySelectorAll(selector));
      } catch (error) {
        throw new Error('Invalid selector: ' + error.message);
      }
      for (const el of matches) roots.push(el);
      for (const el of Array.from(root.querySelectorAll('*'))) {
        if (el.shadowRoot) visit(el.shadowRoot);
      }
    };
    visit(document);
    return roots;
  };

  const snapshot = (selector) => {
    const started = performance.now();
    const lines = [];
    const seen = new Set();
    const roleCounts = new Map();
    let visited = 0;
    let truncated = false;
    let emitted = 0;

    const push = (depth, text) => {
      if (lines.length >= MAX_LINES) {
        truncated = true;
        return false;
      }
      lines.push('  '.repeat(depth) + '- ' + text);
      emitted += 1;
      return true;
    };

    const emitNode = (el, depth) => {
      const role = roleOf(el);
      if (!role) return false;
      // Native <option>s are already represented by their <select>'s value and
      // would otherwise drown a form snapshot in lines.
      if (tagOf(el) === 'OPTION') return false;
      const name = accessibleName(el, role);
      const actionable =
        INTERACTIVE.has(role) ||
        CONTENT.has(role) ||
        (el.hasAttribute('role') && role !== 'presentation' && role !== 'none');
      if (!actionable) return false;
      if (LANDMARKS.has(role) && !name) return false;

      const key = role + '\u0000' + name;
      const nth = roleCounts.get(key) || 0;
      roleCounts.set(key, nth + 1);
      const ref = refFor(el, role, name, nth);

      let line = role + (name ? ' ' + quote(name) : '');
      const attrs = attrsOf(el, role);
      attrs.push('ref=' + ref);
      line += ' [' + attrs.join(', ') + ']';
      const value = valueOf(el);
      if (value && value !== name) line += ': ' + value;
      return push(depth, line);
    };

    const walk = (node, depth, insideEmitted) => {
      if (truncated || visited >= MAX_NODES) {
        truncated = true;
        return;
      }
      if (node.nodeType === 3) {
        if (!insideEmitted) {
          const text = clip(node.nodeValue, MAX_TEXT);
          if (text.length >= 2) push(depth, 'text ' + quote(text));
        }
        return;
      }
      if (node.nodeType !== 1) return;
      if (seen.has(node)) return;
      seen.add(node);
      visited += 1;

      const tag = tagOf(node);
      if (SKIP_TAGS.has(tag)) return;
      if (!isRendered(node)) return;
      if (isA11yHidden(node)) return;

      if (tag === 'IFRAME' || tag === 'FRAME') {
        const name = accessibleName(node, 'Iframe') || clip(node.getAttribute('title'), 100);
        const ref = refFor(node, 'Iframe', name, 0);
        push(depth, 'Iframe' + (name ? ' ' + quote(name) : '') + ' [ref=' + ref + ']');
        let frameDoc = null;
        try {
          frameDoc = node.contentDocument;
        } catch (_) {
          frameDoc = null;
        }
        if (frameDoc && frameDoc.body) {
          walk(frameDoc.body, depth + 1, false);
        } else {
          push(depth + 1, 'text "(cross-origin frame; not inspectable)"');
        }
        return;
      }

      const didEmit = hasBox(node) ? emitNode(node, depth) : false;
      const childDepth = didEmit ? depth + 1 : depth;

      if (node.shadowRoot) {
        for (const child of Array.from(node.shadowRoot.childNodes)) {
          walk(child, childDepth, didEmit);
          if (truncated) return;
        }
      }
      for (const child of Array.from(node.childNodes)) {
        walk(child, childDepth, didEmit);
        if (truncated) return;
      }
    };

    // A selector match may itself be interesting; emit it at depth 0 and then
    // walk its children.
    const seeRoot = (el) => {
      if (!isRendered(el)) return;
      if (isA11yHiddenDeep(el)) return;
      seen.add(el);
      visited += 1;
      const didEmit = hasBox(el) ? emitNode(el, 0) : false;
      const childDepth = didEmit ? 1 : 0;
      if (el.shadowRoot) {
        for (const child of Array.from(el.shadowRoot.childNodes)) walk(child, childDepth, didEmit);
      }
      for (const child of Array.from(el.childNodes)) walk(child, childDepth, didEmit);
    };

    if (selector) {
      let roots;
      try {
        roots = rootsForSelector(selector);
      } catch (error) {
        return error.message;
      }
      if (!roots.length) return 'No match for selector: ' + selector;
      for (const root of roots) {
        if (seen.has(root)) continue;
        seeRoot(root);
      }
    } else if (document.body) {
      walk(document.body, 0, false);
    }

    if (truncated) {
      lines.push('... [truncated: snapshot reached ' + MAX_LINES + ' lines or ' + MAX_NODES
        + ' nodes; pass selector to scope]');
    }
    if (!emitted) lines.push('(no actionable elements)');
    const elapsed = Math.round(performance.now() - started);
    return 'Snapshot: ' + emitted + ' nodes (' + elapsed + 'ms)\n' + lines.join('\n');
  };

  // --- ref resolution -----------------------------------------------------

  const allElements = () => {
    const out = [];
    const visit = (root) => {
      for (const el of Array.from(root.querySelectorAll('*'))) {
        out.push(el);
        if (el.shadowRoot) visit(el.shadowRoot);
      }
    };
    visit(document);
    return out;
  };

  const findByDescriptor = (entry) => {
    if (!entry || !entry.role) return null;
    const candidates = [];
    for (const el of allElements()) {
      if (isStale(el) || !isVisible(el) || isA11yHiddenDeep(el)) continue;
      if (roleOf(el) !== entry.role) continue;
      if (accessibleName(el, entry.role) !== entry.name) continue;
      candidates.push(el);
    }
    return candidates[entry.nth] || (candidates.length === 1 ? candidates[0] : null);
  };

  const resolve = (ref) => {
    const entry = state.refs.get(ref);
    const el = deref(entry);
    if (el && !isStale(el)) return { el, entry };
    const fallback = findByDescriptor(entry);
    if (fallback) {
      state.byEl.set(fallback, ref);
      state.refs.set(ref, {
        el: typeof WeakRef === 'function' ? new WeakRef(fallback) : { deref: () => fallback },
        role: entry.role,
        name: entry.name,
        nth: entry.nth,
      });
      return { el: fallback, entry };
    }
    return { el: null, entry };
  };

  const describe = (el, role, name) => role + (name ? ' ' + quote(name) : '');

  // --- locators ------------------------------------------------------------

  // Text and role targets arrive as prefixed selectors from the server's
  // browser kernel: `text=Add to cart`, `role=button[name="Search"]`. The
  // name part is JSON-quoted, so quotes and brackets survive the round trip.
  const parseLocator = (raw) => {
    const s = raw.trim();
    if (s.startsWith('text=')) return { kind: 'text', value: s.slice(5) };
    if (s.startsWith('role=')) {
      const m = s.match(/^role=([A-Za-z]+)(?:\[name=([\s\S]*)\])?$/);
      if (!m) return null;
      let name = '';
      if (m[2] !== undefined) {
        const rawName = m[2].trim();
        if (rawName.startsWith('"') || rawName.startsWith("'")) {
          try {
            name = norm(JSON.parse(rawName));
          } catch (_) {
            // Single quotes are not JSON; strip and unescape by hand.
            name = norm(rawName.slice(1, -1).replace(/\\'/g, "'"));
          }
        } else {
          name = norm(rawName);
        }
      }
      return { kind: 'role', role: m[1], name };
    }
    return null;
  };

  // Find by visible text: first exact match on accessible name or inner text
  // in document order wins; a substring match is the fallback. Visible and
  // accessibility-visible elements only, like a snapshot would report.
  const findByText = (wantedRaw) => {
    const wanted = norm(wantedRaw);
    if (!wanted) return null;
    let substring = null;
    for (const el of allElements()) {
      if (isStale(el) || !isVisible(el) || isA11yHiddenDeep(el)) continue;
      const role = roleOf(el) || '';
      const name = norm(accessibleName(el, role));
      const text = norm(textOf(el));
      if (name === wanted || text === wanted) return el;
      if (!substring && (name.includes(wanted) || text.includes(wanted))) substring = el;
    }
    return substring;
  };

  // Find by ARIA role and optional name (exact first, then substring).
  const findByRole = (role, nameRaw) => {
    const wanted = norm(nameRaw);
    let substring = null;
    for (const el of allElements()) {
      if (isStale(el) || !isVisible(el) || isA11yHiddenDeep(el)) continue;
      if (roleOf(el) !== role) continue;
      const name = norm(accessibleName(el, role));
      if (!wanted) return el;
      if (name === wanted) return el;
      if (!substring && name.includes(wanted)) substring = el;
    }
    return substring;
  };

  // Resolve a prefixed locator to an element. Returns {error} when the
  // locator kind matched but no element did; {el: null} when the string is
  // not a locator (plain CSS — the caller falls through).
  const resolveLocator = (raw) => {
    const loc = parseLocator(raw);
    if (!loc) return { el: null };
    if (loc.kind === 'text') {
      const el = findByText(loc.value);
      if (!el) return { error: 'No visible match for text: ' + loc.value };
      const role = roleOf(el) || 'element';
      return { el, label: describe(el, role, accessibleName(el, role)) + ' (text ' + quote(loc.value) + ')' };
    }
    const el = findByRole(loc.role, loc.name);
    if (!el) {
      return {
        error: 'No visible match for role ' + loc.role + (loc.name ? ' named ' + quote(loc.name) : ''),
      };
    }
    return { el, label: describe(el, loc.role, accessibleName(el, loc.role)) + ' (role ' + loc.role + ')' };
  };

  // --- structured extraction ----------------------------------------------

  const MAX_EXTRACT_ITEMS = 200;
  const MAX_EXTRACT_FIELDS = 16;
  const MAX_EXTRACT_CHARS = 200000;
  // Signature scan bound: counting every card of a 10k-row page would stall
  // the webview; past this the signature is "many" anyway.
  const MAX_EXTRACT_SCAN = 2000;

  // The first visible match of the scope selector list. Comma parts are
  // alternatives tried in order (#buybox, #corePrice_feature_div), not one
  // result set — a price block should not leak related products in.
  const scopeRoot = (scope) => {
    if (!scope || !scope.trim()) return document.body;
    for (const part of scope.split(',')) {
      const sel = part.trim();
      if (!sel) continue;
      let matches;
      try {
        matches = Array.from(document.querySelectorAll(sel));
      } catch (error) {
        throw new Error('Invalid scope selector: ' + error.message);
      }
      for (const el of matches) {
        if (isVisible(el) && !isA11yHiddenDeep(el)) return el;
      }
    }
    return null;
  };

  // Cheap page signature for waitForNew: item count plus the first and last
  // item's text. Growing or replaced lists change at least one of the three.
  const pageSig = (spec) => {
    let root;
    try {
      root = scopeRoot(spec.scope);
    } catch (error) {
      return error.message;
    }
    if (!root) return 'no-scope';
    if (!spec.items) return 'single';
    let rows;
    try {
      rows = Array.from(root.querySelectorAll(spec.items));
    } catch (error) {
      return 'bad-items';
    }
    let count = 0;
    let first = '';
    let last = '';
    for (const el of rows) {
      if (count >= MAX_EXTRACT_SCAN) break;
      if (!isVisible(el) || isA11yHiddenDeep(el)) continue;
      if (!count) first = norm(textOf(el));
      last = norm(textOf(el));
      count++;
    }
    return count + ':' + first + '|' + last;
  };

  // One extraction pass. Returns {error} text, or {sig, items, truncated}.
  const extractOnce = (spec) => {
    const fieldSpecs = spec.fields && typeof spec.fields === 'object' ? Object.entries(spec.fields) : [];
    if (fieldSpecs.length > MAX_EXTRACT_FIELDS) {
      return { error: 'Too many fields (max ' + MAX_EXTRACT_FIELDS + ').' };
    }
    let root;
    try {
      root = scopeRoot(spec.scope);
    } catch (error) {
      return { error: error.message };
    }
    if (!root) return { error: 'No visible match for scope: ' + spec.scope };
    let rows;
    if (spec.items) {
      try {
        rows = Array.from(root.querySelectorAll(spec.items));
      } catch (error) {
        return { error: 'Invalid items selector: ' + error.message };
      }
    } else {
      rows = [root];
    }
    const sig = pageSig(spec);

    const limit = Math.min(Number(spec.limit) > 0 ? Number(spec.limit) : 30, MAX_EXTRACT_ITEMS);
    const items = [];
    let size = 0;
    let truncated = false;
    let scanned = 0;
    for (const el of rows) {
      if (scanned >= MAX_EXTRACT_SCAN || items.length >= limit || truncated) break;
      if (!isVisible(el) || isA11yHiddenDeep(el)) continue;
      scanned++;
      const row = {};
      for (const [name, rawSpec] of fieldSpecs) {
        const fs = typeof rawSpec === 'string' ? { selector: rawSpec } : rawSpec;
        if (!fs || typeof fs !== 'object' || !fs.selector) {
          row[name] = null;
          continue;
        }
        let nodes;
        try {
          nodes = Array.from(el.querySelectorAll(fs.selector));
        } catch (error) {
          return { error: 'Invalid selector for field ' + name + ': ' + error.message };
        }
        if (fs.all) {
          const values = [];
          for (const node of nodes) {
            const value = fs.attribute ? node.getAttribute(fs.attribute) || '' : queryText(node);
            if (value !== '') values.push(fs.attribute ? String(value).trim() : value);
          }
          row[name] = values;
          continue;
        }
        const node = nodes.find((candidate) => isVisible(candidate)) || nodes[0] || null;
        if (!node) {
          row[name] = null;
          continue;
        }
        // An attribute read of null means the attribute is missing: the
        // field is null, never a fallback to some page-wide selector.
        let value = null;
        if (fs.attribute) {
          const raw = node.getAttribute(fs.attribute);
          if (raw != null) value = fs.trim === false ? raw : raw.trim();
        } else {
          value = queryText(node);
        }
        row[name] = value === null || value === '' ? null : value;
      }
      const rowJson = JSON.stringify(row);
      if (size + rowJson.length > MAX_EXTRACT_CHARS) {
        truncated = true;
        break;
      }
      items.push(row);
      size += rowJson.length;
    }
    return { sig, items, truncated };
  };


  // --- hit test -----------------------------------------------------------

  const hitAt = (el) => {
    const rect = el.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) return { rect, hit: null };
    const x = rect.left + rect.width / 2;
    const y = rect.top + rect.height / 2;
    const doc = el.ownerDocument || document;
    let hit = doc.elementFromPoint(x, y);
    let guard = 0;
    while (hit && guard < 8) {
      guard += 1;
      if (hit.shadowRoot) {
        const inner = hit.shadowRoot.elementFromPoint(x, y);
        if (inner && inner !== hit) {
          hit = inner;
          continue;
        }
      }
      break;
    }
    return { rect, hit, x, y };
  };

  const covers = (target, hit) => {
    if (!hit) return false;
    if (hit === target || target.contains(hit)) return true;
    if (target.shadowRoot && target.shadowRoot.contains(hit)) return true;
    return false;
  };

  const hitError = (el, hit) => {
    if (!hit) return 'Element is not visible or has no clickable box.';
    const role = roleOf(hit) || '';
    const name = accessibleName(hit, role);
    const id = hit.id ? '#' + hit.id : '';
    const cls = hit.classList && hit.classList.length ? '.' + hit.classList[0] : '';
    return 'Element is covered by <' + tagOf(hit).toLowerCase() + id + cls + '>'
      + (role ? ' (' + describe(hit, role, name) + ')' : '');
  };

  const scrollAndHit = (el) => {
    try {
      el.scrollIntoView({ block: 'center', inline: 'center', behavior: 'instant' });
    } catch (_) {
      el.scrollIntoView();
    }
    return hitAt(el);
  };

  // --- actions ------------------------------------------------------------

  const click = (el) => {
    if (el.disabled || el.getAttribute('aria-disabled') === 'true') {
      return { error: 'Element is disabled.' };
    }
    if (!isVisible(el)) {
      return {
        error: 'Element is not visible (hidden by display, visibility, opacity, or has no clickable box).',
      };
    }
    const { rect, hit, x, y } = scrollAndHit(el);
    if (rect.width <= 0 || rect.height <= 0) {
      return { error: 'Element is not visible or has no clickable box.' };
    }
    if (!covers(el, hit)) return { error: hitError(el, hit) };
    const win = (el.ownerDocument && el.ownerDocument.defaultView) || window;
    const base = {
      bubbles: true,
      cancelable: true,
      composed: true,
      view: win,
      button: 0,
      buttons: 1,
      clientX: x,
      clientY: y,
      screenX: win.screenX + x,
      screenY: win.screenY + y,
      pointerId: 1,
      pointerType: 'mouse',
      isPrimary: true,
    };
    try {
      el.dispatchEvent(new win.PointerEvent('pointerover', base));
      el.dispatchEvent(new win.PointerEvent('pointerdown', base));
    } catch (_) {}
    el.dispatchEvent(new win.MouseEvent('mouseover', base));
    el.dispatchEvent(new win.MouseEvent('mousedown', base));
    if (el.focus) {
      try {
        el.focus({ preventScroll: true });
      } catch (_) {
        el.focus();
      }
    }
    try {
      el.dispatchEvent(new win.PointerEvent('pointerup', Object.assign({}, base, { buttons: 0 })));
    } catch (_) {}
    el.dispatchEvent(new win.MouseEvent('mouseup', Object.assign({}, base, { buttons: 0 })));
    el.dispatchEvent(new win.MouseEvent('click', Object.assign({}, base, { buttons: 0 })));
    return { ok: true };
  };

  const setValue = (el, value) => {
    const win = (el.ownerDocument && el.ownerDocument.defaultView) || window;
    const proto = el instanceof win.HTMLTextAreaElement
      ? win.HTMLTextAreaElement.prototype
      : win.HTMLInputElement.prototype;
    const setter = Object.getOwnPropertyDescriptor(proto, 'value');
    if (setter && setter.set) setter.set.call(el, value);
    else el.value = value;
  };

  const selectOption = (el, text) => {
    const win = (el.ownerDocument && el.ownerDocument.defaultView) || window;
    const options = Array.from(el.options || []);
    const match = options.find((option) => option.value === text)
      || options.find((option) => norm(option.text) === norm(text))
      || options.find((option) => norm(option.text).toLowerCase() === norm(text).toLowerCase());
    if (!match) {
      return {
        error: 'No option ' + quote(text) + '. Options: '
          + options.map((option) => quote(clip(option.text, 40))).join(', '),
      };
    }
    el.value = match.value;
    el.dispatchEvent(new win.Event('input', { bubbles: true }));
    el.dispatchEvent(new win.Event('change', { bubbles: true }));
    return { ok: true, selected: match.text };
  };

  const type = (el, text, submit) => {
    const tag = tagOf(el);
    if (tag === 'SELECT') return selectOption(el, text);
    const type = (el.type || '').toLowerCase();
    const nonText = ['checkbox', 'radio', 'button', 'submit', 'reset', 'image', 'file', 'range'];
    if (tag === 'INPUT' && nonText.includes(type)) {
      return { error: 'Element is not a text field; use click.' };
    }
    if (!el.isContentEditable && !('value' in el)) {
      return { error: 'Element is not a text field.' };
    }
    const win = (el.ownerDocument && el.ownerDocument.defaultView) || window;
    const doc = el.ownerDocument || document;
    el.focus();
    if (el.isContentEditable) {
      try {
        doc.execCommand('selectAll', false, null);
        if (!doc.execCommand('insertText', false, text)) el.textContent = text;
      } catch (_) {
        el.textContent = text;
      }
      el.dispatchEvent(new win.InputEvent('input', { bubbles: true, composed: true, data: text }));
      return { ok: true };
    }
    setValue(el, '');
    el.dispatchEvent(new win.InputEvent('input', {
      bubbles: true, composed: true, inputType: 'deleteContentBackward',
    }));
    setValue(el, text);
    el.dispatchEvent(new win.InputEvent('input', {
      bubbles: true, composed: true, inputType: 'insertText', data: text,
    }));
    el.dispatchEvent(new win.Event('change', { bubbles: true }));
    if (submit) {
      const key = {
        key: 'Enter', code: 'Enter', keyCode: 13, which: 13,
        bubbles: true, cancelable: true, composed: true,
      };
      const down = new win.KeyboardEvent('keydown', key);
      el.dispatchEvent(down);
      el.dispatchEvent(new win.KeyboardEvent('keypress', key));
      el.dispatchEvent(new win.KeyboardEvent('keyup', key));
      if (!down.defaultPrevented && el.form) {
        if (el.form.requestSubmit) el.form.requestSubmit();
        else el.form.submit();
      }
    }
    return { ok: true };
  };

  // --- entry point --------------------------------------------------------

  const execute = async (req) => {
    if (req.op === 'snapshot') return snapshot(req.selector);

    if (req.op === 'eval') {
      let value = (0, eval)(req.script || '');
      if (typeof value === 'function') value = value();
      value = await value;
      return cap(serialize(value));
    }

    if (req.op === 'content') {
      const selector = (req.selector || '').trim();
      if (!selector) return document.body ? document.body.innerText : '';
      let elements;
      try {
        elements = Array.from(document.querySelectorAll(selector));
      } catch (error) {
        return 'Invalid selector: ' + error.message;
      }
      if (!elements.length) return 'No match for selector: ' + selector;
      const out = elements.slice(0, 20).map(
        (el, i) => '[' + i + '] ' + (el.innerText || el.textContent || '').trim()
      );
      if (elements.length > 20) {
        out.push('...(' + elements.length + ' matches total, showing 20)');
      }
      return out.join('\n\n');
    }

    if (req.op === 'query') {
      const selector = (req.selector || '').trim();
      if (!selector) return 'Missing selector for query.';
      let elements;
      try {
        elements = Array.from(document.querySelectorAll(selector));
      } catch (error) {
        return 'Invalid selector: ' + error.message;
      }
      if (!elements.length) return 'No match for selector: ' + selector;

      const MAX_QUERY = 30;
      const MAX_QUERY_CHARS = 24000;
      const items = [];
      let visibleCount = 0;
      let size = 0;
      for (const el of elements) {
        if (!isVisible(el) || isA11yHiddenDeep(el)) continue;
        visibleCount += 1;
        if (items.length >= MAX_QUERY) continue;

        const tag = tagOf(el).toLowerCase();
        const role = roleOf(el) || '';
        const text = queryText(el);
        const item = { tag };
        if (role) item.role = role;
        if (text) item.text = text;
        const name = role ? accessibleName(el, role) : '';
        if (name && name !== text) item.name = clip(name, 100);
        let href = '';
        if (tagOf(el) === 'A' && el.href) href = el.href;
        else if (el.getAttribute) href = el.getAttribute('href') || '';
        if (!href && el.querySelector) {
          // A result card is usually a wrapper, not the link itself. Surface
          // its primary link so the agent can open the exact item.
          const link = el.querySelector('a[href]');
          if (link && link.href) href = link.href;
        }
        if (href) item.href = String(href).slice(0, 300);
        const image = el.querySelector ? el.querySelector('img[src]') : null;
        const src = el.currentSrc || el.src
          || (el.getAttribute && el.getAttribute('src'))
          || (image && (image.currentSrc || image.src)) || '';
        if (src) item.src = String(src).slice(0, 300);
        if (tag === 'input' || tag === 'textarea' || tag === 'select') {
          const value = String(el.value || '');
          if (value) item.value = value.slice(0, 120);
        }
        if (tag === 'input' && typeof el.checked === 'boolean') item.checked = el.checked;
        if (el.disabled === true) item.disabled = true;
        const itemJson = JSON.stringify(item);
        if (size + itemJson.length > MAX_QUERY_CHARS) continue;
        items.push(item);
        size += itemJson.length;
      }
      if (!items.length) return 'No visible match for selector: ' + selector;
      const payload = { selector, count: visibleCount, items };
      if (visibleCount > items.length) payload.truncated = true;
      return JSON.stringify(payload);
    }

    if (req.op === 'extract') {
      let spec;
      try {
        spec = JSON.parse(req.spec || '{}');
      } catch (error) {
        return 'Invalid extract spec: ' + error.message;
      }
      if (!spec || typeof spec !== 'object') return 'Invalid extract spec: expected an object';
      const waitMs = Math.min(Number(spec.waitMs) > 0 ? Number(spec.waitMs) : 8000, 15000);
      const prevSig = state.lastSig;
      if (spec.waitForNew) {
        // In-page poll until the matched items change (or the budget runs
        // out) — one host call per page instead of polling from the server.
        const deadline = Date.now() + waitMs;
        while (Date.now() < deadline) {
          const current = pageSig(spec);
          if (current !== prevSig) break;
          await new Promise((resolve) => setTimeout(resolve, 300));
        }
      }
      const out = extractOnce(spec);
      if (out.error) return out.error;
      state.lastSig = out.sig;
      const payload = { count: out.items.length, items: out.items };
      if (out.truncated) payload.truncated = true;
      if (spec.waitForNew && out.sig === prevSig) payload.unchanged = true;
      return JSON.stringify(payload);
    }

    if (req.op === 'scroll') {
      let spec;
      try {
        spec = JSON.parse(req.spec || '{}');
      } catch (error) {
        return 'Invalid scroll spec: ' + error.message;
      }
      const scroller = document.scrollingElement || document.documentElement;
      const viewport = window.innerHeight || 800;
      const direction = spec.direction || 'down';
      const amount = Number(spec.amount) > 0 ? Number(spec.amount) : Math.round(viewport * 0.9);
      if (direction === 'top') window.scrollTo(0, 0);
      else if (direction === 'bottom') window.scrollTo(0, scroller.scrollHeight);
      else if (direction === 'up') window.scrollBy(0, -amount);
      else if (direction === 'down') window.scrollBy(0, amount);
      else return 'Invalid direction: ' + quote(direction) + ' (use up, down, top or bottom)';
      const y = window.scrollY || scroller.scrollTop || 0;
      return JSON.stringify({
        scrollY: Math.round(y),
        scrollHeight: scroller.scrollHeight,
        viewportHeight: viewport,
        atTop: y <= 0,
        atBottom: y + viewport >= scroller.scrollHeight - 2,
      });
    }

    if (req.op === 'wait') {
      const selector = (req.selector || '').trim();
      const urlPart = req.urlContains || '';
      const waitText = req.text || '';
      let matches = null;
      if (selector) {
        try {
          matches = Array.from(document.querySelectorAll(selector));
        } catch (error) {
          return 'WF_ERR: Invalid selector: ' + error.message;
        }
      }
      if (urlPart && !location.href.includes(urlPart)) return 'WF_NO';
      if (waitText && !(document.body && document.body.innerText.includes(waitText))) {
        return 'WF_NO';
      }
      if (matches) {
        if (!matches.length) return 'WF_NO';
        const visible = matches.some(
          (el) => isVisible(el) && !isA11yHiddenDeep(el)
        );
        if (!visible) return 'WF_HIDDEN';
      }
      return 'WF_OK';
    }

    const ref = norm(req.ref);
    let el, label;
    if (ref) {
      const resolved = resolve(ref);
      if (!resolved.el) {
        return 'Ref ' + quote(ref) + ' is stale or unknown. Take a new snapshot and use a fresh ref.';
      }
      el = resolved.el;
      const entry = resolved.entry || {};
      const role = roleOf(el) || entry.role || 'element';
      label = describe(el, role, accessibleName(el, role));
      if (el.disabled || el.getAttribute('aria-disabled') === 'true') {
        return 'Element ' + label + ' is disabled.';
      }
      if (isA11yHiddenDeep(el)) {
        return 'Element ' + label
          + ' is hidden from the accessibility tree. Take a new snapshot and use a visible control.';
      }
    } else {
      // Locator/selector fallback: no ref, act on the first visible match.
      // Used by the server's browser kernel, whose scripts address elements
      // without snapshotting first. `text=`/`role=` locators resolve through
      // the finder; anything else is a CSS selector.
      const selector = (req.selector || '').trim();
      if (!selector) return 'A ref is required. Take a snapshot first.';
      const located = resolveLocator(selector);
      if (located.error) return located.error;
      if (located.el) {
        el = located.el;
        label = located.label;
      } else {
        let elements;
        try {
          elements = Array.from(document.querySelectorAll(selector));
        } catch (error) {
          return 'Invalid selector: ' + error.message;
        }
        el = elements.find((candidate) => isVisible(candidate)) || null;
        if (!el) return 'No visible match for selector: ' + selector;
        const role = roleOf(el) || 'element';
        label = describe(el, role, accessibleName(el, role)) + ' (selector ' + quote(selector) + ')';
      }
      if (el.disabled || el.getAttribute('aria-disabled') === 'true') {
        return 'Element ' + label + ' is disabled.';
      }
    }

    const op = req.op;
    if (op === 'click') {
      const result = click(el);
      if (result.error) return 'Could not click ' + label + ': ' + result.error;
      return 'Clicked ' + label + '.';
    }
    if (op === 'type') {
      const result = type(el, req.text == null ? '' : String(req.text), !!req.submit);
      if (result.error) return 'Could not type into ' + label + ': ' + result.error;
      const extra = req.submit ? ' and pressed Enter' : '';
      return 'Typed into ' + label + extra + '.';
    }
    if (op === 'select') {
      const result = selectOption(el, req.text == null ? '' : String(req.text));
      if (result.error) return 'Could not select in ' + label + ': ' + result.error;
      return 'Selected ' + quote(norm(result.selected)) + ' in ' + label + '.';
    }
    if (op === 'check' || op === 'uncheck') {
      const want = op === 'check';
      if (typeof el.checked !== 'boolean') return 'Element ' + label + ' is not a checkbox.';
      if (el.checked !== want) {
        const result = click(el);
        if (result.error) return 'Could not ' + op + ' ' + label + ': ' + result.error;
      }
      if (el.checked !== want) return 'Element ' + label + ' did not change state.';
      return (want ? 'Checked ' : 'Unchecked ') + label + '.';
    }
    if (op === 'hover') {
      const { hit, x, y } = scrollAndHit(el);
      if (!covers(el, hit)) return 'Could not hover ' + label + ': ' + hitError(el, hit);
      const win = (el.ownerDocument && el.ownerDocument.defaultView) || window;
      const base = { bubbles: true, cancelable: true, composed: true, clientX: x, clientY: y };
      el.dispatchEvent(new win.MouseEvent('mouseover', base));
      el.dispatchEvent(new win.MouseEvent('mousemove', base));
      return 'Hovered ' + label + '.';
    }
    return 'Unknown op: ' + op;
  };

  const run = async (requestId, req) => {
    const post = (payload) => {
      try {
        window.ipc.postMessage(JSON.stringify(Object.assign(
          { type: 'script_result', requestId },
          payload
        )));
      } catch (_) {}
    };
    try {
      post({
        ok: true,
        value: await execute(req),
        url: location.href,
        title: document.title,
      });
    } catch (error) {
      const message = error && error.message
        ? (error.name || 'Error') + ': ' + error.message
        : String(error);
      post({ ok: false, error: message, url: location.href, title: document.title });
    }
  };

  window.__consoleDriverApi = { run };
})()
