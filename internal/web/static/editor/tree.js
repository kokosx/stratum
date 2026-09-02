// tree.js — SDT walk, find, insert, move, validation
import { state, definitions, definitionFor, defaultValue, hydrateObject } from "./state.js";

export function findNode(id, nodes = state.document.nodes, parent = null) {
  for (let i = 0; i < nodes.length; i++) {
    if (nodes[i].id === id) return { node: nodes[i], siblings: nodes, index: i, parent };
    const nested = findNode(id, nodes[i].children || [], nodes[i]);
    if (nested) return nested;
  }
  return null;
}

export function isContainer(node) {
  const def = definitionFor(node);
  return def?.schema.children.mode !== "none";
}

export function subtreeHasChildren(node) {
  return (node.children || []).length > 0 || (node.children || []).some(subtreeHasChildren);
}

export function childrenAllow(definition, block, currentCount) {
  if (!definition) return false;
  const rule = definition.schema.children;
  if (rule.mode === "none") return false;
  if (rule.max !== undefined && rule.max !== null && currentCount >= rule.max) return false;
  return rule.mode === "any" || (rule.mode === "allowed" && rule.blocks.includes(block));
}

export function containerAccepts(containerNode, block, drag) {
  if (!containerNode) return true;
  const definition = definitionFor(containerNode);
  if (!definition) return false;
  const rule = definition.schema.children;
  if (rule.mode === "none") return false;
  if (rule.mode === "allowed" && !rule.blocks.includes(block)) return false;
  if (rule.max !== undefined && rule.max !== null) {
    const count = containerNode.children.length;
    const sameContainer = drag && drag.type === "node" && findNode(drag.nodeId)?.parent === containerNode;
    if (!sameContainer && count >= rule.max) return false;
  }
  return true;
}

export function dragBlock(drag) {
  if (drag.type === "library") return drag.definition.block;
  const found = findNode(drag.nodeId);
  if (!found) return "";
  const def = definitionFor(found.node);
  return def ? def.block : found.node.block;
}

export function isWithin(ancestorId, node) {
  if (node.id === ancestorId) return true;
  return (node.children || []).some((child) => isWithin(ancestorId, child));
}

export function insertionPoint(block) {
  // Deprecated: use mutations.insertionPointForBlock. Returns null when no valid contextual location.
  const selected = state.selectedNodeId && findNode(state.selectedNodeId);
  if (!selected) {
    return { siblings: state.document.nodes, index: state.document.nodes.length };
  }
  const def = definitionFor(selected.node);
  const selectedChildren = selected.node.children || [];
  if (childrenAllow(def, block, selectedChildren.length)) {
    return { siblings: selectedChildren, index: selectedChildren.length };
  }
  if (!selected.parent) return { siblings: selected.siblings, index: selected.index + 1 };
  const parentDef = definitionFor(selected.parent);
  if (childrenAllow(parentDef, block, selected.siblings.length)) {
    return { siblings: selected.siblings, index: selected.index + 1 };
  }
  // No valid contextual location — return null to enforce explicit placement (caller must handle)
  return null;
}

export function randomID() {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return "blk_" + Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

export function createNode(definition) {
  const node = {
    id: randomID(),
    block: definition.block,
    version: definition.version,
    props: defaultValue(definition.schema.props),
    settings: defaultValue(definition.schema.settings),
    children: [],
  };
  if (definition.block === "core/form") {
    const forms = (window.__STRATUM_BOOTSTRAP_FORMS || (state.bootstrapForms || []));
    // fallback to bootstrap.forms
    const bootstrapForms = (document.getElementById("editor-bootstrap") ? JSON.parse(document.getElementById("editor-bootstrap").textContent).forms : []) || [];
    const formsList = bootstrapForms || [];
    const active = formsList.filter((o) => !String(o.label).endsWith(" (Disabled)"));
    const current = node.settings && node.settings.formId;
    if ((!current || current === "") && active.length === 1) {
      node.settings.formId = active[0].value;
    }
  }
  // use bootstrap from state module
  const b = (typeof bootstrap !== 'undefined' ? bootstrap : null) || JSON.parse(document.getElementById("editor-bootstrap").textContent);
  if (definition.block === "core/form") {
    const forms = b.forms || [];
    const active = forms.filter((o) => !String(o.label).endsWith(" (Disabled)"));
    const current = node.settings && node.settings.formId;
    if ((!current || current === "") && active.length === 1) {
      node.settings.formId = active[0].value;
    }
  }
  return node;
}

export function duplicateSubtree(node) {
  const clone = JSON.parse(JSON.stringify(node));
  assignNewIDs(clone);
  return clone;
}

export function assignNewIDs(node) {
  node.id = randomID();
  (node.children || []).forEach(assignNewIDs);
}

export function clonePatternNodes(pattern) {
  const nodes = JSON.parse(JSON.stringify(pattern.document.nodes || []));
  nodes.forEach((n) => assignNewIDs(n));
  nodes.forEach(function hydrateCloned(n) {
    const def = definitions.get(`${n.block}@${n.version}`);
    if (def) {
      n.props ||= {};
      n.settings ||= {};
      n.children ||= [];
      hydrateObject(n.props, def.schema.props);
      hydrateObject(n.settings, def.schema.settings);
    }
    (n.children || []).forEach(hydrateCloned);
  });
  return nodes;
}

function definitionForBlockLocal(block) {
  for (const def of definitions.values()) {
    if (def.block === block) return def;
  }
  for (const d of state.catalog) {
    if (d.block === block) return d;
  }
  return null;
}
function canChildAcceptParentLocal(block, parentNode) {
  const childDef = definitionForBlockLocal(block);
  if (!childDef) return true;
  const parents = childDef.schema?.placement?.parents;
  if (!parents || parents.length === 0) return true;
  if (!parentNode) return false;
  return parents.includes(parentNode.block);
}

export function canInsertRoots(containerNode, roots) {
  for (const r of roots) {
    if (!canChildAcceptParentLocal(r.block, containerNode)) return false;
  }
  if (!containerNode) return true;
  const def = definitionFor(containerNode);
  if (!def) return false;
  const rule = def.schema.children;
  if (rule.mode === "none") return false;
  if (rule.max !== undefined && rule.max !== null) {
    if (containerNode.children.length + roots.length > rule.max) return false;
  }
  if (rule.mode === "allowed") {
    for (const r of roots) {
      if (!rule.blocks.includes(r.block)) return false;
    }
  }
  return true;
}

export function insertionPointForPattern(rootBlocks) {
  const selected = state.selectedNodeId && findNode(state.selectedNodeId);
  if (!selected) {
    if (canInsertRoots(null, rootBlocks)) return { siblings: state.document.nodes, index: state.document.nodes.length };
    return null;
  }
  if (canInsertRoots(selected.node, rootBlocks)) {
    return { siblings: selected.node.children, index: selected.node.children.length };
  }
  if (selected.parent && canInsertRoots(selected.parent, rootBlocks)) {
    return { siblings: selected.siblings, index: selected.index + 1 };
  }
  if (canInsertRoots(null, rootBlocks)) {
    return { siblings: state.document.nodes, index: state.document.nodes.length };
  }
  return null;
}

export function collectNodeIds(nodes, set) {
  for (const n of nodes) {
    set.add(n.id);
    if (n.children) collectNodeIds(n.children, set);
  }
}
