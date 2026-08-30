// dragdrop.js — shared canvas/library/navigator DnD coordination
// This module centralizes drop validation so canvas, navigator and library use the same
// mutation helpers from tree.js (insert/move) and respect ChildrenSchema.
import { state } from "./state.js";
import { containerAccepts, isWithin, findNode } from "./tree.js";

export function canDrop(containerNode, block, drag) {
  if (!containerAccepts(containerNode, block, drag)) return false;
  if (drag && drag.type === "node" && containerNode && isWithin(drag.nodeId, containerNode)) return false;
  return true;
}

export function validateDropTarget(containerNode, block, drag, editable = true) {
  if (!editable) return { ok: false, reason: "external" };
  const ok = canDrop(containerNode, block, drag);
  return { ok, reason: ok ? "" : "schema" };
}

export function clearAllDropUI() {
  document.querySelectorAll(".drop-slot--active, .drop-slot--invalid").forEach(s=>s.classList.remove("drop-slot--active","drop-slot--invalid"));
  document.querySelectorAll(".node--droptarget, .navigator-drop-target, .canvas--dragover").forEach(n=>n.classList.remove("node--droptarget","navigator-drop-target","canvas--dragover"));
}
