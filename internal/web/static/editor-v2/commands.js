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

function findCloneNode(cloneDoc, nodeId) {
  const walk = (nodes) => {
    for (const n of nodes || []) {
      if (n && n.id === nodeId) return n;
      const found = walk(n.children);
      if (found) return found;
    }
    return null;
  };
  return walk(cloneDoc.nodes);
}

function getRawValue(node, path) {
  const dot = path.indexOf(".");
  if (dot === -1) return undefined;
  const scope = path.slice(0, dot);
  const rest = path.slice(dot + 1);
  const parts = rest.split(".");
  let obj = scope === "props" ? node.props : node.settings;
  for (let i = 0; i < parts.length; i++) {
    if (obj == null || typeof obj !== "object") return undefined;
    obj = obj[parts[i]];
  }
  return obj;
}

function setValueAtPath(node, path, value) {
  const dot = path.indexOf(".");
  const scope = path.slice(0, dot);
  const rest = path.slice(dot + 1);
  const parts = rest.split(".");
  let target = scope === "props" ? (node.props ||= {}) : (node.settings ||= {});
  for (let i = 0; i < parts.length - 1; i++) {
    const key = parts[i];
    if (!isSafeKey(key)) continue;
    if (target[key] == null || typeof target[key] !== "object") target[key] = {};
    target = target[key];
  }
  const last = parts[parts.length - 1];
  if (!isSafeKey(last)) return;
  target[last] = value;
}

function validateAgainstSchema(value, schema, path) {
  if (!schema) return null;
  // Handle enum
  if (schema.enum && schema.enum.length) {
    const want = JSON.stringify(value);
    let found = false;
    for (const opt of schema.enum) {
      if (JSON.stringify(opt) === want) { found = true; break; }
    }
    if (!found) return `Invalid value for ${path}.`;
  }
  switch (schema.type) {
    case "string": {
      if (typeof value !== "string") return `${path}: expected string`;
      const len = [...value].length;
      if (schema.minLength != null && len < schema.minLength) return `${path}: too short`;
      if (schema.maxLength != null && len > schema.maxLength) return `${path}: too long`;
      // Generic hard cap to prevent DoS (also for button label without maxLength)
      if (len > 10000) return `${path}: too long`;
      if (schema.pattern) {
        try {
          const re = new RegExp(schema.pattern);
          if (!re.test(value)) return `${path}: does not match pattern`;
        } catch (_) {}
      }
      break;
    }
    case "integer":
    case "number": {
      const num = typeof value === "number" ? value : Number(value);
      if (Number.isNaN(num)) return `${path}: expected number`;
      if (schema.type === "integer" && !Number.isInteger(num)) return `${path}: expected integer`;
      if (schema.minimum != null && num < schema.minimum) return `${path}: too small`;
      if (schema.maximum != null && num > schema.maximum) return `${path}: too large`;
      break;
    }
    case "boolean": {
      if (typeof value !== "boolean") return `${path}: expected boolean`;
      break;
    }
    case "object": {
      if (value == null || typeof value !== "object" || Array.isArray(value)) return `${path}: expected object`;
      if (schema.properties) {
        for (const [k, propSchema] of Object.entries(schema.properties)) {
          if (schema.required && schema.required.includes(k) && !(k in value)) {
            return `${path}.${k}: required`;
          }
          if (k in value) {
            const err = validateAgainstSchema(value[k], propSchema, `${path}.${k}`);
            if (err) return err;
          }
        }
        // Strict: unknown fields not allowed
        for (const k of Object.keys(value)) {
          if (!isSafeKey(k)) return `Invalid field ${k}`;
          if (!(k in schema.properties)) {
            return `${path}.${k}: unknown field`;
          }
        }
      }
      break;
    }
    case "array": {
      if (!Array.isArray(value)) return `${path}: expected array`;
      if (schema.items) {
        for (let i = 0; i < value.length; i++) {
          const err = validateAgainstSchema(value[i], schema.items, `${path}[${i}]`);
          if (err) return err;
        }
      }
      break;
    }
    default:
      break;
  }
  return null;
}

function isRichTextSchema(schema) {
  // Heuristic: object with version/content properties and required version
  return schema && schema.type === "object" && schema.properties && "version" in schema.properties && "content" in schema.properties;
}

export function updateNodeField({ nodeId, path, value }) {
  if (!nodeId || typeof nodeId !== "string") return { ok: false, reason: "Unknown block." };
  if (!path || typeof path !== "string") return { ok: false, reason: "Unknown field." };
  const parts = path.split(".");
  for (const p of parts) {
    if (!isSafeKey(p)) return { ok: false, reason: "Invalid field." };
  }
  if (parts.length < 2 || (parts[0] !== "props" && parts[0] !== "settings")) {
    return { ok: false, reason: "Unknown field." };
  }
  const node = findDocumentNode(nodeId);
  if (!node) return { ok: false, reason: "Block not found." };
  const def = definitionForBlock(node.block, node.version);
  if (!def || !def.schema) return { ok: false, reason: "Unknown block definition." };
  // Resolve schema for path
  let schema = parts[0] === "props" ? def.schema.props : def.schema.settings;
  for (let i = 1; i < parts.length; i++) {
    const key = parts[i];
    if (!schema || !schema.properties || !(key in schema.properties)) {
      return { ok: false, reason: "Unknown field." };
    }
    schema = schema.properties[key];
  }
  // Check field belongs to actual schema (already) and is inline? But updateNodeField is generic; spec says allowed field must belong to block's actual schema (we do).
  // However we also enforce inline? The command should allow any props/settings field that exists, not only inline. M5A only uses inline but keep generic.
  const fieldMeta = def.schema.editor?.fields?.[path];
  // If field has inline restriction? No, allow any.

  // Coerce and validate value
  let coerced;
  const control = fieldMeta?.control;
  const isRichText = control === "richtext" || isRichTextSchema(schema);
  if (isRichText) {
    if (typeof value !== "string") return { ok: false, reason: "Expected text." };
    let plain = String(value);
    plain = plain.replace(/\r\n/g, "\n").replace(/\r/g, "\n").replace(/\n/g, " ");
    if (plain.trim() === "") plain = "";
    if (plain.length > 10000) plain = plain.slice(0, 10000);
    if (plain === "") {
      coerced = { version: 1, content: [] };
    } else {
      coerced = { version: 1, content: [{ text: plain }] };
    }
    const err = validateAgainstSchema(coerced, schema, path);
    if (err) return { ok: false, reason: err };
  } else {
    // Non-richtext: expect string/number/boolean per schema type
    if (schema.type === "string") {
      if (typeof value !== "string") return { ok: false, reason: "Expected text." };
      let plain = String(value).replace(/\r\n/g, "\n").replace(/\r/g, "\n").replace(/\n/g, " ");
      if (plain.trim() === "") plain = "";
      if (plain.length > 10000) plain = plain.slice(0, 10000);
      coerced = plain;
      const err = validateAgainstSchema(coerced, schema, path);
      if (err) return { ok: false, reason: err };
    } else if (schema.type === "integer" || schema.type === "number") {
      const num = typeof value === "number" ? value : Number(value);
      if (Number.isNaN(num)) return { ok: false, reason: "Expected number." };
      coerced = num;
      const err = validateAgainstSchema(coerced, schema, path);
      if (err) return { ok: false, reason: err };
    } else if (schema.type === "boolean") {
      if (typeof value !== "boolean") return { ok: false, reason: "Expected boolean." };
      coerced = value;
    } else {
      // Generic fallback: sanitize object
      coerced = sanitizeObject(value);
      const err = validateAgainstSchema(coerced, schema, path);
      if (err) return { ok: false, reason: err };
    }
  }

  const previousValue = getRawValue(node, path);
  // Compare for unchanged (deep)
  try {
    if (JSON.stringify(previousValue) === JSON.stringify(coerced)) {
      return { ok: true, node, previousValue, value: coerced, unchanged: true };
    }
  } catch (_) {}

  const next = cloneDocument(state.document);
  const cloneNode = findCloneNode(next, nodeId);
  if (!cloneNode) return { ok: false, reason: "Block not found." };
  const sanitized = sanitizeObject(coerced);
  // For string/richtext, sanitizeObject will handle appropriately (string returns string, object sanitized)
  // sanitizeObject for string returns string (since not object), so use coerced directly if string
  const toSet = typeof coerced === "string" ? coerced : sanitized;
  setValueAtPath(cloneNode, path, toSet);
  // preserve id/block/version already
  cloneNode.id = node.id;
  cloneNode.block = node.block;
  cloneNode.version = node.version;

  setDocument(next);
  const updated = findDocumentNode(nodeId);
  return { ok: true, node: updated, previousValue, value: toSet };
}

// Expose for tests/internal use
export { createNode, defaultValue, randomID };
