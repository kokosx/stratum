// inline-editor.js — M5A/M5B inline content editing (plain + rich)
// Manages contenteditable lifecycle on real rendered field element inside iframe.

import { state, findDocumentNode, definitionForBlock, plainTextFromRichText, placeholderForInlineField, inlineFieldsForNode, isRichTextValid } from "./state.js";
import { updateNodeField } from "./commands.js";
import { renderRichTextToDOM, domToRichText, selectionToOffsets, offsetsToRange, restoreSelectionFromOffsets, toggleMarkInRichText, normalizeRichText, htmlToRichText, insertRichTextAtSelection, isSafeHref } from "./richtext-editor.js";
import * as RichToolbar from "./richtext-toolbar.js";

let active = null; // { fieldEl, nodeId, instanceKey, path, originalValue, originalRichText, mode, canvas, handlers }
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

function attachPlainHandlers(fieldEl, canvas) {
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
    setTimeout(() => {
      if (active && active.fieldEl === fieldEl) commitActiveEdit();
    }, 0);
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
  const onCompositionEnd = () => { composing = false; try { canvas.requestSync(); } catch (_) {} };
  const onKeyDown = (e) => {
    if (composing || e.isComposing) return;
    // Escape priority: popover first, then cancel
    if (e.key === "Escape") {
      if (RichToolbar.isPopoverVisible()) {
        e.preventDefault(); e.stopPropagation(); e.stopImmediatePropagation && e.stopImmediatePropagation();
        // Save offsets for restore? Popover's close will restore
        RichToolbar.hidePopover();
        // Restore selection
        try {
          const saved = RichToolbar.getSavedOffsets();
          if (saved) restoreSelectionFromOffsets(fieldEl, saved.start, saved.end);
        } catch (_) {}
        return;
      }
      e.preventDefault(); e.stopPropagation(); e.stopImmediatePropagation && e.stopImmediatePropagation();
      cancelActiveEdit();
      return;
    }
    if (e.key === "Enter") {
      // M5B: Enter commits, no paragraph
      e.preventDefault(); e.stopPropagation();
      commitActiveEdit();
      return;
    }
    if (e.key === "Tab") {
      e.preventDefault(); commitActiveEdit(); return;
    }
    // Shortcuts
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
      triggerLink();
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
    let html = "";
    let text = "";
    try { html = (e.clipboardData || window.clipboardData).getData("text/html") || ""; } catch (_) {}
    try { text = (e.clipboardData || window.clipboardData).getData("text/plain") || ""; } catch (_) {}
    if (html && html.trim() !== "") {
      const pasted = htmlToRichText(html);
      // If pasted is empty (e.g., script only), fallback to plain
      if (pasted.content.length === 0 && text) {
        const plain = text.replace(/\r\n/g, "\n").replace(/\r/g, "\n").replace(/\n/g, " ");
        // Insert plain via model
        const offsets = selectionToOffsets(fieldEl);
        if (!offsets) return;
        const current = domToRichText(fieldEl);
        const inserted = { version: 1, content: plain ? [{ text: plain }] : [] };
        // Use insert helper
        // For simplicity, use insertRichTextAtSelection
        insertRichTextAtSelection(fieldEl, inserted);
        try { canvas.requestSync(); } catch (_) {}
        updateToolbar();
        return;
      }
      const offsets = selectionToOffsets(fieldEl);
      if (!offsets) {
        // No selection, append
        const current = domToRichText(fieldEl);
        const merged = normalizeRichText({ version: 1, content: [...current.content, ...pasted.content] });
        renderRichTextToDOM(fieldEl, merged);
        const newPos = merged.content.reduce((a, r) => a + r.text.length, 0);
        restoreSelectionFromOffsets(fieldEl, newPos, newPos);
      } else {
        insertRichTextAtSelection(fieldEl, pasted);
      }
    } else if (text) {
      const plain = text.replace(/\r\n/g, "\n").replace(/\r/g, "\n").replace(/\n/g, " ");
      const offsets = selectionToOffsets(fieldEl);
      if (!offsets) return;
      const pasted = { version: 1, content: plain ? [{ text: plain }] : [] };
      insertRichTextAtSelection(fieldEl, pasted);
    }
    try { canvas.requestSync(); } catch (_) {}
    updateToolbar();
  };
  const onDrop = (e) => { try { e.preventDefault(); e.stopPropagation(); } catch (_) {} };
  const onBeforeInput = (e) => {
    // Block paste/drop via beforeinput, but allow typing
    if (e.inputType === "insertFromPaste" || e.inputType === "insertFromDrop" || e.inputType === "insertFromYank") e.preventDefault();
    if (e.inputType === "insertReplacementText" && e.dataTransfer) e.preventDefault();
    // Prevent Enter from inserting <div> or <br>
    if (e.inputType === "insertParagraph" || e.inputType === "insertLineBreak") e.preventDefault();
  };
  const onInput = () => {
    // Sanitize disallowed tags: keep only allowed marks, unwrap others
    // For rich, we allow strong/em/s/code/a, but remove span/div etc. Our domToRichText will handle unwrapping on serialize,
    // but we want to keep DOM clean live as well: remove style/class, unwrap disallowed
    let needsSanitize = false;
    if (fieldEl.querySelector) {
      const disallowed = fieldEl.querySelector("span, div, p, ul, ol, li, h1, h2, h3, h4, h5, h6, blockquote, pre, img, svg, script, style, iframe, figure");
      if (disallowed) needsSanitize = true;
      // Check for style/class attributes on allowed tags
      const withStyle = fieldEl.querySelector("[style], [class]");
      if (withStyle) {
        // Only allow class on fieldEl? For rich we don't want any class on inner marks
        // Remove style/class from allowed tags but keep tag
        fieldEl.querySelectorAll("[style]").forEach(el => el.removeAttribute("style"));
        fieldEl.querySelectorAll("[class]").forEach(el => {
          // Keep no class (except maybe fieldEl's own)
          if (el !== fieldEl) el.removeAttribute("class");
        });
        needsSanitize = true;
      }
    }
    // For now, we don't re-render on every input unless disallowed, to preserve caret
    // But we still need to request geometry
    try { canvas.requestSync(); } catch (_) {}
    updateToolbar();
  };
  const onBlur = () => {
    setTimeout(() => {
      if (active && active.fieldEl === fieldEl) {
        // But toolbar interaction should not commit (check if toolbar/popover active)
        // If popover is visible, don't commit yet
        if (RichToolbar.isPopoverVisible()) return;
        // If click was on toolbar, selection is preserved via mousedown preventDefault, so blur shouldn't have happened for toolbar clicks
        // For outside clicks, commit
        commitActiveEdit();
      }
    }, 0);
  };
  const onClick = (e) => { e.stopPropagation(); };
  const onMouseDown = (e) => { e.stopPropagation(); };
  const onSelectionChange = () => {
    // Only handle when rich editing active and fieldEl is active
    if (!active || active.fieldEl !== fieldEl) return;
    updateToolbar();
    try { canvas.requestSync(); } catch (_) {}
  };
  const onScroll = () => {
    // Hide toolbar if reposition unsafe, let it reappear on next selectionchange
    RichToolbar.hideToolbar();
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
  // selectionchange on document
  try { fieldEl.ownerDocument.addEventListener("selectionchange", onSelectionChange); } catch (_) {}
  try { fieldEl.ownerDocument.defaultView && fieldEl.ownerDocument.defaultView.addEventListener("scroll", onScroll, true); } catch (_) {}
  // Also listen to scroll on fieldEl's ancestors? Use canvas scroll handler already hides toolbar via sync, but we also hide here

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
  handlers.onSelectionChange = onSelectionChange;
  handlers.onScroll = onScroll;
  handlers.fieldEl = fieldEl;
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
  try { if (h.onSelectionChange) el.ownerDocument.removeEventListener("selectionchange", h.onSelectionChange); } catch (_) {}
  try { if (h.onScroll && el.ownerDocument.defaultView) el.ownerDocument.defaultView.removeEventListener("scroll", h.onScroll, true); } catch (_) {}
  try { el.ownerDocument.removeEventListener("selectionchange", h.onSelectionChange); } catch (_) {}
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

// Helpers for rich toolbar
function getActiveMarks() {
  if (!active || !active.fieldEl || active.mode !== "rich") return [];
  const offsets = selectionToOffsets(active.fieldEl);
  if (!offsets || offsets.start === offsets.end) return [];
  const rich = domToRichText(active.fieldEl);
  let pos = 0;
  const selectedRuns = [];
  for (const run of rich.content) {
    const runEnd = pos + run.text.length;
    if (runEnd <= offsets.start || pos >= offsets.end) { pos = runEnd; continue; }
    selectedRuns.push(run);
    pos = runEnd;
  }
  if (selectedRuns.length === 0) return [];
  // Intersection of marks: marks that appear in every selected run
  let common = selectedRuns[0].marks ? [...selectedRuns[0].marks] : [];
  for (let i = 1; i < selectedRuns.length; i++) {
    const cur = selectedRuns[i].marks || [];
    common = common.filter(cm => cur.some(m => m.type === cm.type && (cm.type !== "link" || m.href === cm.href) && (cm.href || "") === (m.href || "")));
  }
  // For link, need to check uniform href
  return common.map(m => m.type);
}

function updateToolbar() {
  if (!active || active.mode !== "rich" || !active.canvas) return;
  const fieldEl = active.fieldEl;
  const offsets = selectionToOffsets(fieldEl);
  if (!offsets || offsets.start === offsets.end) {
    RichToolbar.hideToolbar();
    return;
  }
  // Check selection inside field
  const sel = fieldEl.ownerDocument.getSelection();
  if (!sel || sel.rangeCount === 0) { RichToolbar.hideToolbar(); return; }
  const range = sel.getRangeAt(0);
  if (!fieldEl.contains(range.commonAncestorContainer) && range.commonAncestorContainer !== fieldEl) {
    RichToolbar.hideToolbar(); return;
  }
  let rect = null;
  try { rect = range.getBoundingClientRect(); } catch (_) {}
  if (!rect || (rect.width === 0 && rect.height === 0)) {
    try { rect = fieldEl.getBoundingClientRect(); } catch (_) {}
  }
  const marks = getActiveMarks();
  RichToolbar.showToolbar(active.canvas, fieldEl, rect, marks);
}

function applyRichMark(markType, href) {
  if (!active || active.mode !== "rich") return false;
  const fieldEl = active.fieldEl;
  const offsets = selectionToOffsets(fieldEl);
  if (!offsets || offsets.start === offsets.end) return false;
  const current = domToRichText(fieldEl);
  const updated = toggleMarkInRichText(current, offsets.start, offsets.end, markType, href);
  renderRichTextToDOM(fieldEl, updated);
  restoreSelectionFromOffsets(fieldEl, offsets.start, offsets.end);
  try { active.canvas.requestSync(); } catch (_) {}
  updateToolbar();
  return true;
}

function triggerLink() {
  if (!active || active.mode !== "rich") return;
  const fieldEl = active.fieldEl;
  const offsets = selectionToOffsets(fieldEl);
  if (!offsets || offsets.start === offsets.end) return;
  const current = domToRichText(fieldEl);
  // Find uniform link href in selection
  let href = "";
  let pos = 0;
  let uniformHref = null;
  let uniform = true;
  for (const run of current.content) {
    const runEnd = pos + run.text.length;
    if (runEnd <= offsets.start || pos >= offsets.end) { pos = runEnd; continue; }
    const link = run.marks && run.marks.find(m => m.type === "link");
    const curHref = link ? link.href : null;
    if (uniformHref === null) uniformHref = curHref;
    else if (uniformHref !== curHref) uniform = false;
    pos = runEnd;
  }
  if (uniform && uniformHref) href = uniformHref;
  const saved = { ...offsets };
  RichToolbar.showPopover(active.canvas, fieldEl, href, saved,
    // apply callback
    (mark, val) => {
      // val is href for link
      const curOffsets = RichToolbar.getSavedOffsets() || saved;
      // Restore selection first
      restoreSelectionFromOffsets(fieldEl, curOffsets.start, curOffsets.end);
      if (mark === "link") {
        const cur = domToRichText(fieldEl);
        const updated = toggleMarkInRichText(cur, curOffsets.start, curOffsets.end, "link", val);
        renderRichTextToDOM(fieldEl, updated);
        restoreSelectionFromOffsets(fieldEl, curOffsets.start, curOffsets.end);
        try { active.canvas.requestSync(); } catch (_) {}
        updateToolbar();
      }
    },
    // remove callback
    () => {
      const curOffsets = RichToolbar.getSavedOffsets() || saved;
      restoreSelectionFromOffsets(fieldEl, curOffsets.start, curOffsets.end);
      const cur = domToRichText(fieldEl);
      const updated = toggleMarkInRichText(cur, curOffsets.start, curOffsets.end, "link", null);
      renderRichTextToDOM(fieldEl, updated);
      restoreSelectionFromOffsets(fieldEl, curOffsets.start, curOffsets.end);
      try { active.canvas.requestSync(); } catch (_) {}
      updateToolbar();
    },
    // close callback
    () => {
      try {
        const curOffsets = RichToolbar.getSavedOffsets() || saved;
        if (curOffsets) restoreSelectionFromOffsets(fieldEl, curOffsets.start, curOffsets.end);
      } catch (_) {}
    }
  );
  // Also set toolbar callbacks for link
  RichToolbar.setToolbarCallbacks(
    (mark, hrefVal) => {
      if (mark === "link") triggerLink();
      else applyRichMark(mark);
    },
    () => {
      const curOffsets = RichToolbar.getSavedOffsets() || saved;
      if (curOffsets) restoreSelectionFromOffsets(fieldEl, curOffsets.start, curOffsets.end);
      const cur = domToRichText(fieldEl);
      const updated = toggleMarkInRichText(cur, curOffsets.start, curOffsets.end, "link", null);
      renderRichTextToDOM(fieldEl, updated);
      restoreSelectionFromOffsets(fieldEl, curOffsets.start, curOffsets.end);
      RichToolbar.hidePopover();
      updateToolbar();
    },
    () => {
      RichToolbar.hidePopover();
      try {
        const curOffsets = RichToolbar.getSavedOffsets() || saved;
        if (curOffsets) restoreSelectionFromOffsets(fieldEl, curOffsets.start, curOffsets.end);
      } catch (_) {}
    }
  );
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

export function startInlineEdit(nodeId, instanceKey, canvas, forcedPath) {
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

  active = { fieldEl, nodeId, instanceKey, path, originalValue, originalRichText, mode, canvas, handlers: null };
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

  if (mode === "rich") attachRichHandlers(fieldEl, canvas);
  else attachPlainHandlers(fieldEl, canvas);

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
      if (RichToolbar.isPopoverVisible()) {
        e.preventDefault(); e.stopPropagation(); e.stopImmediatePropagation && e.stopImmediatePropagation();
        RichToolbar.hidePopover();
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
    setTimeout(() => {
      if (active && active.fieldEl === fieldEl) {
        if (RichToolbar.isPopoverVisible()) return;
        commitActiveEdit();
      }
    }, 0);
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
  const onCompositionEnd = () => { composing = false; try { canvas.requestSync(); } catch (_) {} };
  const onKeyDown = (e) => {
    if (composing || e.isComposing) return;
    if (e.key === "Escape") {
      if (RichToolbar.isPopoverVisible()) {
        e.preventDefault(); e.stopPropagation(); e.stopImmediatePropagation && e.stopImmediatePropagation();
        RichToolbar.hidePopover();
        const saved = RichToolbar.getSavedOffsets();
        if (saved) restoreSelectionFromOffsets(fieldEl, saved.start, saved.end);
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
      // toggle bold
      const offsets = selectionToOffsets(fieldEl);
      if (!offsets || offsets.start === offsets.end) return;
      const cur = domToRichText(fieldEl);
      const updated = toggleMarkInRichText(cur, offsets.start, offsets.end, "bold");
      renderRichTextToDOM(fieldEl, updated);
      restoreSelectionFromOffsets(fieldEl, offsets.start, offsets.end);
      try { canvas.requestSync(); } catch (_) {}
      // update toolbar
      return;
    }
    if (mod && !e.shiftKey && e.key.toLowerCase() === "i") {
      e.preventDefault(); e.stopPropagation();
      const offsets = selectionToOffsets(fieldEl);
      if (!offsets || offsets.start === offsets.end) return;
      const cur = domToRichText(fieldEl);
      const updated = toggleMarkInRichText(cur, offsets.start, offsets.end, "italic");
      renderRichTextToDOM(fieldEl, updated);
      restoreSelectionFromOffsets(fieldEl, offsets.start, offsets.end);
      try { canvas.requestSync(); } catch (_) {}
      return;
    }
    if (mod && e.key.toLowerCase() === "k") {
      e.preventDefault(); e.stopPropagation();
      // trigger link UI
      const offsets = selectionToOffsets(fieldEl);
      if (!offsets || offsets.start === offsets.end) return;
      // Save offsets for popover
      const cur = domToRichText(fieldEl);
      let href = "";
      // find uniform href
      let pos = 0, uniformHref = null, uniform = true;
      for (const run of cur.content) {
        const runEnd = pos + run.text.length;
        if (runEnd <= offsets.start || pos >= offsets.end) { pos = runEnd; continue; }
        const link = run.marks && run.marks.find(m => m.type === "link");
        const curHref = link ? link.href : null;
        if (uniformHref === null) uniformHref = curHref;
        else if (uniformHref !== curHref) uniform = false;
        pos = runEnd;
      }
      if (uniform && uniformHref) href = uniformHref;
      RichToolbar.showPopover(canvas, fieldEl, href, offsets,
        (mark, val) => {
          const curOffsets = RichToolbar.getSavedOffsets() || offsets;
          restoreSelectionFromOffsets(fieldEl, curOffsets.start, curOffsets.end);
          const cur2 = domToRichText(fieldEl);
          const updated = toggleMarkInRichText(cur2, curOffsets.start, curOffsets.end, "link", val);
          renderRichTextToDOM(fieldEl, updated);
          restoreSelectionFromOffsets(fieldEl, curOffsets.start, curOffsets.end);
          try { canvas.requestSync(); } catch (_) {}
        },
        () => {
          const curOffsets = RichToolbar.getSavedOffsets() || offsets;
          restoreSelectionFromOffsets(fieldEl, curOffsets.start, curOffsets.end);
          const cur2 = domToRichText(fieldEl);
          const updated = toggleMarkInRichText(cur2, curOffsets.start, curOffsets.end, "link", null);
          renderRichTextToDOM(fieldEl, updated);
          restoreSelectionFromOffsets(fieldEl, curOffsets.start, curOffsets.end);
          try { canvas.requestSync(); } catch (_) {}
        },
        () => {
          try {
            const curOffsets = RichToolbar.getSavedOffsets() || offsets;
            if (curOffsets) restoreSelectionFromOffsets(fieldEl, curOffsets.start, curOffsets.end);
          } catch (_) {}
        }
      );
      return;
    }
    if (mod && e.shiftKey && e.key.toLowerCase() === "x") {
      e.preventDefault(); e.stopPropagation();
      const offsets = selectionToOffsets(fieldEl);
      if (!offsets || offsets.start === offsets.end) return;
      const cur = domToRichText(fieldEl);
      const updated = toggleMarkInRichText(cur, offsets.start, offsets.end, "strike");
      renderRichTextToDOM(fieldEl, updated);
      restoreSelectionFromOffsets(fieldEl, offsets.start, offsets.end);
      try { canvas.requestSync(); } catch (_) {}
      return;
    }
  };
  const onPaste = (e) => {
    try { e.preventDefault(); } catch (_) {}
    let html = "";
    let text = "";
    try { html = (e.clipboardData || window.clipboardData).getData("text/html") || ""; } catch (_) {}
    try { text = (e.clipboardData || window.clipboardData).getData("text/plain") || ""; } catch (_) {}
    if (html && html.trim() !== "") {
      const pasted = htmlToRichText(html);
      if (pasted.content.length === 0 && text) {
        const plain = text.replace(/\r\n/g, "\n").replace(/\r/g, "\n").replace(/\n/g, " ");
        const offsets = selectionToOffsets(fieldEl);
        if (!offsets) return;
        const inserted = { version: 1, content: plain ? [{ text: plain }] : [] };
        insertRichTextAtSelection(fieldEl, inserted);
        try { canvas.requestSync(); } catch (_) {}
        return;
      }
      const offsets = selectionToOffsets(fieldEl);
      if (!offsets) {
        const current = domToRichText(fieldEl);
        const merged = normalizeRichText({ version: 1, content: [...current.content, ...pasted.content] });
        renderRichTextToDOM(fieldEl, merged);
        const newPos = merged.content.reduce((a, r) => a + r.text.length, 0);
        restoreSelectionFromOffsets(fieldEl, newPos, newPos);
      } else {
        insertRichTextAtSelection(fieldEl, pasted);
      }
    } else if (text) {
      const plain = text.replace(/\r\n/g, "\n").replace(/\r/g, "\n").replace(/\n/g, " ");
      const offsets = selectionToOffsets(fieldEl);
      if (!offsets) return;
      const pasted = { version: 1, content: plain ? [{ text: plain }] : [] };
      insertRichTextAtSelection(fieldEl, pasted);
    }
    try { canvas.requestSync(); } catch (_) {}
  };
  const onDrop = (e) => { try { e.preventDefault(); e.stopPropagation(); } catch (_) {} };
  const onBeforeInput = (e) => {
    if (e.inputType === "insertFromPaste" || e.inputType === "insertFromDrop" || e.inputType === "insertFromYank") e.preventDefault();
    if (e.inputType === "insertReplacementText" && e.dataTransfer) e.preventDefault();
    if (e.inputType === "insertParagraph" || e.inputType === "insertLineBreak") e.preventDefault();
  };
  const onInput = () => {
    // Sanitize: remove disallowed tags, keep only allowed
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
      // Re-serialize via RichText to normalize, then re-render, preserving selection
      const offsets = selectionToOffsets(fieldEl);
      const rich = domToRichText(fieldEl);
      renderRichTextToDOM(fieldEl, rich);
      if (offsets) restoreSelectionFromOffsets(fieldEl, offsets.start, offsets.end);
    }
    try { canvas.requestSync(); } catch (_) {}
    // Update toolbar position
    try {
      const offsets = selectionToOffsets(fieldEl);
      if (offsets && offsets.start !== offsets.end) {
        const sel = fieldEl.ownerDocument.getSelection();
        let rect = null;
        if (sel && sel.rangeCount>0) rect = sel.getRangeAt(0).getBoundingClientRect();
        const marks = (() => {
          const rich = domToRichText(fieldEl);
          let pos=0, selectedRuns=[];
          for (const run of rich.content) {
            const runEnd=pos+run.text.length;
            if (runEnd<=offsets.start || pos>=offsets.end) {pos=runEnd; continue;}
            selectedRuns.push(run);
            pos=runEnd;
          }
          if (selectedRuns.length===0) return [];
          let common = selectedRuns[0].marks ? [...selectedRuns[0].marks] : [];
          for (let i=1;i<selectedRuns.length;i++) {
            const cur = selectedRuns[i].marks||[];
            common = common.filter(cm=> cur.some(m=> m.type===cm.type && (cm.type!=="link" || m.href===cm.href)));
          }
          return common.map(m=>m.type);
        })();
        RichToolbar.showToolbar(canvas, fieldEl, rect, marks);
      } else {
        RichToolbar.hideToolbar();
      }
    } catch (_) {}
  };
  const onBlur = () => {
    setTimeout(() => {
      if (RichToolbar.isPopoverVisible()) return;
      if (active && active.fieldEl === fieldEl) commitActiveEdit();
    }, 0);
  };
  const onClick = (e) => { e.stopPropagation(); };
  const onMouseDown = (e) => { e.stopPropagation(); };
  const onSelectionChange = () => {
    if (!active || active.fieldEl !== fieldEl) return;
    // Update toolbar visibility
    try {
      const offsets = selectionToOffsets(fieldEl);
      if (!offsets || offsets.start===offsets.end) { RichToolbar.hideToolbar(); return; }
      const sel = fieldEl.ownerDocument.getSelection();
      if (!sel || sel.rangeCount===0) { RichToolbar.hideToolbar(); return; }
      const range = sel.getRangeAt(0);
      if (!fieldEl.contains(range.commonAncestorContainer) && range.commonAncestorContainer!==fieldEl) { RichToolbar.hideToolbar(); return; }
      const rect = range.getBoundingClientRect();
      const rich = domToRichText(fieldEl);
      let pos=0, selectedRuns=[];
      for (const run of rich.content) {
        const runEnd=pos+run.text.length;
        if (runEnd<=offsets.start || pos>=offsets.end) {pos=runEnd; continue;}
        selectedRuns.push(run);
        pos=runEnd;
      }
      let common=[];
      if (selectedRuns.length>0) {
        common = selectedRuns[0].marks ? [...selectedRuns[0].marks] : [];
        for (let i=1;i<selectedRuns.length;i++) {
          const cur=selectedRuns[i].marks||[];
          common=common.filter(cm=> cur.some(m=> m.type===cm.type && (cm.type!=="link" || m.href===cm.href)));
        }
      }
      const marks = common.map(m=>m.type);
      RichToolbar.showToolbar(canvas, fieldEl, rect, marks);
    } catch (_) { RichToolbar.hideToolbar(); }
    try { canvas.requestSync(); } catch (_) {}
  };
  const onScroll = () => { RichToolbar.hideToolbar(); };
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
  handlers.onBlur = onBlur;
  handlers.onClick = onClick;
  handlers.onMouseDown = onMouseDown;
  handlers.onSelectionChange = onSelectionChange;
  handlers.onScroll = onScroll;
  handlers.fieldEl = fieldEl;
  // Setup toolbar callbacks
  RichToolbar.setToolbarCallbacks(
    (mark, href) => {
      if (mark === "link") {
        // trigger link popover
        const offsets = selectionToOffsets(fieldEl);
        if (!offsets || offsets.start===offsets.end) return;
        const cur = domToRichText(fieldEl);
        let hrefVal = "";
        let pos=0, uniformHref=null, uniform=true;
        for (const run of cur.content) {
          const runEnd=pos+run.text.length;
          if (runEnd<=offsets.start || pos>=offsets.end) {pos=runEnd; continue;}
          const link = run.marks && run.marks.find(m=>m.type==="link");
          const curHref = link ? link.href : null;
          if (uniformHref===null) uniformHref=curHref;
          else if (uniformHref!==curHref) uniform=false;
          pos=runEnd;
        }
        if (uniform && uniformHref) hrefVal=uniformHref;
        RichToolbar.showPopover(canvas, fieldEl, hrefVal, offsets,
          (m, v) => {
            const curOffsets = RichToolbar.getSavedOffsets() || offsets;
            restoreSelectionFromOffsets(fieldEl, curOffsets.start, curOffsets.end);
            const cur2 = domToRichText(fieldEl);
            const updated = toggleMarkInRichText(cur2, curOffsets.start, curOffsets.end, "link", v);
            renderRichTextToDOM(fieldEl, updated);
            restoreSelectionFromOffsets(fieldEl, curOffsets.start, curOffsets.end);
            try { canvas.requestSync(); } catch (_) {}
          },
          () => {
            const curOffsets = RichToolbar.getSavedOffsets() || offsets;
            restoreSelectionFromOffsets(fieldEl, curOffsets.start, curOffsets.end);
            const cur2 = domToRichText(fieldEl);
            const updated = toggleMarkInRichText(cur2, curOffsets.start, curOffsets.end, "link", null);
            renderRichTextToDOM(fieldEl, updated);
            restoreSelectionFromOffsets(fieldEl, curOffsets.start, curOffsets.end);
            try { canvas.requestSync(); } catch (_) {}
          },
          () => {
            try { const curOffsets = RichToolbar.getSavedOffsets() || offsets; if(curOffsets) restoreSelectionFromOffsets(fieldEl, curOffsets.start, curOffsets.end); } catch (_) {}
          }
        );
      } else {
        const offsets = selectionToOffsets(fieldEl);
        if (!offsets || offsets.start===offsets.end) return;
        const cur = domToRichText(fieldEl);
        const updated = toggleMarkInRichText(cur, offsets.start, offsets.end, mark);
        renderRichTextToDOM(fieldEl, updated);
        restoreSelectionFromOffsets(fieldEl, offsets.start, offsets.end);
        try { canvas.requestSync(); } catch (_) {}
      }
    },
    null,
    null
  );
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
