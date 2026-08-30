// dragdrop.js — shared canvas/library/navigator DnD coordination
import { state } from "./state.js";
import { containerAccepts, isWithin, findNode } from "./tree.js";
import { canInsert, canMove } from "./mutations.js";

export function canDrop(containerNode, block, drag) {
  if (!containerAccepts(containerNode, block, drag)) return false;
  if (drag && drag.type === "node" && containerNode && isWithin(drag.nodeId, containerNode)) return false;
  if (drag && drag.type === "node") {
    const found = findNode(drag.nodeId);
    if (found && found.parent) {
      // quick min check for source
      try {
        const c = canMove(drag.nodeId, containerNode, containerNode ? containerNode.children.length : state.document.nodes.length);
        if (!c.ok) return false;
      } catch (_) {}
    }
  }
  return true;
}

export function validateDropTarget(containerNode, block, drag, editable = true) {
  if (!editable) return { ok: false, reason: "external" };
  if (drag && drag.type === "node") {
    try {
      const targetIdx = containerNode ? containerNode.children.length : state.document.nodes.length;
      const res = canMove(drag.nodeId, containerNode, targetIdx);
      if (!res.ok) return { ok: false, reason: "schema" };
    } catch (_) {}
  }
  const ok = canDrop(containerNode, block, drag);
  return { ok, reason: ok ? "" : "schema" };
}

export function clearAllDropUI() {
  document.querySelectorAll(".drop-slot--active, .drop-slot--invalid").forEach(s=>s.classList.remove("drop-slot--active","drop-slot--invalid"));
  document.querySelectorAll(".node--droptarget, .navigator-drop-target, .canvas--dragover").forEach(n=>n.classList.remove("node--droptarget","navigator-drop-target","canvas--dragover"));
}
