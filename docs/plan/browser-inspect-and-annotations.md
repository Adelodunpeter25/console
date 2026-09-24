# Built-in Browser Element Inspection & Annotation Plan

## 1. Overview & Motivation
When developing web applications, developers frequently need to point the AI agent to specific UI elements (buttons, headers, layout containers, modal dialogs) to request style fixes, feature additions, or behavior changes.

Currently, developers have to manually take screenshots, paste them into the chat, and describe the target element in words (e.g. *"the blue button on the top right"*). 

This plan details the **Visual Element Inspection & Annotation** system for Console's built-in browser (similar to Cursor's Browser Inspector, Codex Desktop, and v0). It allows developers to:
1. Toggle **Inspect / Annotate Mode** in any built-in browser tab.
2. Hover over DOM elements with live bounding box highlights, tag names, and class lists.
3. Click an element to open a quick annotation popover to type instructions.
4. Automatically attach structured context chips to the active Composer (including component name, React/Vite source file location, computed styles, DOM snippet, and a cropped screenshot thumbnail).
5. Provide the AI agent with zero-ambiguity visual and source-level grounding to directly edit code.

---

## 2. User Experience & Interaction Flow

```
┌─────────────────────────────────────────────────────────────┐
│ Console Browser Tab                                         │
│ [ ↖ Inspect ] [ ◀ ▶ ↻ ] [ http://localhost:3000/cart     ] │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ Shopping Cart                                        │   │
│  │                                                      │   │
│  │  ┌──────────────────────────────────────────────┐    │   │
│  │  │ [✓] Checkout Button                          │    │   │
│  │  │ <CheckoutButton /> • src/Cart.tsx:42         │    │   │
│  │  └──────────────────┬───────────────────────────┘    │   │
│  │                     │                                │   │
│  │          ┌──────────┴──────────────────────────┐     │   │
│  │          │ 💬 "Make this button full-width     │     │   │
│  │          │    and add a loading spinner"       │     │   │
│  │          │                                     │     │   │
│  │          │ [Esc to Cancel]    [Attach to Chat] │     │   │
│  │          └─────────────────────────────────────┘     │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                               │ Click "Attach to Chat" (or ↵)
                               ▼
┌─────────────────────────────────────────────────────────────┐
│ Composer                                                    │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ 🏷️ [CheckoutButton • src/Cart.tsx:42 ✕]                 │ │
│ │ "Make this button full-width and add a loading spinner" │ │
│ └─────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

### Steps:
1. **Activate Inspect Mode**: User clicks the inspect icon (`↖` or `⌥I`) on the browser toolbar.
2. **Hover Highlight**: A high-contrast overlay highlights elements under the cursor, displaying:
   - Tag name, ID, and Tailwind/CSS class names.
   - Framework component name (e.g. `<CartItem />`, `<CheckoutButton />`).
   - Dimensions (`240 × 44 px`).
3. **Select & Comment**:
   - Clicking an element freezes the highlight and opens an annotation popover anchored to the element.
   - User types instructions and presses `Enter` or clicks **Attach to Chat**.
4. **Composer Staging**:
   - An interactive annotation chip is inserted into the active composer pane.
   - Hovering the chip previews the cropped element thumbnail and metadata.
5. **Submission**:
   - When the user sends their message, Console packages the annotation data into the prompt payload.

---

## 3. Component & Source Extraction Engine

When an element is clicked, an injected JavaScript script (`inspector.js`) queries the DOM and browser runtime to extract rich developer metadata.

### 3.1 Metadata Captured

| Field | Source / Method | Example |
| :--- | :--- | :--- |
| **Component Name** | React Fiber tree traversal (`__reactFiber$*`), Vue `__vnode`, or Svelte component | `CheckoutButton` |
| **Source File Location** | React DevTools fiber debug source (`_debugSource`), Vite inspector attributes (`data-source-loc`), or Next.js React DevTools hook | `apps/web/src/components/CheckoutButton.tsx:42:7` |
| **Selector Path** | Unique CSS selector generated up to root / unique ID | `main > div.cart-container > button#btn-checkout` |
| **Bounding Box** | `element.getBoundingClientRect()` | `{ x: 280, y: 450, width: 200, height: 44 }` |
| **Computed Styles** | Relevant computed CSS (colors, margins, padding, display, font) | `{ display: "flex", backgroundColor: "rgb(59, 130, 246)", ... }` |
| **DOM Snapshot** | Outer HTML snippet (truncated to 1KB max, sensitive text masked) | `<button class="btn-primary flex items-center ...">...</button>` |
| **Accessibility Info** | Element role and ARIA attributes | `role: "button", name: "Proceed to Checkout"` |
| **Visual Thumbnail** | WKWebView / WebView2 cropped screenshot of the element bounds | Base64 PNG / JPEG (max 32KB) |

### 3.2 React Fiber / Vite Extraction Logic (`inspector.js`)
```javascript
function extractComponentMetadata(element) {
  let fiberKey = Object.keys(element).find(k => k.startsWith('__reactFiber$') || k.startsWith('__reactInternalInstance$'));
  if (fiberKey) {
    let fiber = element[fiberKey];
    while (fiber) {
      if (typeof fiber.type === 'function' || typeof fiber.type === 'object') {
        const name = fiber.type.displayName || fiber.type.name;
        const source = fiber._debugSource;
        if (name && !name.startsWith('_')) {
          return {
            componentName: name,
            sourceFile: source ? `${source.fileName}:${source.lineNumber}:${source.columnNumber}` : null
          };
        }
      }
      fiber = fiber.return;
    }
  }
  // Check for Vite / Next.js data-source attributes
  const dataLoc = element.closest('[data-source-loc]')?.getAttribute('data-source-loc');
  return { componentName: null, sourceFile: dataLoc || null };
}
```

---

## 4. Data Model

### Rust Desktop State (`console-core` & Desktop App)
```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BrowserElementAnnotation {
    pub id: String,
    pub session_id: Option<String>,
    pub url: String,
    pub title: String,
    pub component_name: Option<String>,
    pub source_location: Option<String>,
    pub selector: String,
    pub html_snippet: String,
    pub dimensions: RectDimensions,
    pub user_comment: String,
    pub screenshot_base64: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RectDimensions {
    pub x: f64,
    pub y: f64,
    pub width: f64,
    pub height: f64,
}
```

### Go Server / API Model (`server-go`)
```go
type BrowserAnnotation struct {
    ID             string         `json:"id"`
    URL            string         `json:"url"`
    ComponentName  string         `json:"component_name,omitempty"`
    SourceLocation string         `json:"source_location,omitempty"`
    Selector       string         `json:"selector"`
    HTMLSnippet    string         `json:"html_snippet"`
    UserComment    string         `json:"user_comment"`
    ImageBase64    string         `json:"image_base64,omitempty"`
}
```

---

## 5. Agent Prompt Formatting

When a message contains one or more browser annotations, the system prompt builder formats them into structured `<browser_annotation>` context blocks:

```xml
<browser_annotation>
  <target_url>http://localhost:3000/cart</target_url>
  <component>CheckoutButton</component>
  <source_location>src/components/CheckoutButton.tsx:42</source_location>
  <css_selector>main > div.cart-actions > button#checkout-btn</css_selector>
  <element_dimensions>200px x 44px at (x: 280, y: 450)</element_dimensions>
  <html_snippet>
    <![CDATA[
    <button id="checkout-btn" class="bg-blue-600 hover:bg-blue-700 text-white font-medium py-2 px-4 rounded">
      Proceed to Checkout
    </button>
    ]]>
  </html_snippet>
  <user_annotation>Make this button full-width and add a loading spinner when clicked</user_annotation>
</browser_annotation>
```

The agent is instructed to:
1. Use `<source_location>` and `<component>` directly to pinpoint the file to edit.
2. Verify CSS classes and markup against `<html_snippet>`.
3. Use the desktop `browser` tool (`measure`, `screenshot`, `eval`) after making edits to confirm visual correctness.

---

## 6. Implementation Roadmap

### Phase 1: In-Page Inspector Script & IPC Bridge
- Implement `inspector.js` (hover bounding boxes, element selection lock, React Fiber / Vite locator extraction, escape-to-dismiss).
- Connect the script via Webview IPC bridge (`window.webkit.messageHandlers.consoleInspector.postMessage` / WebView2 postMessage).
- Add element screenshot cropping using native webview capture APIs.

### Phase 2: Desktop UI & Composer Staging
- Add **Inspect** toggle button to the browser navigation bar.
- Build floating annotation popover in the browser view (comment text box + "Attach to Chat").
- Update `state/attachments.rs` to support `BrowserAnnotation` chips in composer alongside images and files.
- Render dismissible attachment chip in the active composer pane with preview tooltip.

### Phase 3: Server Prompt Integration & Verification
- Update prompt builder in `server-go` (`internal/agent/prompt/builder.go`) to format `<browser_annotation>` blocks.
- Inject base64 visual thumbnail into multimodal vision-capable models (e.g. Claude 3.7 Sonnet, GPT-4o).
- Add unit tests for serialization, schema validation, and prompt generation.

### Phase 4: Full-Loop Agent Verification
- Wire the agent's `browser` tool so the agent can take follow-up measurements or re-inspect elements after editing code.

---

## 7. Edge Cases & Resilience

- **Shadow DOM & iFrames**: Recursive piercing into open ShadowRoots and same-origin iframes.
- **Dynamic / Re-rendering Pages**: The highlight overlay uses CSS fixed positioning and recalculates on scroll or window resize.
- **Sensitive Inputs**: Passwords and `data-private` input values are sanitized before creating the HTML snippet.
- **Network / URL Changes**: If the user navigates away before attaching, the inspect session resets cleanly.
