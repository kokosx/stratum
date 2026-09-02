// mutations.js — central schema-safe factory + structural validators
// Single source of truth for min/max/allowed/mode checks. Canvas/Navigator/Library/Editor must use these.
import { state, definitions, definitionFor, defaultValue, hydrateObject } from "./state.js";
import { findNode, isContainer, randomID } from "./tree.js";

function ruleFor(def) { return def?.schema?.children || { mode: "none" }; }

function displayNameFor(def, fallback="block") {
  return def?.displayName || fallback;
}

function canChildAcceptParent(block, parentNode) {
  const childDef = definitionForBlock(block);
  if (!childDef) return { ok: true, reason: "" };
  const parents = childDef.schema?.placement?.parents;
  if (!parents || parents.length === 0) return { ok: true, reason: "" };
  if (!parentNode) {
    const label = childDef.displayName || block;
    return { ok: false, reason: `"${label}" cannot be placed at the top level.` };
  }
  if (!parents.includes(parentNode.block)) {
    const parentDef = definitionFor(parentNode);
    const parentLabel = parentDef?.displayName || parentNode.block;
    const childLabel = childDef.displayName || block;
    return { ok: false, reason: `"${childLabel}" cannot be placed inside "${parentLabel}".` };
  }
  return { ok: true, reason: "" };
}

// Resolve definition by block name (latest version)
function definitionForBlock(block) {
  let best = null;
  let bestVer = -1;
  for (const def of definitions.values()) {
    if (def.block === block && def.version > bestVer) {
      best = def;
      bestVer = def.version;
    }
  }
  if (best) return best;
  // fallback to catalog (maybe not yet in definitions map)
  for (const d of state.catalog) {
    if (d.block === block && d.version > bestVer) {
      best = d;
      bestVer = d.version;
    }
  }
  return best;
}

export function createValidNode(definition) {
  const node = {
    id: randomID(),
    block: definition.block,
    version: definition.version,
    props: defaultValue(definition.schema.props),
    settings: defaultValue(definition.schema.settings),
    children: [],
  };
  // form single-option auto-fill (keep legacy)
  try {
    const b = (typeof bootstrap !== 'undefined' ? bootstrap : null) || JSON.parse(document.getElementById("editor-bootstrap").textContent);
    if (definition.block === "core/form") {
      const forms = b.forms || [];
      const active = forms.filter((o) => !String(o.label).endsWith(" (Disabled)"));
      if ((!node.settings.formId || node.settings.formId === "") && active.length === 1) {
        node.settings.formId = active[0].value;
      }
    }
  } catch (_) {}

  // Ensure required children per schema (schema-driven, no hard-coded block names)
  const rule = ruleFor(definition);
  const starters = definition.schema?.editor?.starterChildren;
  if (Array.isArray(starters) && starters.length) {
    for (const sc of starters) {
      const childDef = definitions.get(`${sc.block}@${sc.version}`) || definitionForBlock(sc.block);
      if (!childDef) {
        console.warn(`createValidNode: missing definition for starter ${sc.block}@${sc.version}`);
        continue;
      }
      const child = createValidNode(childDef);
      if (sc.version && child.version !== sc.version) child.version = sc.version;
      node.children.push(child);
    }
    // If starterChildren were configured but none could be created, fall through to implicit min handling
    if (node.children.length) return node;
  }
  // 2) Implicit: single allowed block + min>0 → create min children
  if (rule.min && rule.min > 0 && rule.mode === "allowed" && Array.isArray(rule.blocks) && rule.blocks.length === 1) {
    const needed = rule.min;
    const childBlock = rule.blocks[0];
    const childDef = definitionForBlock(childBlock);
    if (childDef) {
      for (let i=0;i<needed;i++) {
        const child = createValidNode(childDef);
        node.children.push(child);
      }
    }
  }
  return node;
}

// Helpers for validation messages
function minReason(parentDef) {
  const rule = ruleFor(parentDef);
  const min = rule.min;
  const label = displayNameFor(parentDef, "Container");
  const unit = min === 1 ? "child block" : "child blocks";
  // Try to name allowed child nicely when single type
  if (rule.blocks && rule.blocks.length===1) {
    const childDef = definitionForBlock(rule.blocks[0]);
    const childLabel = childDef ? childDef.displayName : rule.blocks[0];
    return `${label} requires at least ${min} ${childLabel}.`;
  }
  return `${label} requires at least ${min} ${unit}.`;
}
function maxReason(parentDef) {
  const rule = ruleFor(parentDef);
  const max = rule.max;
  const label = displayNameFor(parentDef, "Container");
  const unit = max === 1 ? "child block" : "child blocks";
  return `${label} allows at most ${max} ${unit}.`;
}

export function canInsert(parentNode, block, index, drag = null) {
  // parentNode null => root: enforce placement.parents
  if (!parentNode) {
    const placement = canChildAcceptParent(block, null);
    if (!placement.ok) return placement;
    return { ok: true, reason: "" };
  }
  const def = definitionFor(parentNode);
  if (!def) return { ok: false, reason: "Unknown container." };
  const placementEarly = canChildAcceptParent(block, parentNode);
  if (!placementEarly.ok) return placementEarly;
  const rule = ruleFor(def);
  if (rule.mode === "none") return { ok: false, reason: `${displayNameFor(def)} does not allow child blocks.` };
  if (rule.mode === "allowed" && !rule.blocks.includes(block)) {
    const childDef = definitionForBlock(block);
    const childLabel = childDef ? `"${childDef.displayName}"` : `"${block}"`;
    return { ok: false, reason: `${childLabel} is not allowed inside ${displayNameFor(def)}.` };
  }
  if (rule.max !== undefined && rule.max !== null) {
    const count = parentNode.children ? parentNode.children.length : 0;
    // For moves within same container, count will decrease by 1 before insert
    const sameContainer = drag && drag.type === "node" && findNode(drag.nodeId)?.parent === parentNode;
    const effective = sameContainer ? count : count + 1;
    // When moving within same, insertion doesn't increase count, so allow
    if (!sameContainer && count >= rule.max) return { ok: false, reason: maxReason(def) };
  }
  return { ok: true, reason: "" };
}

export function canInsertRoots(parentNode, roots, drag = null) {
  for (const r of roots) {
    const placement = canChildAcceptParent(r.block, parentNode);
    if (!placement.ok) return placement;
  }
  if (!parentNode) return { ok: true, reason: "" };
  const def = definitionFor(parentNode);
  if (!def) return { ok: false, reason: "Unknown container." };
  const rule = ruleFor(def);
  if (rule.mode === "none") return { ok: false, reason: `${displayNameFor(def)} does not allow child blocks.` };
  if (rule.max !== undefined && rule.max !== null) {
    const needed = roots.length;
    const count = parentNode.children.length;
    const sameContainer = false; // patterns are new nodes, not moves
    if (count + needed > rule.max) return { ok: false, reason: maxReason(def) };
  }
  if (rule.mode === "allowed") {
    for (const r of roots) {
      if (!rule.blocks.includes(r.block)) {
        const childDef = definitionForBlock(r.block);
        const childLabel = childDef ? `"${childDef.displayName}"` : `"${r.block}"`;
        return { ok: false, reason: `${childLabel} is not allowed inside ${displayNameFor(def)}.` };
      }
    }
  }
  return { ok: true, reason: "" };
}

export function canRemove(nodeId) {
  const found = findNode(nodeId);
  if (!found) return { ok: false, reason: "Block not found." };
  const parent = found.parent;
  if (!parent) return { ok: true, reason: "" }; // root removal always allowed
  const parentDef = definitionFor(parent);
  if (!parentDef) return { ok: true, reason: "" };
  const rule = ruleFor(parentDef);
  if (rule.min !== undefined && rule.min !== null) {
    const after = parent.children.length - 1;
    if (after < rule.min) return { ok: false, reason: minReason(parentDef) };
  }
  return { ok: true, reason: "" };
}

export function canMove(nodeId, targetParentNode, targetIndex) {
  const found = findNode(nodeId);
  if (!found) return { ok: false, reason: "Block not found." };
  const movingNode = found.node;
  const block = movingNode.block;
  if (targetParentNode) {
    function contains(ancestor, targetId) {
      if (!ancestor) return false;
      if (ancestor.id === targetId) return true;
      for (const ch of ancestor.children || []) if (contains(ch, targetId)) return true;
      return false;
    }
    if (contains(movingNode, targetParentNode.id)) return { ok: false, reason: "Cannot move a block inside itself." };
  }
  const sameParent = found.parent === targetParentNode;
  // source must remain valid after removal — skip if same parent (net count unchanged)
  if (!sameParent) {
    const canRem = canRemove(nodeId);
    if (!canRem.ok) return canRem;
  }
  const drag = { type: "node", nodeId };
  // For same-parent moves, need to handle index adjustment: if moving forward, effective insertion index decreases by 1
  let effectiveIndex = targetIndex;
  if (sameParent && found.index < targetIndex) effectiveIndex = targetIndex - 1;
  if (sameParent && found.index === effectiveIndex) {
    return { ok: false, reason: "Already at that position." };
  }
  const canIns = canInsert(targetParentNode, block, effectiveIndex, drag);
  if (!canIns.ok) return canIns;
  if (found.parent === targetParentNode && found.index === targetIndex) {
    return { ok: false, reason: "Already at that position." };
  }
  return { ok: true, reason: "" };
}

export function canDuplicate(nodeId) {
  const found = findNode(nodeId);
  if (!found) return { ok: false, reason: "Block not found." };
  const parent = found.parent;
  const block = found.node.block;
  // Duplicate adds one more sibling after found
  const drag = null; // duplicate is new node
  return canInsert(parent, block, found.index+1, drag);
}

export function canIndent(nodeId) {
  const found = findNode(nodeId);
  if (!found || found.index < 1) return { ok: false, reason: "No previous sibling to indent into." };
  const prev = found.siblings[found.index-1];
  // prev must be container
  if (!isContainer(prev)) return { ok: false, reason: `${displayNameFor(definitionFor(prev), prev.block)} cannot contain blocks.` };
  return canMove(nodeId, prev, prev.children.length);
}

export function canOutdent(nodeId) {
  const found = findNode(nodeId);
  if (!found || !found.parent) return { ok: false, reason: "Already at root." };
  const parentFound = findNode(found.parent.id);
  if (!parentFound) return { ok: false, reason: "Parent not found." };
  const newParent = parentFound.parent;
  return canMove(nodeId, newParent, parentFound.index+1);
}

// Whether a parent boundary could accept at least one insertable block.
// Uses real catalog probe (not just mode check) – returns false when max reached
// or allowed list empty. Root checks placement.
export function hasLegalInsertion(parentNode, index) {
  if (!parentNode) {
    for (const d of definitions.values()) {
      if (d.hidden) continue;
      if (canInsert(null, d.block, index).ok) return true;
    }
    for (const d of state.catalog) {
      if (d.hidden) continue;
      if (canInsert(null, d.block, index).ok) return true;
    }
    return false;
  }
  const def = definitionFor(parentNode);
  if (!def) return false;
  const rule = ruleFor(def);
  if (rule.mode === "none") return false;
  if (rule.max != null && parentNode.children && parentNode.children.length >= rule.max) return false;
  if (rule.mode === "any") {
    for (const d of definitions.values()) {
      if (d.hidden) continue;
      if (canInsert(parentNode, d.block, index).ok) return true;
    }
    for (const d of state.catalog) {
      if (d.hidden) continue;
      if (canInsert(parentNode, d.block, index).ok) return true;
    }
    return false;
  }
  if (rule.mode === "allowed") {
    if (!Array.isArray(rule.blocks) || rule.blocks.length === 0) return false;
    // Check if at least one allowed block is actually available in catalog and not hidden,
    // and canInsert would succeed for that block.
    for (const blk of rule.blocks) {
      let defForBlk = null;
      for (const d of definitions.values()) if (d.block === blk) { defForBlk = d; break; }
      if (!defForBlk) {
        for (const d of state.catalog) if (d.block === blk) { defForBlk = d; break; }
      }
      if (!defForBlk) continue;
      if (defForBlk.hidden) continue;
      const ch = canInsert(parentNode, blk, index);
      if (ch.ok) return true;
    }
    return false;
  }
  return false;
}

export function legalBoundariesFor(parentNode) {
  const siblings = parentNode ? parentNode.children : state.document.nodes;
  const len = siblings ? siblings.length : 0;
  const out = [];
  for (let i = 0; i <= len; i++) {
    const legal = hasLegalInsertion(parentNode, i);
    out.push({ parentId: parentNode ? parentNode.id : null, parentNode, index: i, legal });
  }
  return out;
}

export function getDefinitionForBlock(block) {
  return definitionForBlock(block);
}

// Resolve insertionTarget object {parentId,index} to {parentNode,siblings,index}
export function resolveInsertionTarget(target) {
  if (!target) return null;
  if (target.parentId == null) {
    return { parentNode: null, siblings: state.document.nodes, index: Math.min(target.index, state.document.nodes.length) };
  }
  const parentFound = findNode(target.parentId);
  if (!parentFound) return null;
  const parentNode = parentFound.node;
  const siblings = parentNode.children;
  return { parentNode, siblings, index: Math.min(target.index, siblings.length) };
}

export function insertionPointForBlock(block) {
  // Prefer explicit insertionTarget if valid — strict, no fallback if target is set
  if (state.insertionTarget) {
    const resolved = resolveInsertionTarget(state.insertionTarget);
    if (!resolved) return null;
    const can = canInsert(resolved.parentNode, block, resolved.index);
    if (can.ok) return resolved;
    return null;
  }
  // Otherwise try contextual: selected container, then selected sibling
  const selected = state.selectedNodeId && findNode(state.selectedNodeId);
  if (selected) {
    const canChild = canInsert(selected.node, block, selected.node.children.length);
    if (canChild.ok) return { parentNode: selected.node, siblings: selected.node.children, index: selected.node.children.length };
    if (selected.parent) {
      const canSibling = canInsert(selected.parent, block, selected.index+1);
      if (canSibling.ok) return { parentNode: selected.parent, siblings: selected.siblings, index: selected.index+1 };
    } else {
      // root sibling
      const canRootSibling = canInsert(null, block, selected.index+1);
      if (canRootSibling.ok) return { parentNode: null, siblings: state.document.nodes, index: selected.index+1 };
    }
  }
  // Last try root append only for empty document / no selection (explicit UX: not surprising teleport)
  if (state.document.nodes.length === 0) {
    const canRoot = canInsert(null, block, 0);
    if (canRoot.ok) return { parentNode: null, siblings: state.document.nodes, index: 0 };
  }
  return null;
}
