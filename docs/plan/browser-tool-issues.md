# Browser Tool Diagnostic and Remediation Plan

## Overview

This plan addresses reliability, diagnostic honesty, and synchronization issues in the agent `browser` tool across the Go server backend and Desktop client.

---

## Identified Issues & Diagnosis

1. **Silent Fake Success in Headless/Disconnected Mode**
   - When no desktop client is connected (`handler == nil`), the server currently returns simulated string responses (e.g., `"Navigated to..."`, `"Page content retrieved."`).
   - The agent cannot distinguish between genuine desktop webview actions and simulated placeholders, causing wasted turns and inaccurate reasoning.

2. **Ambiguous Transport vs. Execution Failures**
   - Timeouts waiting for a desktop client response are surfaced in the same manner as script runtime exceptions or DOM errors.
   - The agent cannot determine whether the desktop client disconnected, the request timed out, or JavaScript execution failed in the page context.

3. **Incomplete Selector Reporting in `get_content`**
   - DOM queries with CSS selectors do not provide element match counts or structured feedback when 0 elements match.
   - The agent cannot tell whether a query matched nothing or whether the page content was empty.

4. **Race Conditions in Navigation Readiness (`navigate` vs. Subsequent Actions)**
   - Navigation returns before the target page or webview tab finishes settling or registering in the workspace state.
   - Immediate subsequent actions (`get_content`, `run_js`) fail intermittently with "No active or matching browser tab found".

---

## Step-by-Step Remediation Plan

### Step 1: Make Headless and Unbound Mode Explicit
- **Target Area:** Server Go browser tool implementation.
- **Actions:**
  - Remove fake success placeholder strings for headless mode.
  - Fail fast with an explicit error when no desktop browser client handler is registered.
  - Inform the agent directly that desktop interaction is unavailable and recommend alternative tools (`webFetch`, `webSearch`).

### Step 2: Distinguish Transport Timeouts from Client Errors
- **Target Area:** Server Go decision and browser action response handling.
- **Actions:**
  - Differentiate between transport timeouts (desktop client unresponsive or disconnected) and webview execution errors.
  - Surface distinct error messages for client-level timeouts versus script/DOM evaluation failures.

### Step 3: Enhance `get_content` Selector Feedback
- **Target Area:** Desktop browser action evaluator and DOM inspector script.
- **Actions:**
  - Update DOM query extraction to include matched element counts.
  - Return clear diagnostic feedback when a selector matches 0 elements versus when matching elements are empty.
  - Include structured metadata in the result payload (matching count, target selector, extracted text).

### Step 4: Settle Navigation and Add Tab Resolution Resilience
- **Target Area:** Desktop browser tab management and action dispatch.
- **Actions:**
  - Ensure navigation waits for page load / DOM settlement before acknowledging completion.
  - Add transient retry and polling when locating active browser tabs to prevent race conditions immediately after opening or navigating tabs.

### Step 5: Test and Verification Suite
- **Target Area:** Backend Go test suite and Desktop integration checks.
- **Actions:**
  - Add unit tests verifying that calls without a connected client immediately return the explicit disconnected error.
  - Add unit tests verifying timeout errors are distinguishable from script errors.
  - Verify multi-step sequences (`navigate` immediately followed by `get_content` with selector) succeed reliably.
