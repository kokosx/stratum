// inline-edit.js — first-party inline editing adapters (heading/text/button)
// V1 supports core/heading, core/text, core/button label via contenteditable overlay.
// Inline edits update SDT props directly, push history on commit, and debounce server preview.

import { state } from "./state.js";
import { findNode } from "./tree.js";

const INLINE_EDITABLE = {
  "core/heading": { prop: "text", rich: true },
  "core/text": { prop: "text", rich: false },
  "core/button": { prop: "label", rich: false },
};

export function isInlineEditable(node) {
  const cfg = INLINE_EDITABLE[node.block];
  return !!cfg;
}

export function startInlineEdit(nodeId, instanceKey, canvasController) {
  const found = findNode(nodeId);
  if (!found) return false;
  const cfg = INLINE_EDITABLE[found.node.block];
  if (!cfg) return false;
  const info = canvasController ? canvasController.index.get(instanceKey) : null;
  if (!info || !info.editable) return false;
  // Create contenteditable overlay positioned over the node's rect
  const doc = canvasController.iframe.contentDocument;
  if (!doc) return false;
  const rect = info.rects[0];
  if (!rect) return false;
  // Use overlay div in parent (not inside iframe) for editing to avoid sandbox restrictions
  const overlay = document.getElementById("editor-canvas-overlay");
  if (!overlay) return false;
  const editor = document.createElement("div");
  editor.contentEditable = "true";
  editor.className = "inline-edit-overlay";
  editor.style.position = "absolute";
  editor.style.left = rect.left + "px";
  editor.style.top = rect.top + "px";
  editor.style.width = Math.max(120, rect.width) + "px";
  editor.style.minHeight = Math.max(24, rect.height) + "px";
  // Copy theme typography from rendered element for visual continuity
  let renderedEl = null;
  try {
    let probe = info.startComment.nextSibling;
    while (probe && probe.nodeType === Node.COMMENT_NODE) probe = probe.nextSibling;
    if (probe && probe.nodeType === 1) renderedEl = probe;
    else if (info.range) {
      const c = info.range.commonAncestorContainer;
      if (c && c.nodeType===1) renderedEl = c;
    }
  } catch (_) {}
  if (renderedEl) {
    try {
      const cs = doc.defaultView.getComputedStyle(renderedEl);
      const props = ["fontFamily","fontSize","fontWeight","fontStyle","lineHeight","letterSpacing","textAlign","color","textTransform"];
      for (const p of props) {
        const v = cs[p];
        if (v) editor.style[p] = v;
      }
      editor.style.width = Math.max(120, renderedEl.getBoundingClientRect().width) + "px";
    } catch (_) {}
  }
  editor.style.background = "white";
  editor.style.border = "1px solid #3b82f6";
  editor.style.padding = "4px 8px";
  editor.style.zIndex = "9999";
  // Initialize text from props
  let currentText = "";
  if (cfg.rich && found.node.props[cfg.prop] && found.node.props[cfg.prop].version === 1) {
    currentText = (found.node.props[cfg.prop].content || []).map(r=>r.text||"").join("");
  } else {
    currentText = String(found.node.props[cfg.prop] || "");
  }
  editor.textContent = currentText;
  overlay.append(editor);
  editor.focus();
  // Select all
  try {
    const range = document.createRange();
    range.selectNodeContents(editor);
    const sel = window.getSelection();
    sel.removeAllRanges();
    sel.addRange(range);
  } catch (_) {}
  let committed = false;
  function commit() {
    if (committed) return;
    committed = true;
    const newText = editor.textContent || "";
    if (newText !== currentText) {
      // Update SDT
      if (cfg.rich) {
        found.node.props[cfg.prop] = { version: 1, content: [{ text: newText }] };
      } else {
        found.node.props[cfg.prop] = newText;
      }
      if (window.__stratum_maybePushHistory) window.__stratum_maybePushHistory();
      if (window.__stratum_changed) window.__stratum_changed({ tree: false, inspector: false });
      // keep selection
      state.selectedNodeId = nodeId;
      state.selectedInstanceKey = instanceKey;
    }
    editor.remove();
    if (canvasController) canvasController.refresh();
  }
  editor.addEventListener("blur", commit);
  editor.addEventListener("keydown", (e)=>{
    if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); editor.blur(); }
    if (e.key === "Escape") { committed = true; editor.remove(); }
  });
  return true;
}
