// history.js — bounded in-memory semantic SDT snapshots
import { findDocumentNode, setDocument, state, subscribeDocument } from "./state.js";

const HISTORY_LIMIT = 50;
const undoStack = [];
const redoStack = [];
const listeners = new Set();
let unsubscribeDocument = null;

function serialize(documentValue) {
  try { return JSON.stringify(documentValue); } catch (_) { return ""; }
}

function notifyHistory() {
  const snapshot = { canUndo: canUndo(), canRedo: canRedo() };
  for (const listener of listeners) {
    try { listener(snapshot); } catch (_) {}
  }
}

function pushBounded(stack, snapshot) {
  if (!snapshot) return;
  stack.push(snapshot);
  if (stack.length > HISTORY_LIMIT) stack.shift();
}

function recordDocumentChange(nextDocument, meta) {
  if (meta?.history === "skip") return;
  const previous = meta?.previousDocumentJSON || "";
  const current = serialize(nextDocument);
  if (!previous || !current || previous === current) return;
  pushBounded(undoStack, previous);
  redoStack.length = 0;
  notifyHistory();
}

export function initializeHistory() {
  if (unsubscribeDocument) return unsubscribeDocument;
  unsubscribeDocument = subscribeDocument(recordDocumentChange);
  return unsubscribeDocument;
}

export function resetHistory() {
  undoStack.length = 0;
  redoStack.length = 0;
  notifyHistory();
}

export function canUndo() {
  return undoStack.length > 0;
}

export function canRedo() {
  return redoStack.length > 0;
}

function restore(snapshot, oppositeStack) {
  if (!snapshot) return false;
  const current = serialize(state.document);
  if (!current) return false;
  let documentValue;
  try { documentValue = JSON.parse(snapshot); } catch (_) { return false; }

  pushBounded(oppositeStack, current);
  delete state.__pendingSelectionId;
  delete state.__pendingSelectionIds;
  delete state.__pendingSelectionSuppressInlineId;
  setDocument(documentValue, { renderHint: "structural", history: "skip" });
  if (state.selection?.nodeId && !findDocumentNode(state.selection.nodeId)) {
    state.selection = null;
  }
  notifyHistory();
  return true;
}

export function undo() {
  if (!canUndo()) return false;
  const snapshot = undoStack.pop();
  if (!restore(snapshot, redoStack)) {
    pushBounded(undoStack, snapshot);
    return false;
  }
  return true;
}

export function redo() {
  if (!canRedo()) return false;
  const snapshot = redoStack.pop();
  if (!restore(snapshot, undoStack)) {
    pushBounded(redoStack, snapshot);
    return false;
  }
  return true;
}

export function subscribeHistory(listener) {
  if (typeof listener !== "function") return () => {};
  listeners.add(listener);
  try { listener({ canUndo: canUndo(), canRedo: canRedo() }); } catch (_) {}
  return () => listeners.delete(listener);
}

export { HISTORY_LIMIT };
