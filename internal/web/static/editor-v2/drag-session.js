// drag-session.js — tiny runtime drag state, no globals, shared across canvas/navigator
// State shape: { kind:"node"|"block", nodeId?, instanceKey?, definition?, source:{parentId,index} }

let current = null;

export function startSession(session) {
  if (!session) { current = null; return; }
  current = { ...session };
  if (session.source) current.source = { ...session.source };
  if (session.definition) current.definition = { ...session.definition };
}

export function clearSession() {
  current = null;
}

export function getSession() {
  return current ? { ...current } : null;
}

export function isDragging() {
  return !!current;
}

export function getDragNodeId() {
  return current && current.kind === "node" ? current.nodeId : null;
}
