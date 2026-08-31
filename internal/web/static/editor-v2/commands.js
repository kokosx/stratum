// commands.js — SDT mutations + node factory (single command for M4)
import { state, setDocument, findDocumentNode, definitionForBlock } from "./state.js";
import { canInsert } from "./insertion.js";

function randomID() {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return "blk_" + Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

function isSafeKey(key) {
  return key !== "__proto__" && key !== "prototype" && key !== "constructor";
}

function sanitizeObject(value) {
  if (value === null || typeof value !== "object") return value;
  if (Array.isArray(value)) return value.map(sanitizeObject);
  const out = Object.create(null);
  for (const [k, v] of Object.entries(value)) {
    if (!isSafeKey(k)) continue;
    out[k] = sanitizeObject(v);
  }
  return Object.assign({}, out);
}

function defaultValue(schema) {
  if (!schema) return "";
  if (schema.default !== null && schema.default !== undefined) {
    try {
      const cloned = JSON.parse(JSON.stringify(schema.default));
      return sanitizeObject(cloned);
    } catch (_) { return sanitizeObject(schema.default); }
  }
  if (schema.type === "object") {
    const result = Object.create(null);
    const props = schema.properties || {};
    for (const [name, child] of Object.entries(props)) {
      if (!isSafeKey(name)) continue;
      result[name] = defaultValue(child);
    }
    // return plain object but without polluted prototype
    return Object.assign({}, result);
  }
  if (schema.type === "array") return [];
  if (schema.type === "boolean") return false;
  if (schema.type === "integer" || schema.type === "number") return schema.minimum ?? 0;
  return schema.enum?.[0] ?? "";
}

function createNode(definition, depth = 0) {
  if (!definition || !definition.block) return null;
  if (depth > 6) {
    // eslint-disable-next-line no-console
    console.warn(`createNode: max depth reached for ${definition.block}`);
    return null;
  }
  const node = {
    id: randomID(),
    block: definition.block,
    version: definition.version,
    props: defaultValue(definition.schema?.props),
    settings: defaultValue(definition.schema?.settings),
    children: [],
  };
  // Respect starterChildren recursively (generic, no hardcode)
  const starters = definition.schema?.editor?.starterChildren;
  if (Array.isArray(starters) && starters.length) {
    let created = 0;
    for (const sc of starters) {
      const childDef = definitionForBlock(sc.block, sc.version) || definitionForBlock(sc.block);
      if (!childDef) {
        // eslint-disable-next-line no-console
        console.warn(`createNode: missing definition for starter ${sc.block}@${sc.version}`);
        continue;
      }
      const child = createNode(childDef, depth + 1);
      if (!child) continue;
      // only override version if that version actually exists
      if (sc.version && child.version !== sc.version) {
        const versionExists = definitionForBlock(sc.block, sc.version);
        if (versionExists) child.version = sc.version;
      }
      node.children.push(child);
      created++;
    }
    if (created > 0) return node;
    // if none created, fall through to implicit min handling
  }
  // Implicit: single allowed block + min>0 → create min children
  const rule = definition.schema?.children;
  if (rule && rule.min && rule.min > 0 && rule.mode === "allowed" && Array.isArray(rule.blocks) && rule.blocks.length === 1) {
    const childBlock = rule.blocks[0];
    const childDef = definitionForBlock(childBlock);
    if (childDef) {
      for (let i = 0; i < rule.min; i++) {
        const child = createNode(childDef, depth + 1);
        if (child) node.children.push(child);
      }
    }
  }
  return node;
}

function cloneDocument(doc) {
  try {
    if (typeof structuredClone === "function") return structuredClone(doc);
  } catch (_) {}
  return JSON.parse(JSON.stringify(doc));
}

function findCloneParent(cloneDoc, parentId) {
  if (parentId == null) return null;
  const walk = (nodes) => {
    for (const n of nodes || []) {
      if (n && n.id === parentId) return n;
      const found = walk(n.children);
      if (found) return found;
    }
    return null;
  };
  return walk(cloneDoc.nodes);
}

export function insertBlock({ definition, parentId, index }) {
  if (!definition || !definition.block) {
    return { ok: false, reason: "Unknown block." };
  }
  const targetParentId = parentId ?? null;
  const targetIndex = Number(index) || 0;

  // validate using live state (generic schema legality)
  const parentNode = targetParentId == null ? null : findDocumentNode(targetParentId);
  if (targetParentId != null && !parentNode) {
    return { ok: false, reason: "Parent not found." };
  }
  const legal = canInsert(parentNode, definition, targetIndex);
  if (!legal.ok) {
    return { ok: false, reason: legal.reason };
  }

  const node = createNode(definition);
  if (!node) return { ok: false, reason: "Could not create block." };

  // immutable enough: clone then mutate single array
  const next = cloneDocument(state.document);
  next.nodes ||= [];
  let targetArr;
  if (targetParentId == null) {
    targetArr = next.nodes;
  } else {
    const cloneParent = findCloneParent(next, targetParentId);
    if (!cloneParent) return { ok: false, reason: "Parent not found in clone." };
    cloneParent.children ||= [];
    targetArr = cloneParent.children;
  }
  const clamped = Math.max(0, Math.min(targetIndex, targetArr.length));
  targetArr.splice(clamped, 0, node);

  setDocument(next);
  return { ok: true, node, parentId: targetParentId, index: clamped };
}

// Expose for tests/internal use
export { createNode, defaultValue, randomID };
