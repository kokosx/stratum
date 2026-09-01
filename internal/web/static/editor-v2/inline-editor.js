// inline-editor.js — M5A/M5B inline content editing (plain + rich)
// Manages contenteditable lifecycle on real rendered field element inside iframe.

import { state, findDocumentNode, definitionForBlock, plainTextFromRichText, placeholderForInlineField, inlineFieldsForNode } from "./state.js";
import { updateNodeField } from "./commands.js";
import { renderRichTextToDOM, domToRichText, selectionToOffsets, offsetsToRange, restoreSelectionFromOffsets, toggleMarkInRichText, normalizeRichText, htmlToRichText, insertRichTextAtSelection, isSafeHref } from "./richtext-editor.js";
import * as RichToolbar from "./richtext-toolbar.js";

let active = null; // rich mode additionally owns { rich: { selection, ui, internalMutation } }
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

function getCurrentRichText(node, path) {
  const key = path.split(".").pop();
  const scope = path.split(".")[0];
  const obj = scope === "props" ? node.props : node.settings;
  if (!obj) return { version: 1, content: [] };
  const raw = obj[key];
  if (raw && typeof raw === "object" && raw.version === 1 && Array.isArray(raw.content)) {
    // Clone
    try { return JSON.parse(JSON.stringify(raw)); } catch (_) { return { version: 1, content: [] }; }
  }
  return { version: 1, content: [] };
}

function findFieldElement(canvas, instance, path) {
  if (!canvas || !canvas.doc || !instance) return null;
  const doc = canvas.doc;
  for (const root of instance.rootElements || []) {
    if (!root) continue;
    try {
      if (root.getAttribute && root.getAttribute("data-stratum-editor-field") === path) return root;
      const found = root.querySelector ? root.querySelector(`[data-stratum-editor-field="${path}"]`) : null;
      if (found) return found;
    } catch (_) {}
  }
  try {
    const all = doc.querySelectorAll(`[data-stratum-editor-field="${path}"]`);
    for (const el of all) {
      for (const root of instance.rootElements || []) {
        if (root && root.contains && root.contains(el)) return el;
      }
    }
    if (all.length === 1) return all[0];
  } catch (_) {}
  return null;
}

function placeCaretFromPoint(fieldEl, clientX, clientY) {
  if (typeof clientX !== "number" || typeof clientY !== "number") return false;
  const doc = fieldEl.ownerDocument;
  if (!doc) return false;
  try {
    let range = null;
    if (typeof doc.caretPositionFromPoint === "function") {
      const pos = doc.caretPositionFromPoint(clientX, clientY);
      if (pos && pos.offsetNode && fieldEl.contains(pos.offsetNode)) {
        range = doc.createRange();
        range.setStart(pos.offsetNode, pos.offset);
        range.collapse(true);
      }
    }
    if (!range && typeof doc.caretRangeFromPoint === "function") {
      const r = doc.caretRangeFromPoint(clientX, clientY);
      if (r && r.startContainer && fieldEl.contains(r.startContainer)) {
        range = r.cloneRange ? r.cloneRange() : r;
        try { range.collapse(true); } catch (_) {}
      }
    }
    if (range) {
      const sel = doc.getSelection ? doc.getSelection() : (doc.defaultView && doc.defaultView.getSelection ? doc.defaultView.getSelection() : null);
      if (sel) {
        sel.removeAllRanges();
        sel.addRange(range);
        return true;
      }
    }
  } catch (_) {}
  return false;
}

function placeCaretAtEnd(fieldEl) {
  try {
    const len = fieldEl.textContent ? fieldEl.textContent.length : 0;
    return restoreSelectionFromOffsets(fieldEl, len, len);
  } catch (_) { return false; }
}

function restoreRichEditingContext(fieldEl, offsets) {
  if (!fieldEl || !offsets || typeof offsets.start !== "number" || typeof offsets.end !== "number") return false;
  // Ensure field remains contenteditable
  try {
    if (!fieldEl.hasAttribute("contenteditable") || fieldEl.getAttribute("contenteditable") === "false") {
      fieldEl.setAttribute("contenteditable", "true");
    }
    if (!fieldEl.hasAttribute("data-stratum-editing")) {
      fieldEl.setAttribute("data-stratum-editing", "true");
    }
  } catch (_) {}
  const doc = fieldEl.ownerDocument;
  if (doc.activeElement !== fieldEl) {
    try {
      fieldEl.focus({ preventScroll: true });
    } catch (_) {
      try { fieldEl.focus(); } catch (_) {}
    }
  }
  let restored = false;
  try {
    restored = restoreSelectionFromOffsets(fieldEl, offsets.start, offsets.end);
  } catch (_) { restored = false; }
  // Verify selection belongs to field
  try {
    const sel = doc.getSelection ? doc.getSelection() : (doc.defaultView && doc.defaultView.getSelection ? doc.defaultView.getSelection() : null);
    if (sel && sel.rangeCount > 0) {
      const range = sel.getRangeAt(0);
      const container = range.commonAncestorContainer;
      if (!fieldEl.contains(container) && container !== fieldEl) {
        restored = false;
      }
    } else {
      restored = false;
    }
  } catch (_) { restored = false; }
  if (active?.rich) active.rich.selection = { start: offsets.start, end: offsets.end };
  return restored;
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
  try { if (h.onBlur) el.removeEventListener("blur", h.onBlur); } catch (_) {}
  try { el.removeEventListener("click", h.onClick, true); } catch (_) {}
  try { if (h.onMouseDown) el.removeEventListener("mousedown", h.onMouseDown, true); } catch (_) {}
  try { if (h.onPointerDown) el.removeEventListener("pointerdown", h.onPointerDown, true); } catch (_) {}
  try { if (h.onMouseDownFallback) el.removeEventListener("mousedown", h.onMouseDownFallback, true); } catch (_) {}
  try { if (h.onSelectionChange) el.ownerDocument.removeEventListener("selectionchange", h.onSelectionChange); } catch (_) {}
  try { if (h.onScroll) el.ownerDocument.removeEventListener("scroll", h.onScroll, true); } catch (_) {}
  try { if (h.onScroll && el.ownerDocument.defaultView) el.ownerDocument.defaultView.removeEventListener("scroll", h.onScroll, true); } catch (_) {}
}

function cleanupEditingState() {
  if (!active) return;
  const { fieldEl, canvas, mode } = active;
  detachHandlers();
  try { fieldEl.removeAttribute("contenteditable"); } catch (_) {}
  try { fieldEl.removeAttribute("data-stratum-editing"); } catch (_) {}
  try { if (fieldEl.dataset) delete fieldEl.dataset.originalText; } catch (_) {}
  if (mode === "rich") {
    try { RichToolbar.hideToolbar(); RichToolbar.hidePopover(); RichToolbar.destroyToolbar && RichToolbar.destroyToolbar(); } catch (_) {}
  }
  active = null;
  try { if (state.editing) state.editing = null; } catch (_) {}
  if (canvas) {
    try { canvas.requestSync(); } catch (_) {}
    try { if (canvas.overlay) canvas.syncGeometry && canvas.syncGeometry(); } catch (_) {}
  }
  composing = false;
}

export function isInlineEditing() {
  return !!active && !!state.editing;
}

export function commitBeforeEditorContextChange() {
  if (!active) return false;
  return commitActiveEdit();
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

export function isActiveEditorSessionEvent(event) {
  if (!active || !event) return false;
  try {
    const path = event.composedPath ? event.composedPath() : [];
    for (const node of path) {
      if (node === active.fieldEl) return true;
      if (node?.getAttribute?.("data-stratum-editor-ui") === "true") return true;
    }
    if (isActiveFieldElement(event.target)) return true;
    if (event.target?.closest?.('[data-stratum-editor-ui="true"]')) return true;
  } catch (_) {}
  return false;
}

// Helpers for rich toolbar
function getActiveMarks() {
  if (!active?.rich || !active.fieldEl) return [];
  const selection = active.rich.selection;
  if (!selection || selection.start === selection.end) return [];
  const rich = domToRichText(active.fieldEl);
  let pos = 0;
  const selectedRuns = [];
  for (const run of rich.content) {
    const runEnd = pos + run.text.length;
    if (runEnd <= selection.start || pos >= selection.end) { pos = runEnd; continue; }
    selectedRuns.push(run);
    pos = runEnd;
  }
  if (selectedRuns.length === 0) return [];
  let common = selectedRuns[0].marks ? [...selectedRuns[0].marks] : [];
  for (let i = 1; i < selectedRuns.length; i++) {
    const cur = selectedRuns[i].marks || [];
    common = common.filter(cm => cur.some(m => m.type === cm.type && (cm.type !== "link" || m.href === cm.href) && (cm.href || "") === (m.href || "")));
  }
  return common.map(m => m.type);
}

function rectForRichSelection() {
  if (!active?.rich?.selection) return null;
  const { fieldEl } = active;
  const { start, end } = active.rich.selection;
  try {
    const range = offsetsToRange(fieldEl, start, end);
    const rect = range?.getBoundingClientRect?.();
    if (rect && (rect.width > 0 || rect.height > 0)) return rect;
  } catch (_) {}
  try { return fieldEl.getBoundingClientRect(); } catch (_) { return null; }
}

function updateRichToolbar() {
  if (!active?.rich || !active.canvas) return;
  if (active.rich.ui === "link") return;
  const selection = active.rich.selection;
  if (!selection || selection.start === selection.end) {
    RichToolbar.hideToolbar();
    if (active.rich.ui !== "link") active.rich.ui = "none";
    return;
  }
  active.rich.ui = "toolbar";
  RichToolbar.showToolbar(active.canvas, rectForRichSelection(), getActiveMarks());
}

function applyRichMark(markType, href) {
  if (!active?.rich) return false;
  if (markType === "link" && href != null && href !== "" && !isSafeHref(href)) return false;
  const session = active;
  const fieldEl = active.fieldEl;
  const offsets = active.rich.selection;
  if (!offsets || offsets.start === offsets.end) return false;
  const len = fieldEl.textContent ? fieldEl.textContent.length : 0;
  let s = Math.max(0, Math.min(offsets.start, len));
  let e = Math.max(0, Math.min(offsets.end, len));
  if (s > e) [s, e] = [e, s];
  if (s === e) return false;
  const start = s;
  const end = e;
  const current = domToRichText(fieldEl);
  const updated = toggleMarkInRichText(current, start, end, markType, href);
  session.rich.internalMutation = true;
  try {
    renderRichTextToDOM(fieldEl, updated);
    restoreRichEditingContext(fieldEl, { start, end });
  } finally {
    session.rich.internalMutation = false;
  }
  if (active !== session) return false;
  session.rich.selection = { start, end };
  try { session.canvas.requestSync(); } catch (_) {}
  updateRichToolbar();
  return true;
}

function openLink() {
  if (!active?.rich) return false;
  const selection = active.rich.selection;
  if (!selection || selection.start === selection.end) return false;
  const current = domToRichText(active.fieldEl);
  let href = "";
  let pos = 0;
  let uniformHref = null;
  let uniform = true;
  for (const run of current.content) {
    const runEnd = pos + run.text.length;
    if (runEnd <= selection.start || pos >= selection.end) { pos = runEnd; continue; }
    const link = run.marks && run.marks.find(m => m.type === "link");
    const curHref = link ? link.href : null;
    if (uniformHref === null) uniformHref = curHref;
    else if (uniformHref !== curHref) uniform = false;
    pos = runEnd;
  }
  if (uniform && uniformHref) href = uniformHref;
  active.rich.ui = "link";
  RichToolbar.showPopover(active.canvas, rectForRichSelection(), href);
  return true;
}

function resumeRichToolbar() {
  if (!active?.rich?.selection) return false;
  const session = active;
  const selection = { ...session.rich.selection };
  RichToolbar.hidePopover();
  session.rich.ui = "toolbar";
  session.rich.internalMutation = true;
  try {
    restoreRichEditingContext(session.fieldEl, selection);
  } finally {
    session.rich.internalMutation = false;
  }
  if (active !== session) return false;
  session.rich.selection = selection;
  updateRichToolbar();
  return true;
}

export function commitActiveEdit() {
  if (!active) return false;
  if (composing) return false;
  const { fieldEl, nodeId, path, originalValue, originalRichText, canvas, mode } = active;
  if (mode === "rich") {
    const currentRich = domToRichText(fieldEl);
    const normalizedOriginal = originalRichText ? JSON.stringify(normalizeRichText(originalRichText)) : JSON.stringify({version:1,content:[]});
    const normalizedCurrent = JSON.stringify(normalizeRichText(currentRich));
    if (normalizedCurrent === normalizedOriginal) {
      cleanupEditingState();
      return true;
    }
    // Validate already done via domToRichText normalize, but also ensure not empty?
    // Check if currentRich is valid (should be)
    const needsRefresh = (() => {
      try {
        const keys = canvas && canvas.nodeToKeys ? canvas.nodeToKeys.get(nodeId) : [];
        return keys && keys.length > 1;
      } catch (_) { return false; }
    })();
    const hint = needsRefresh ? "refresh" : "defer";
    const result = updateNodeField({ nodeId, path, value: currentRich, renderHint: hint });
    if (!result || !result.ok) {
      cleanupEditingState();
      return false;
    }
    if (result.unchanged) {
      cleanupEditingState();
      return true;
    }
    cleanupEditingState();
    // For single occurrence, we kept DOM (defer) and already have rich DOM, so keep selection
    // For multiple, we did refresh, so preview will reload and selection will be restored via pending logic
    // For now, keep selection on edited block (single)
    if (hint === "defer") {
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
          }
        }
      } catch (_) {}
    }
    return true;
  } else {
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
    const needsRefresh = (() => {
      try {
        const keys = canvas && canvas.nodeToKeys ? canvas.nodeToKeys.get(nodeId) : [];
        return keys && keys.length > 1;
      } catch (_) { return false; }
    })();
    const hint = needsRefresh ? "refresh" : "defer";
    const result = updateNodeField({ nodeId, path, value: plain, renderHint: hint });
    if (!result || !result.ok) {
      cleanupEditingState();
      return false;
    }
    if (result.unchanged) {
      cleanupEditingState();
      return true;
    }
    cleanupEditingState();
    if (hint === "defer") {
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
          }
        }
      } catch (_) {}
    }
    return true;
  }
}

export function cancelActiveEdit() {
  if (!active) return false;
  const { fieldEl, originalValue, originalRichText, mode } = active;
  if (mode === "rich") {
    try { renderRichTextToDOM(fieldEl, originalRichText || { version: 1, content: [] }); } catch (_) {}
  } else {
    try { fieldEl.textContent = originalValue || ""; } catch (_) {}
  }
  cleanupEditingState();
  return true;
}

export function startInlineEdit(nodeId, instanceKey, canvas, forcedPath, maybeOpts) {
  if (!nodeId || !instanceKey || !canvas) return false;
  const node = findDocumentNode(nodeId);
  if (!node) return false;
  const instance = canvas.index ? canvas.index.get(instanceKey) : null;
  if (!instance || !instance.editable) return false;
  if (instance.editable === false) return false;
  let path = forcedPath;
  if (!path) {
    const fields = inlineFieldsForNode(node);
    if (fields.length !== 1) return false;
    path = fields[0];
  } else {
    const fields = inlineFieldsForNode(node);
    if (!fields.includes(path)) return false;
  }
  const fieldEl = findFieldElement(canvas, instance, path);
  if (!fieldEl) return false;
  if (active && active.nodeId === nodeId && active.path === path && active.instanceKey === instanceKey) {
    return true;
  }
  if (active) {
    commitActiveEdit();
  }
  const def = definitionForBlock(node.block, node.version);
  const fieldMeta = def?.schema?.editor?.fields?.[path];
  const inlineMode = fieldMeta?.inlineMode || "plain";
  const isRich = inlineMode === "rich" && fieldMeta?.control === "richtext";
  let originalValue = "";
  let originalRichText = null;
  let mode = "plain";
  if (isRich) {
    mode = "rich";
    originalRichText = getCurrentRichText(node, path);
    // Normalize for comparison
    originalRichText = normalizeRichText(originalRichText);
    // Render current rich text into fieldEl before editing
    renderRichTextToDOM(fieldEl, originalRichText);
  } else {
    originalValue = getCurrentPlain(node, path);
    if (typeof originalValue === "string" && originalValue.trim() === "") originalValue = "";
    try { fieldEl.textContent = originalValue; } catch (_) {}
  }

  active = {
    fieldEl,
    nodeId,
    instanceKey,
    path,
    originalValue,
    originalRichText,
    mode,
    canvas,
    handlers: null,
    rich: mode === "rich" ? { selection: null, ui: "none", internalMutation: false } : null,
  };
  try { state.editing = { nodeId, instanceKey, path, mode, originalValue: mode === "rich" ? originalRichText : originalValue }; } catch (_) {}

  try {
    if (!fieldEl.getAttribute("data-placeholder")) {
      const ph = placeholderForInlineField(node.block, path);
      if (ph) fieldEl.setAttribute("data-placeholder", ph);
    }
  } catch (_) {}

  try {
    fieldEl.setAttribute("data-stratum-editing", "true");
    if (mode === "rich") {
      fieldEl.setAttribute("contenteditable", "true");
    } else {
      try {
        fieldEl.setAttribute("contenteditable", "plaintext-only");
        if (fieldEl.contentEditable !== "plaintext-only") {
          fieldEl.setAttribute("contenteditable", "true");
        }
      } catch (_) {
        fieldEl.setAttribute("contenteditable", "true");
      }
    }
  } catch (_) {}

  try {
    fieldEl.focus({ preventScroll: true });
  } catch (_) {
    try { fieldEl.focus(); } catch (_) {}
  }
  // Place caret: preserve pointer position if provided, else at end (keyboard), never select-all.
  try {
    let placed = false;
    if (maybeOpts && (typeof maybeOpts.clientX === "number" || typeof maybeOpts.x === "number")) {
      const x = typeof maybeOpts.clientX === "number" ? maybeOpts.clientX : maybeOpts.x;
      const y = typeof maybeOpts.clientY === "number" ? maybeOpts.clientY : maybeOpts.y;
      placed = placeCaretFromPoint(fieldEl, x, y);
    }
    if (!placed) {
      if (!placeCaretAtEnd(fieldEl)) {
        const doc = fieldEl.ownerDocument;
        const range = doc.createRange();
        range.selectNodeContents(fieldEl);
        range.collapse(false);
        const sel = doc.getSelection ? doc.getSelection() : (doc.defaultView && doc.defaultView.getSelection ? doc.defaultView.getSelection() : null);
        if (sel) { sel.removeAllRanges(); sel.addRange(range); }
      }
    }
  } catch (_) {}

  if (mode === "rich") {
    attachRichHandlers(fieldEl, canvas);
    const initialSelection = selectionToOffsets(fieldEl);
    if (initialSelection) active.rich.selection = { ...initialSelection };
  } else {
    attachPlainHandlers(fieldEl, canvas);
  }

  try {
    if (canvas.overlay) {
      canvas.overlay.clearInsertion();
      canvas.overlay.clearHover();
    }
    canvas.requestSync && canvas.requestSync();
  } catch (_) {}

  return true;
}

// Keep old names for compatibility
function attachPlainHandlers(fieldEl, canvas) {
  // Reuse plain handlers (extracted)
  if (!fieldEl) return;
  const handlers = {};
  const onCompositionStart = () => { composing = true; };
  const onCompositionEnd = () => { composing = false; try { canvas.requestSync(); } catch (_) {} };
  const onKeyDown = (e) => {
    if (composing || e.isComposing) return;
    if (e.key === "Escape") {
      e.preventDefault(); e.stopPropagation(); e.stopImmediatePropagation && e.stopImmediatePropagation();
      cancelActiveEdit();
      return;
    }
    if (e.key === "Enter") {
      e.preventDefault(); e.stopPropagation();
      commitActiveEdit();
      return;
    }
    if (e.key === "Tab") {
      e.preventDefault(); commitActiveEdit(); return;
    }
  };
  const onPaste = (e) => {
    try { e.preventDefault(); } catch (_) {}
    let text = "";
    try { text = (e.clipboardData || window.clipboardData).getData("text/plain") || ""; } catch (_) { text = ""; }
    text = text.replace(/\r\n/g, "\n").replace(/\r/g, "\n").replace(/\n/g, " ");
    try {
      const sel = fieldEl.ownerDocument.getSelection ? fieldEl.ownerDocument.getSelection() : window.getSelection();
      if (!sel || sel.rangeCount === 0) {
        fieldEl.textContent += text;
      } else {
        const range = sel.getRangeAt(0);
        range.deleteContents();
        const textNode = fieldEl.ownerDocument.createTextNode(text);
        range.insertNode(textNode);
        range.setStartAfter(textNode);
        range.collapse(true);
        sel.removeAllRanges();
        sel.addRange(range);
      }
    } catch (_) { try { fieldEl.textContent += text; } catch (_) {} }
    try { canvas.requestSync(); } catch (_) {}
  };
  const onDrop = (e) => { try { e.preventDefault(); e.stopPropagation(); } catch (_) {} };
  const onBeforeInput = (e) => {
    if (e.inputType === "insertFromPaste" || e.inputType === "insertFromDrop" || e.inputType === "insertFromYank") e.preventDefault();
    if (e.inputType === "insertReplacementText" && e.dataTransfer) e.preventDefault();
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
          let charIndex = 0, startNode, startOffset, endNode, endOffset;
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
    try { canvas.requestSync(); } catch (_) {}
  };
  const onBlur = () => {
    queueMicrotask(() => {
      if (active && active.fieldEl === fieldEl) {
        commitActiveEdit();
      }
    });
  };
  const onClick = (e) => { e.stopPropagation(); };
  const onMouseDown = (e) => { e.stopPropagation(); };
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

function attachRichHandlers(fieldEl, canvas) {
  if (!fieldEl) return;
  const handlers = {};
  const onCompositionStart = () => { composing = true; };
  const onCompositionEnd = () => {
    composing = false;
    captureBrowserSelection();
    try { canvas.requestSync(); } catch (_) {}
  };

  const captureBrowserSelection = () => {
    if (!active?.rich || active.fieldEl !== fieldEl || active.rich.internalMutation) return false;
    if (active.rich.ui === "link") return false;
    const offsets = selectionToOffsets(fieldEl);
    if (!offsets) return false;
    active.rich.selection = { start: offsets.start, end: offsets.end };
    updateRichToolbar();
    try { canvas.requestSync(); } catch (_) {}
    return true;
  };

  const onKeyDown = (e) => {
    if (composing || e.isComposing) return;
    if (e.key === "Escape") {
      if (RichToolbar.isPopoverVisible()) {
        e.preventDefault(); e.stopPropagation(); e.stopImmediatePropagation && e.stopImmediatePropagation();
        resumeRichToolbar();
        return;
      }
      e.preventDefault(); e.stopPropagation(); e.stopImmediatePropagation && e.stopImmediatePropagation();
      cancelActiveEdit();
      return;
    }
    if (e.key === "Enter") {
      e.preventDefault(); e.stopPropagation();
      commitActiveEdit();
      return;
    }
    if (e.key === "Tab") {
      e.preventDefault(); commitActiveEdit(); return;
    }
    const mod = e.metaKey || e.ctrlKey;
    if (mod && !e.shiftKey && e.key.toLowerCase() === "b") {
      e.preventDefault(); e.stopPropagation();
      applyRichMark("bold");
      return;
    }
    if (mod && !e.shiftKey && e.key.toLowerCase() === "i") {
      e.preventDefault(); e.stopPropagation();
      applyRichMark("italic");
      return;
    }
    if (mod && e.key.toLowerCase() === "k") {
      e.preventDefault(); e.stopPropagation();
      openLink();
      return;
    }
    if (mod && e.shiftKey && e.key.toLowerCase() === "x") {
      e.preventDefault(); e.stopPropagation();
      applyRichMark("strike");
      return;
    }
  };
  const onPaste = (e) => {
    try { e.preventDefault(); } catch (_) {}
    captureBrowserSelection();
    let html = "";
    let text = "";
    try { html = (e.clipboardData || window.clipboardData).getData("text/html") || ""; } catch (_) {}
    try { text = (e.clipboardData || window.clipboardData).getData("text/plain") || ""; } catch (_) {}
    const session = active;
    if (!session?.rich) return;
    session.rich.internalMutation = true;
    try {
      if (html && html.trim() !== "") {
        let pasted = htmlToRichText(html);
        if (pasted.content.length === 0 && text) {
          const plain = text.replace(/\r\n/g, "\n").replace(/\r/g, "\n").replace(/\n/g, " ");
          pasted = { version: 1, content: plain ? [{ text: plain }] : [] };
        }
        if (!insertRichTextAtSelection(fieldEl, pasted)) {
          const current = domToRichText(fieldEl);
          const merged = normalizeRichText({ version: 1, content: [...current.content, ...pasted.content] });
          renderRichTextToDOM(fieldEl, merged);
          const newPosition = merged.content.reduce((total, run) => total + run.text.length, 0);
          restoreSelectionFromOffsets(fieldEl, newPosition, newPosition);
        }
      } else if (text) {
        const plain = text.replace(/\r\n/g, "\n").replace(/\r/g, "\n").replace(/\n/g, " ");
        insertRichTextAtSelection(fieldEl, { version: 1, content: plain ? [{ text: plain }] : [] });
      }
    } finally {
      session.rich.internalMutation = false;
    }
    captureBrowserSelection();
    try { canvas.requestSync(); } catch (_) {}
  };
  const onDrop = (e) => { try { e.preventDefault(); e.stopPropagation(); } catch (_) {} };
  const onBeforeInput = (e) => {
    if (e.inputType === "insertFromPaste" || e.inputType === "insertFromDrop" || e.inputType === "insertFromYank") e.preventDefault();
    if (e.inputType === "insertReplacementText" && e.dataTransfer) e.preventDefault();
    if (e.inputType === "insertParagraph" || e.inputType === "insertLineBreak") e.preventDefault();
  };
  const onInput = () => {
    captureBrowserSelection();
    let needsSanitize = false;
    if (fieldEl.querySelector) {
      const disallowed = fieldEl.querySelector("span, div, p, ul, ol, li, h1, h2, h3, h4, h5, h6, blockquote, pre, img, svg, script, style, iframe, figure, table, tr, td, th");
      if (disallowed) needsSanitize = true;
      const withStyle = fieldEl.querySelector("[style]");
      if (withStyle) {
        fieldEl.querySelectorAll("[style]").forEach(el => el.removeAttribute("style"));
        needsSanitize = true;
      }
      const withClass = fieldEl.querySelector("[class]");
      if (withClass) {
        // Allow no class on inner marks, but keep fieldEl's own data attributes
        fieldEl.querySelectorAll("[class]").forEach(el => { if (el !== fieldEl) el.removeAttribute("class"); });
      }
    }
    if (needsSanitize) {
      const session = active;
      const offsets = session?.rich?.selection ? { ...session.rich.selection } : null;
      const rich = domToRichText(fieldEl);
      if (session?.rich) session.rich.internalMutation = true;
      try {
        renderRichTextToDOM(fieldEl, rich);
        if (offsets) restoreSelectionFromOffsets(fieldEl, offsets.start, offsets.end);
      } finally {
        if (session?.rich) session.rich.internalMutation = false;
      }
    }
    try { canvas.requestSync(); } catch (_) {}
    updateRichToolbar();
  };
  const onClick = (e) => { e.stopPropagation(); };
  const onPointerDown = (e) => {
    e.stopPropagation();
  };
  const onSelectionChange = () => {
    captureBrowserSelection();
  };
  const onScroll = () => {
    if (!active?.rich || active.rich.ui !== "toolbar") return;
    RichToolbar.hideToolbar();
    active.rich.ui = "none";
  };
  fieldEl.addEventListener("compositionstart", onCompositionStart);
  fieldEl.addEventListener("compositionend", onCompositionEnd);
  fieldEl.addEventListener("keydown", onKeyDown, true);
  fieldEl.addEventListener("paste", onPaste);
  fieldEl.addEventListener("drop", onDrop);
  fieldEl.addEventListener("beforeinput", onBeforeInput);
  fieldEl.addEventListener("input", onInput);
  fieldEl.addEventListener("click", onClick, true);
  fieldEl.addEventListener("pointerdown", onPointerDown, true);
  fieldEl.addEventListener("mousedown", onPointerDown, true);
  try { fieldEl.ownerDocument.addEventListener("selectionchange", onSelectionChange); } catch (_) {}
  try { fieldEl.ownerDocument.defaultView && fieldEl.ownerDocument.defaultView.addEventListener("scroll", onScroll, true); } catch (_) {}
  try { fieldEl.ownerDocument.addEventListener("scroll", onScroll, true); } catch (_) {}
  handlers.onCompositionStart = onCompositionStart;
  handlers.onCompositionEnd = onCompositionEnd;
  handlers.onKeyDown = onKeyDown;
  handlers.onPaste = onPaste;
  handlers.onDrop = onDrop;
  handlers.onBeforeInput = onBeforeInput;
  handlers.onInput = onInput;
  handlers.onClick = onClick;
  handlers.onPointerDown = onPointerDown;
  handlers.onMouseDownFallback = onPointerDown;
  handlers.onSelectionChange = onSelectionChange;
  handlers.onScroll = onScroll;
  handlers.fieldEl = fieldEl;
  RichToolbar.setToolbarCallbacks({
    toggleMark: (mark) => applyRichMark(mark),
    openLink: () => openLink(),
    applyLink: (href) => {
      if (!active?.rich) return;
      RichToolbar.hidePopover();
      active.rich.ui = "toolbar";
      applyRichMark("link", href);
    },
    removeLink: () => {
      if (!active?.rich) return;
      RichToolbar.hidePopover();
      active.rich.ui = "toolbar";
      applyRichMark("link", null);
    },
    closeLink: () => resumeRichToolbar(),
  });

  if (active) active.handlers = handlers;
}

// For tests
export function __resetForTest() {
  if (active) {
    try { detachHandlers(); } catch (_) {}
    active = null;
  }
  composing = false;
  try { if (state.editing) state.editing = null; } catch (_) {}
  try { RichToolbar.hideToolbar(); RichToolbar.hidePopover(); } catch (_) {}
}
