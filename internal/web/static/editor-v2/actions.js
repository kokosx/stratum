// actions.js — shared editor actions for outer shell and iframe controls
import { deleteNode, duplicateNode } from "./commands.js";
import { canRedo, canUndo, redo, undo } from "./history.js";
import { commitBeforeEditorContextChange, isInlineEditing } from "./inline-editor.js";
import { findDocumentNode, state } from "./state.js";

function showFailure(result, fallback) {
  const message = result?.reason || fallback;
  try {
    if (message && typeof window !== "undefined" && window.stratumToast) {
      window.stratumToast("error", message);
    }
  } catch (_) {}
}

function finishInlineEditing() {
  if (!isInlineEditing()) return true;
  const committed = commitBeforeEditorContextChange();
  return committed || !isInlineEditing();
}

function editableNodeId(nodeId) {
  const selected = state.selection;
  const candidate = nodeId || selected?.nodeId;
  if (!candidate || typeof candidate !== "string") return null;
  if (!nodeId && selected?.editable === false) return null;
  return findDocumentNode(candidate) ? candidate : null;
}

export function duplicateBlock(nodeId) {
  if (!finishInlineEditing()) return { ok: false, reason: "Finish editing before duplicating this block." };
  const targetId = editableNodeId(nodeId);
  if (!targetId) return { ok: false, reason: "Block not found." };
  const result = duplicateNode({ nodeId: targetId });
  if (!result.ok) showFailure(result, "Could not duplicate block.");
  return result;
}

export function deleteBlock(nodeId) {
  if (!finishInlineEditing()) return { ok: false, reason: "Finish editing before deleting this block." };
  const targetId = editableNodeId(nodeId);
  if (!targetId) return { ok: false, reason: "Block not found." };
  const result = deleteNode({ nodeId: targetId });
  if (!result.ok) showFailure(result, "Could not delete block.");
  return result;
}

export function undoDocument() {
  if (!finishInlineEditing() || !canUndo()) return false;
  return undo();
}

export function redoDocument() {
  if (!finishInlineEditing() || !canRedo()) return false;
  return redo();
}

function isTextInput(target) {
  if (!target || typeof target !== "object") return false;
  try {
    if (target.isContentEditable) return true;
    if (target.closest?.("[contenteditable='true'],[contenteditable='plaintext-only'],textarea,input,select")) return true;
  } catch (_) {}
  return false;
}

export function handleEditorShortcut(event) {
  if (!event || event.defaultPrevented || isInlineEditing() || isTextInput(event.target)) return false;
  const key = String(event.key || "").toLowerCase();
  const modifier = !!(event.metaKey || event.ctrlKey);

  if (modifier && !event.altKey && key === "z") {
    if (event.shiftKey) return redoDocument();
    return undoDocument();
  }
  if (event.ctrlKey && !event.metaKey && !event.altKey && !event.shiftKey && key === "y") {
    return redoDocument();
  }
  if (modifier && !event.altKey && !event.shiftKey && key === "d") {
    const targetId = editableNodeId();
    if (!targetId) return false;
    duplicateBlock(targetId);
    return true;
  }
  if (!modifier && !event.altKey && !event.shiftKey && (key === "delete" || key === "backspace")) {
    const targetId = editableNodeId();
    if (!targetId) return false;
    deleteBlock(targetId);
    return true;
  }
  return false;
}
