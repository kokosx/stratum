// inline-editor.js — M5A basic inline content editing
// Manages contenteditable lifecycle on real rendered field element inside iframe.
// Generic: discovers field via explicit data-stratum-editor-field marker, never by tag.

import { state, findDocumentNode, definitionForBlock, plainTextFromRichText, placeholderForInlineField, inlineFieldsForNode } from "./state.js";
import { updateNodeField } from "./commands.js";

let active = null; // { fieldEl, nodeId, instanceKey, path, originalValue, mode, canvas, handlers }
let composing = false;

function getCurrentPlain(node, path) {
  const key = path.split(".").pop();
  const scope = path.split(".")[0];
  const obj = scope === "props" ? node.props : node.settings;
  if (!obj) return "";
  const raw = obj[key];
  if (raw == null) return "";
  if (typeof raw === "string") return raw;
  if (raw && typeof raw === "object" && raw.version === 1 && Array.isArray(raw.content)) {
    return plainTextFromRichText(raw);
  }
  return String(raw);
}

function findFieldElement(canvas, instance, path) {
  if (!canvas || !canvas.doc || !instance) return null;
  const doc = canvas.doc;
  // Search within each root element
  for (const root of instance.rootElements || []) {
    if (!root) continue;
    try {
      if (root.getAttribute && root.getAttribute("data-stratum-editor-field") === path) return root;
      const found = root.querySelector ? root.querySelector(`[data-stratum-editor-field="${path}"]`) : null;
      if (found) return found;
    } catch (_) {}
  }
  // Fallback: global search and containment check
  try {
    const all = doc.querySelectorAll(`[data-stratum-editor-field="${path}"]`);
    for (const el of all) {
      for (const root of instance.rootElements || []) {
        if (root && root.contains && root.contains(el)) return el;
      }
    }
    // If still not found, return first global if only one
    if (all.length === 1) return all[0];
  } catch (_) {}
  return null;
}

function attachHandlers(fieldEl, canvas) {
  if (!fieldEl) return;
  const handlers = {};

  const onCompositionStart = () => { composing = true; };
  const onCompositionEnd = () => { composing = false; };

  const onKeyDown = (e) => {
    if (composing || e.isComposing) return;
    if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      e.stopImmediatePropagation && e.stopImmediatePropagation();
      cancelActiveEdit();
      return;
    }
    if (e.key === "Enter") {
      // Single-line fields (heading/button) commit; Text plain also commits for M5A
      e.preventDefault();
      e.stopPropagation();
      // For IME, already returned above
      commitActiveEdit();
      return;
    }
    // Tab: commit and allow normal tab handling (do not insert tab)
    if (e.key === "Tab") {
      e.preventDefault();
      commitActiveEdit();
      // after commit, focus may go to next element; keep selection
      return;
    }
  };

  const onPaste = (e) => {
    try { e.preventDefault(); } catch (_) {}
    let text = "";
    try {
      text = (e.clipboardData || window.clipboardData).getData("text/plain") || "";
    } catch (_) { text = ""; }
    text = text.replace(/\r\n/g, "\n").replace(/\r/g, "\n").replace(/\n/g, " ");
    try {
      const sel = fieldEl.ownerDocument.getSelection ? fieldEl.ownerDocument.getSelection() : window.getSelection();
      if (!sel || sel.rangeCount === 0) {
        fieldEl.textContent += text;
        return;
      }
      const range = sel.getRangeAt(0);
      range.deleteContents();
      const textNode = fieldEl.ownerDocument.createTextNode(text);
      range.insertNode(textNode);
      range.setStartAfter(textNode);
      range.collapse(true);
      sel.removeAllRanges();
      sel.addRange(range);
    } catch (_) {
      fieldEl.textContent += text;
    }
  };

  const onDrop = (e) => {
    try { e.preventDefault(); e.stopPropagation(); } catch (_) {}
  };

  const onBeforeInput = (e) => {
    if (e.inputType === "insertFromPaste" || e.inputType === "insertFromDrop" || e.inputType === "insertFromYank") {
      e.preventDefault();
    }
    // Also block insertReplacementText that could be rich
    if (e.inputType === "insertReplacementText" && e.dataTransfer) {
      e.preventDefault();
    }
  };

  const onInput = () => {
    if (fieldEl.querySelector && fieldEl.querySelector("*")) {
      const plain = fieldEl.textContent || "";
      const sel = fieldEl.ownerDocument.getSelection ? fieldEl.ownerDocument.getSelection() : null;
      let start = 0, end = 0;
      try {
        if (sel && sel.rangeCount > 0) {
          const range = sel.getRangeAt(0);
          const pre = fieldEl.ownerDocument.createRange();
          pre.selectNodeContents(fieldEl);
          pre.setEnd(range.startContainer, range.startOffset);
          start = pre.toString().length;
          end = start + range.toString().length;
        }
      } catch (_) {}
      fieldEl.textContent = plain;
      try {
        const sel2 = fieldEl.ownerDocument.getSelection();
        if (sel2) {
          let charIndex = 0;
          let startNode, startOffset, endNode, endOffset;
          const walker = fieldEl.ownerDocument.createTreeWalker(fieldEl, NodeFilter.SHOW_TEXT, null);
          let node;
          while ((node = walker.nextNode())) {
            const next = charIndex + node.textContent.length;
            if (startNode === undefined && start >= charIndex && start <= next) { startNode = node; startOffset = start - charIndex; }
            if (endNode === undefined && end >= charIndex && end <= next) { endNode = node; endOffset = end - charIndex; }
            charIndex = next;
            if (startNode && endNode) break;
          }
          if (startNode && endNode) {
            const r = fieldEl.ownerDocument.createRange();
            r.setStart(startNode, startOffset);
            r.setEnd(endNode, endOffset);
            sel2.removeAllRanges(); sel2.addRange(r);
          }
        }
      } catch (_) {}
    }
  };

  const onBlur = () => {
    // Defer to allow click handling inside canvas to commit first
    setTimeout(() => {
      if (active && active.fieldEl === fieldEl) {
        // If blur was due to commit via Enter, already cleared; otherwise commit
        commitActiveEdit();
      }
    }, 0);
  };

  // Stop propagation for clicks inside field so canvas doesn't re-select
  const onClick = (e) => {
    e.stopPropagation();
  };
  const onMouseDown = (e) => {
    e.stopPropagation();
  };

  fieldEl.addEventListener("compositionstart", onCompositionStart);
  fieldEl.addEventListener("compositionend", onCompositionEnd);
  fieldEl.addEventListener("keydown", onKeyDown, true);
  fieldEl.addEventListener("paste", onPaste);
  fieldEl.addEventListener("drop", onDrop);
  fieldEl.addEventListener("beforeinput", onBeforeInput);
  fieldEl.addEventListener("input", onInput);
  fieldEl.addEventListener("blur", onBlur);
  fieldEl.addEventListener("click", onClick, true);
  fieldEl.addEventListener("mousedown", onMouseDown, true);

  handlers.onCompositionStart = onCompositionStart;
  handlers.onCompositionEnd = onCompositionEnd;
  handlers.onKeyDown = onKeyDown;
  handlers.onPaste = onPaste;
  handlers.onDrop = onDrop;
  handlers.onBeforeInput = onBeforeInput;
  handlers.onInput = onInput;
  handlers.onBlur = onBlur;
  handlers.onClick = onClick;
  handlers.onMouseDown = onMouseDown;

  if (active) active.handlers = handlers;
}

function detachHandlers() {
  if (!active || !active.fieldEl || !active.handlers) return;
  const el = active.fieldEl;
  const h = active.handlers;
  try { el.removeEventListener("compositionstart", h.onCompositionStart); } catch (_) {}
  try { el.removeEventListener("compositionend", h.onCompositionEnd); } catch (_) {}
  try { el.removeEventListener("keydown", h.onKeyDown, true); } catch (_) {}
  try { el.removeEventListener("paste", h.onPaste); } catch (_) {}
  try { el.removeEventListener("drop", h.onDrop); } catch (_) {}
  try { el.removeEventListener("beforeinput", h.onBeforeInput); } catch (_) {}
  try { el.removeEventListener("input", h.onInput); } catch (_) {}
  try { el.removeEventListener("blur", h.onBlur); } catch (_) {}
  try { el.removeEventListener("click", h.onClick, true); } catch (_) {}
  try { el.removeEventListener("mousedown", h.onMouseDown, true); } catch (_) {}
}

function cleanupEditingState() {
  if (!active) return;
  const { fieldEl, canvas } = active;
  detachHandlers();
  try { fieldEl.removeAttribute("contenteditable"); } catch (_) {}
  try { fieldEl.removeAttribute("data-stratum-editing"); } catch (_) {}
  try { if (fieldEl.dataset) delete fieldEl.dataset.originalText; } catch (_) {}
  // Remove placeholder handling class? Keep data-placeholder for future empty
  active = null;
  try { if (state.editing) state.editing = null; } catch (_) {}
  if (canvas) {
    try { canvas.requestSync(); } catch (_) {}
    // Restore overlay selection outline
    try { if (canvas.overlay) canvas.syncGeometry && canvas.syncGeometry(); } catch (_) {}
  }
  composing = false;
}

export function isInlineEditing() {
  return !!active && !!state.editing;
}

export function getActiveFieldElement() {
  return active ? active.fieldEl : null;
}

export function isActiveFieldElement(target) {
  if (!active || !active.fieldEl) return false;
  try {
    if (target === active.fieldEl) return true;
    if (active.fieldEl.contains && active.fieldEl.contains(target)) return true;
  } catch (_) {}
  return false;
}

export function commitActiveEdit() {
  if (!active) return false;
  if (composing) return false;
  const { fieldEl, nodeId, path, originalValue, canvas } = active;
  let plain = "";
  try { plain = fieldEl.textContent || ""; } catch (_) { plain = ""; }
  plain = plain.replace(/\r\n/g, "\n").replace(/\r/g, "\n").replace(/\n/g, " ");
  if (plain.trim() === "") plain = "";
  else if (plain.length > 10000) plain = plain.slice(0, 10000);
  const normalizedOriginal = String(originalValue || "");
  if (plain === normalizedOriginal) {
    cleanupEditingState();
    return true;
  }
  const result = updateNodeField({ nodeId, path, value: plain, renderHint: "defer" });
  if (!result || !result.ok) {
    // On failure, keep editing? For now, cancel and show?
    cleanupEditingState();
    return false;
  }
  if (result.unchanged) {
    cleanupEditingState();
    return true;
  }
  cleanupEditingState();
  // Keep selection on edited block
  try {
    if (canvas && canvas.index) {
      const keys = canvas.nodeToKeys.get(nodeId) || [];
      const editableKeys = keys.filter(k => {
        const inst = canvas.index.get(k);
        return inst && inst.editable;
      });
      if (editableKeys.length === 1) {
        const inst = canvas.index.get(editableKeys[0]);
        if (inst) canvas.selectInstance(inst);
      } else if (canvas.selected && canvas.selected.nodeId === nodeId) {
        // keep existing selection
      }
    }
  } catch (_) {}
  return true;
}

export function cancelActiveEdit() {
  if (!active) return false;
  const { fieldEl, originalValue } = active;
  try { fieldEl.textContent = originalValue || ""; } catch (_) {}
  cleanupEditingState();
  return true;
}

export function startInlineEdit(nodeId, instanceKey, canvas, forcedPath) {
  if (!nodeId || !instanceKey || !canvas) return false;
  const node = findDocumentNode(nodeId);
  if (!node) return false;
  const instance = canvas.index ? canvas.index.get(instanceKey) : null;
  if (!instance || !instance.editable) return false;
  // Never enter for external/read-only
  if (instance.editable === false) return false;
  // Discover inline field
  let path = forcedPath;
  if (!path) {
    const fields = inlineFieldsForNode(node);
    if (fields.length !== 1) return false;
    path = fields[0];
  } else {
    // validate forced path is inline plain safe
    const fields = inlineFieldsForNode(node);
    if (!fields.includes(path)) return false;
  }
  const fieldEl = findFieldElement(canvas, instance, path);
  if (!fieldEl) return false;

  // If already editing same field, do nothing
  if (active && active.nodeId === nodeId && active.path === path && active.instanceKey === instanceKey) {
    return true;
  }
  // Commit current if different
  if (active) {
    commitActiveEdit();
  }

  let originalValue = getCurrentPlain(node, path);
  if (typeof originalValue === "string" && originalValue.trim() === "") originalValue = "";
  // Prepare editing state
  const mode = "plain";
  active = { fieldEl, nodeId, instanceKey, path, originalValue, mode, canvas, handlers: null };
  try { state.editing = { nodeId, instanceKey, path, mode, originalValue }; } catch (_) {}

  // Set placeholder if not present
  try {
    if (!fieldEl.getAttribute("data-placeholder")) {
      const ph = placeholderForInlineField(node.block, path);
      if (ph) fieldEl.setAttribute("data-placeholder", ph);
    }
  } catch (_) {}

  try {
    fieldEl.textContent = originalValue;
  } catch (_) {}

  // Apply contenteditable
  try {
    fieldEl.setAttribute("data-stratum-editing", "true");
    // Prefer plaintext-only where supported, fallback to true
    try {
      fieldEl.setAttribute("contenteditable", "plaintext-only");
      // Test if browser kept it; if not, it will fallback to true automatically but we ensure fallback
      if (fieldEl.contentEditable !== "plaintext-only") {
        fieldEl.setAttribute("contenteditable", "true");
      }
    } catch (_) {
      fieldEl.setAttribute("contenteditable", "true");
    }
  } catch (_) {}

  // Focus and select
  try {
    fieldEl.focus({ preventScroll: true });
  } catch (_) {
    try { fieldEl.focus(); } catch (_) {}
  }
  try {
    const doc = fieldEl.ownerDocument;
    const range = doc.createRange();
    range.selectNodeContents(fieldEl);
    const sel = doc.getSelection ? doc.getSelection() : (fieldEl.ownerDocument.defaultView && fieldEl.ownerDocument.defaultView.getSelection ? fieldEl.ownerDocument.defaultView.getSelection() : null);
    if (sel) {
      sel.removeAllRanges();
      sel.addRange(range);
    }
  } catch (_) {}

  attachHandlers(fieldEl, canvas);

  // Suppress insertion affordances and hover
  try {
    if (canvas.overlay) {
      canvas.overlay.clearInsertion();
      canvas.overlay.clearHover();
    }
    canvas.requestSync && canvas.requestSync();
  } catch (_) {}

  return true;
}

// For tests
export function __resetForTest() {
  if (active) {
    try { detachHandlers(); } catch (_) {}
    active = null;
  }
  composing = false;
  try { if (state.editing) state.editing = null; } catch (_) {}
}
