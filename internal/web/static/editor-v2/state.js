// state.js — V2 minimal state, no global spaghetti
const bootstrapEl = document.getElementById("editor-v2-bootstrap");
let raw = {};
try {
  raw = bootstrapEl ? JSON.parse(bootstrapEl.textContent || "{}") : {};
} catch (_) {
  raw = {};
}

export const bootstrap = raw;

function derivePublicInfo() {
  const candidates = [
    raw.actions?.publicPreviewUrl,
    raw.actions?.publicPreviewURL,
    raw.actions?.PublicPreviewURL,
    raw.publicUrl,
    raw.PublicURL,
    bootstrap.publicUrl,
    raw.publicPreviewUrl,
  ];
  let rawUrl = "";
  for (const c of candidates) {
    if (typeof c === "string" && c.trim() !== "") {
      rawUrl = c.trim();
      break;
    }
  }
  if (!rawUrl && raw.resource?.location) rawUrl = String(raw.resource.location);
  if (!rawUrl && bootstrap.resource?.location) rawUrl = String(bootstrap.resource.location);

  let publicUrl = rawUrl;
  try {
    if (rawUrl) {
      const u = new URL(rawUrl, window.location.origin);
      publicUrl = u.origin + u.pathname + u.search;
    } else {
      const slug = raw.slug || bootstrap.slug || raw.resource?.slug || "";
      if (slug) {
        let p = String(slug).trim();
        if (!p.startsWith("/")) p = "/" + p;
        p = p.replace(/\/+$/, "") || "/";
        publicUrl = window.location.origin + p;
      } else {
        publicUrl = window.location.origin + "/";
      }
    }
  } catch (_) {
    publicUrl = rawUrl || window.location.origin + "/";
  }
  return { publicUrl };
}

const derived = derivePublicInfo();

function authoritativeResourceURL() {
  const candidates = [
    bootstrap.resource?.location,
    bootstrap.actions?.publicPreviewUrl,
    bootstrap.actions?.publicPreviewURL,
  ];
  for (const candidate of candidates) {
    if (typeof candidate === "string" && candidate.trim()) return candidate.trim();
  }
  return "";
}

// selection is pure runtime, does not mutate SDT here
// { nodeId, instanceKey, editable, ownerType, ownerId, ownerLabel } or null
function createSelectionState() {
  return { current: null, hoveredKey: null };
}

const sel = createSelectionState();
const selectionListeners = new Set();

export const panelState = {
  left: null,
  right: null,
  recent: [],
};

const panelListeners = new Set();

function notifySelection(next, previous) {
  for (const listener of selectionListeners) {
    try { listener(next, previous); } catch (_) {}
  }
}

function notifyPanels() {
  const snapshot = { left: panelState.left, right: panelState.right, recent: [...panelState.recent] };
  for (const listener of panelListeners) {
    try { listener(snapshot); } catch (_) {}
  }
}

const documentListeners = new Set();
let persistedDocumentJSON = "";
try { persistedDocumentJSON = JSON.stringify(bootstrap.document || { version: 1, nodes: [] }); } catch (_) { persistedDocumentJSON = ""; }
function notifyDocument(next, meta) {
  for (const listener of documentListeners) {
    try { listener(next, meta); } catch (_) {}
  }
}

const editingStore = { current: null };
const editingListeners = new Set();
function notifyEditing(next, previous) {
  for (const listener of editingListeners) {
    try { listener(next, previous); } catch (_) {}
  }
}

export const state = {
  // generic resource descriptor, M1 focuses on entry/page but keeps shape generic
  resource: bootstrap.resource || {},
  // viewport: desktop | tablet | mobile
  viewport: "desktop",
  // preview endpoint
  previewUrl: bootstrap.previewUrl || bootstrap.actions?.previewUrl || "/admin/editor/preview",
  // document tree (SDT) from bootstrap
  document: bootstrap.document || { version: 1, nodes: [] },
  // metadata fallback from bootstrap top-level
  title: bootstrap.title || bootstrap.resource?.label || "",
  slug: bootstrap.slug || "",
  excerpt: bootstrap.excerpt || "",
  contentTypeId: bootstrap.contentTypeId || bootstrap.resource?.contentTypeId || "",
  entryId: bootstrap.entryId || bootstrap.resource?.id || "",
  status: bootstrap.status || "",
  templateName: bootstrap.templateName || "",
  csrfToken: bootstrap.csrfToken || "",
  // public URL for View Live / bootstrap
  publicUrl: derived.publicUrl,
  // Server-authoritative URL for document metadata. Unlike publicUrl this does
  // not synthesize a route for drafts that are not publicly available.
  resourceUrl: authoritativeResourceURL(),
  dirty: false,
  // M2 selection
  get selection() {
    return sel.current;
  },
  set selection(v) {
    const previous = sel.current;
    sel.current = v;
    notifySelection(v, previous);
  },
  get hoveredKey() {
    return sel.hoveredKey;
  },
  set hoveredKey(v) {
    sel.hoveredKey = v;
  },
  get editing() {
    return editingStore.current;
  },
  set editing(v) {
    const previous = editingStore.current;
    editingStore.current = v;
    notifyEditing(v, previous);
  },
};

export function subscribeSelection(listener) {
  if (typeof listener !== "function") return () => {};
  selectionListeners.add(listener);
  return () => selectionListeners.delete(listener);
}

export function subscribePanels(listener) {
  if (typeof listener !== "function") return () => {};
  panelListeners.add(listener);
  return () => panelListeners.delete(listener);
}

export function subscribeDocument(listener) {
  if (typeof listener !== "function") return () => {};
  documentListeners.add(listener);
  return () => documentListeners.delete(listener);
}

export function subscribeEditing(listener) {
  if (typeof listener !== "function") return () => {};
  editingListeners.add(listener);
  return () => editingListeners.delete(listener);
}

export function setEditing(next) {
  state.editing = next;
}
export function clearEditing() {
  state.editing = null;
}
export function getEditing() {
  return state.editing;
}
export function isEditing() {
  return !!state.editing;
}

export function setDocument(nextDocument, options = {}) {
  if (!nextDocument || typeof nextDocument !== "object") return;
  let previousDocumentJSON = "";
  try { previousDocumentJSON = JSON.stringify(state.document); } catch (_) {}
  // immutable enough: clone to avoid external mutation aliasing
  let cloned;
  try {
    cloned = typeof structuredClone === "function" ? structuredClone(nextDocument) : JSON.parse(JSON.stringify(nextDocument));
  } catch (_) {
    cloned = JSON.parse(JSON.stringify(nextDocument));
  }
  cloned.nodes ||= [];
  state.document = cloned;
  // internal dirty flag — prepares future Save/Undo, no elaborate history yet (§79)
  try {
    const now = JSON.stringify(state.document);
    state.dirty = now !== persistedDocumentJSON;
  } catch (_) { state.dirty = true; }
  const hint = options && options.renderHint ? options.renderHint : "refresh";
  const history = options && options.history === "skip" ? "skip" : "record";
  notifyDocument(state.document, { renderHint: hint, history, previousDocumentJSON });
}

export function syncDirtyBaseline() {
  try { persistedDocumentJSON = JSON.stringify(state.document); } catch (_) {}
  state.dirty = false;
}

export function setPanel(slot, panel) {
  const allowed = slot === "left"
    ? new Set([null, "blocks", "navigator"])
    : slot === "right"
      ? new Set([null, "inspector", "document"])
      : null;
  if (!allowed || !allowed.has(panel)) return false;
  if (panelState[slot] === panel) return false;
  panelState[slot] = panel;
  panelState.recent = panelState.recent.filter((item) => item !== slot);
  if (panel) panelState.recent.push(slot);
  notifyPanels();
  return true;
}

export function togglePanel(slot, panel) {
  return setPanel(slot, panelState[slot] === panel ? null : panel);
}

export function activatePanel(slot, panel) {
  if (panelState[slot] !== panel) return setPanel(slot, panel);
  if (mostRecentPanelSlot() === slot) return false;
  panelState.recent = panelState.recent.filter((item) => item !== slot);
  panelState.recent.push(slot);
  notifyPanels();
  return true;
}

export function mostRecentPanelSlot() {
  for (let i = panelState.recent.length - 1; i >= 0; i--) {
    const slot = panelState.recent[i];
    if (panelState[slot]) return slot;
  }
  return null;
}

// Helpers for displayName lookup (block id -> displayName)
// Uses bootstrap catalog + definitions if present, falls back to minimal map.
// NEVER returns technical IDs — always friendly.
const displayNameCache = new Map();

function humanizeBlockName(block) {
  if (!block || typeof block !== "string") return "";
  let name = block.includes("/") ? block.split("/").pop() : block;
  // handle kebab/sneak
  name = name.replace(/[_-]+/g, " ");
  // insert space before capitals for camelCase (rare)
  name = name.replace(/([a-z])([A-Z])/g, "$1 $2");
  return name
    .split(" ")
    .filter(Boolean)
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase())
    .join(" ");
}

function buildDisplayNameMap() {
  if (displayNameCache.size) return displayNameCache;
  const candidates = [];
  if (Array.isArray(bootstrap.catalog)) candidates.push(...bootstrap.catalog);
  if (Array.isArray(bootstrap.definitions)) candidates.push(...bootstrap.definitions);
  // catalog entries are {block, version, displayName}
  for (const item of candidates) {
    if (!item || !item.block) continue;
    // keep latest by block name (if multiple versions, larger version wins)
    const existing = displayNameCache.get(item.block);
    const disp = item.displayName && String(item.displayName).trim() ? String(item.displayName).trim() : "";
    const friendly = disp || humanizeBlockName(item.block);
    if (!existing || (item.version && existing.version < item.version)) {
      displayNameCache.set(item.block, { displayName: friendly, version: item.version });
    }
  }
  // Semantic fallback for known core blocks if no catalog provided or missing displayName
  const fallback = {
    "core/section": "Section",
    "core/stack": "Stack",
    "core/grid": "Grid",
    "core/heading": "Heading",
    "core/text": "Text",
    "core/button": "Button",
    "core/image": "Image",
    "core/collection": "Collection",
    "core/accordion": "Accordion",
    "core/navigation": "Navigation",
    "core/site-part": "Site Part",
    "core/entry-field": "Entry Field",
    "core/entry-title": "Entry Title",
    "core/entry-excerpt": "Entry Excerpt",
    "core/entry-content": "Entry Content",
    "core/entry-media": "Entry Media",
    "core/featured-image": "Featured Image",
    "core/content-slot": "Page Content",
    "core/archive-title": "Archive Title",
    "core/archive-description": "Archive Description",
    "core/site-name": "Site Name",
    "core/site-logo": "Site Logo",
    "core/form": "Form",
    "core/card": "Card",
  };
  for (const [k, v] of Object.entries(fallback)) {
    if (!displayNameCache.has(k)) displayNameCache.set(k, { displayName: v, version: 0 });
  }
  return displayNameCache;
}

export function displayNameForBlock(blockName) {
  if (!blockName || typeof blockName !== "string") return "Block";
  // Never treat technical IDs (blk_*, uuid, AaGwQ...) as block names — they lack "/"
  if (!blockName.includes("/")) {
    // Might be a technical nodeId passed by mistake — never show it
    return "Block";
  }
  const map = buildDisplayNameMap();
  const entry = map.get(blockName);
  if (entry && entry.displayName) return entry.displayName;
  // humanized fallback
  const hum = humanizeBlockName(blockName);
  return hum || "Block";
}

export function blockCatalog() {
  return Array.isArray(bootstrap.catalog) ? bootstrap.catalog : [];
}

export function definitionForBlock(block, version) {
  const candidates = [];
  if (Array.isArray(bootstrap.definitions)) candidates.push(...bootstrap.definitions);
  if (Array.isArray(bootstrap.catalog)) candidates.push(...bootstrap.catalog);
  let latest = null;
  for (const item of candidates) {
    if (!item || item.block !== block) continue;
    if (version && Number(item.version) === Number(version)) return item;
    if (!latest || Number(item.version || 0) > Number(latest.version || 0)) latest = item;
  }
  return latest;
}

export function findDocumentNode(nodeId) {
  if (!nodeId) return null;
  const walk = (nodes) => {
    for (const node of nodes || []) {
      if (node && node.id === nodeId) return node;
      const found = walk(node && node.children);
      if (found) return found;
    }
    return null;
  };
  return walk(state.document && state.document.nodes);
}

export function findDocumentParent(nodeId) {
  if (!nodeId) return null;
  const walk = (nodes, parent) => {
    for (let i = 0; i < (nodes || []).length; i++) {
      const node = nodes[i];
      if (node && node.id === nodeId) return { parent, index: i, siblings: nodes, node };
      const nested = walk(node && node.children, node);
      if (nested) return nested;
    }
    return null;
  };
  return walk(state.document && state.document.nodes, null);
}

export function isContainerNode(node) {
  if (!node) return false;
  const def = definitionForBlock(node.block, node.version);
  if (!def || !def.schema || !def.schema.children) return false;
  return def.schema.children.mode !== "none";
}

function isTechnicalValue(value) {
  if (!value || typeof value !== "string") return false;
  const text = value.trim();
  return /^(blk|entry|site|page)[-_]/i.test(text)
    || /^[0-9a-f]{8}-[0-9a-f]{4}-/i.test(text)
    || (/^[A-Za-z0-9_-]{16,}$/.test(text) && /[0-9_-]/.test(text));
}

function extractPlainText(value) {
  if (typeof value === "string") return value.trim();
  if (value && typeof value === "object" && value.version === 1 && Array.isArray(value.content)) {
    return value.content.map((run) => typeof run?.text === "string" ? run.text : "").join("").trim();
  }
  return "";
}

function getFieldValue(node, path) {
  if (!path || typeof path !== "string") return undefined;
  const parts = path.split(".");
  if (parts.length < 2) return undefined;
  const scope = parts[0];
  let obj = null;
  if (scope === "props") obj = node.props;
  else if (scope === "settings") obj = node.settings;
  else return undefined;
  for (let i = 1; i < parts.length; i++) {
    if (obj == null || typeof obj !== "object") return undefined;
    obj = obj[parts[i]];
  }
  return obj;
}

function humanizeSegment(segment) {
  let name = segment.replace(/([a-z])([A-Z])/g, "$1 $2").replace(/[_-]+/g, " ").trim();
  if (!name) return "Value";
  if (name.toLowerCase() === "url") return "Link";
  return name.split(/\s+/).filter(Boolean).map((w) => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase()).join(" ");
}

export function friendlyLabelForPath(path, definition) {
  const field = definition?.schema?.editor?.fields?.[path];
  if (field?.label) {
    if (path === "props.text" && field.control === "richtext" && field.label === "Text") return "Content";
    if (path === "props.url" && field.label === "URL") return "Link";
    return field.label;
  }
  const last = path.split(".").pop() || "";
  if (last.toLowerCase() === "url") return "Link";
  return humanizeSegment(last);
}

function isTechnicalField(path, raw) {
  const last = path.split(".").pop() || "";
  if (/Id$/i.test(last)) return true;
  if (typeof raw === "string" && isTechnicalValue(raw)) return true;
  return false;
}

function formatInspectorValue(raw, path) {
  if (raw == null) return "";
  if (typeof raw === "string") {
    const t = raw.trim();
    if (!t) return "";
    return t.length > 140 ? t.slice(0, 137) + "\u2026" : t;
  }
  if (typeof raw === "number") {
    if (path.endsWith("level")) return `H${raw}`;
    return String(raw);
  }
  if (typeof raw === "boolean") {
    return raw ? "Yes" : "";
  }
  if (raw && typeof raw === "object" && raw.version === 1 && Array.isArray(raw.content)) {
    const txt = raw.content.map((run) => typeof run?.text === "string" ? run.text : "").join("").trim();
    if (!txt) return "";
    return txt.length > 140 ? txt.slice(0, 137) + "\u2026" : txt;
  }
  if (Array.isArray(raw)) {
    const joined = raw.filter((v) => typeof v === "string" && v.trim()).join(", ").trim();
    if (!joined) return "";
    return joined.length > 140 ? joined.slice(0, 137) + "\u2026" : joined;
  }
  return "";
}

export function nodeSummaryFor(node) {
  if (!node) return "";
  const def = definitionForBlock(node.block, node.version);
  const fields = def?.schema?.editor?.summaryFields || def?.SummaryFields || [];
  for (const path of fields) {
    const raw = getFieldValue(node, path);
    const txt = extractPlainText(raw);
    if (txt) return txt.slice(0, 70);
    if (typeof raw === "number" && String(raw).trim()) return String(raw).slice(0, 70);
  }
  const fieldOrder = Object.keys(def?.schema?.editor?.fields || {});
  for (const path of fieldOrder) {
    if (!path.startsWith("props.")) continue;
    const last = path.split(".").pop() || "";
    if (/Id$/i.test(last)) continue;
    const raw = getFieldValue(node, path);
    const txt = extractPlainText(raw);
    if (txt) return txt.slice(0, 70);
    if (typeof raw === "number" && String(raw).trim()) return String(raw).slice(0, 70);
  }
  const props = node.props || {};
  for (const [k, v] of Object.entries(props)) {
    if (/Id$/i.test(k)) continue;
    const txt = extractPlainText(v);
    if (txt) return txt.slice(0, 70);
    if (typeof v === "string" && v.trim()) return v.trim().slice(0, 70);
  }
  return "";
}

export function inspectorFactsFor(node) {
  if (!node) return [];
  const def = definitionForBlock(node.block, node.version);
  if (!def) return [];
  const facts = [];
  const seen = new Set();
  const summaryFields = def.schema?.editor?.summaryFields || def.SummaryFields || [];
  for (const path of summaryFields) {
    const raw = getFieldValue(node, path);
    const formatted = formatInspectorValue(raw, path);
    if (!formatted) continue;
    if (isTechnicalField(path, raw)) continue;
    const label = friendlyLabelForPath(path, def);
    facts.push([label, formatted]);
    seen.add(path);
  }
  const fieldPaths = Object.keys(def.schema?.editor?.fields || {});
  const propsPaths = fieldPaths.filter((p) => p.startsWith("props."));
  for (const path of propsPaths) {
    if (seen.has(path)) continue;
    const raw = getFieldValue(node, path);
    const formatted = formatInspectorValue(raw, path);
    if (!formatted) continue;
    if (isTechnicalField(path, raw)) continue;
    if (facts.length >= 4) break;
    const label = friendlyLabelForPath(path, def);
    facts.push([label, formatted]);
  }
  return facts;
}

export function friendlyLabelForUnknown() {
  return "Template element";
}

export function clearSelection() {
  state.selection = null;
}
export function setSelection(selObj) {
  state.selection = selObj;
}

// === Inline editing helpers (M5A) ===

export function isPlainRichTextValue(value) {
  if (!value || typeof value !== "object") return false;
  if (value.version !== 1 || !Array.isArray(value.content)) return false;
  if (value.content.length === 0) return true;
  // Plain means every run is plain text without marks (no bold/italic/link etc.)
  for (const run of value.content) {
    if (!run || typeof run.text !== "string") return false;
    if (run.marks && run.marks.length > 0) return false;
  }
  return true;
}

export function plainTextFromRichText(value) {
  if (!value || typeof value !== "object" || !Array.isArray(value.content)) return "";
  return value.content.map((r) => (typeof r.text === "string" ? r.text : "")).join("");
}

export function isRichTextValid(value) {
  if (!value || typeof value !== "object") return false;
  if (value.version !== 1 || !Array.isArray(value.content)) return false;
  for (const run of value.content) {
    if (!run || typeof run.text !== "string") return false;
    if (run.marks != null) {
      if (!Array.isArray(run.marks)) return false;
      for (const m of run.marks) {
        if (!m || typeof m.type !== "string") return false;
        if (!["bold","italic","strike","code","link"].includes(m.type)) return false;
        if (m.type === "link") {
          if (typeof m.href !== "string" || m.href.trim() === "") return false;
        }
      }
    }
  }
  return true;
}

export function isFieldPlainSafe(node, path) {
  const def = definitionForBlock(node.block, node.version);
  const field = def?.schema?.editor?.fields?.[path];
  if (!field || !field.inline) return false;
  const raw = getFieldValue(node, path);
  if (field.control === "richtext") {
    if (field.inlineMode === "rich") {
      if (raw == null) return true;
      return isRichTextValid(raw);
    }
    // plain mode: only plain runs
    if (raw == null) return true;
    return isPlainRichTextValue(raw);
  }
  if (field.control === "text" || field.control === "textarea") {
    if (raw == null) return true;
    return typeof raw === "string";
  }
  return false;
}

export function inlineFieldsForNode(node) {
  if (!node) return [];
  const def = definitionForBlock(node.block, node.version);
  if (!def || !def.schema || !def.schema.editor || !def.schema.editor.fields) return [];
  const result = [];
  for (const [path, field] of Object.entries(def.schema.editor.fields)) {
    if (!field.inline) continue;
    if (!isFieldPlainSafe(node, path)) continue;
    result.push(path);
  }
  return result;
}

export function primaryInlineFieldForNode(node) {
  const fields = inlineFieldsForNode(node);
  if (fields.length !== 1) return null;
  return fields[0];
}

export function isInlineEditableNode(node) {
  return inlineFieldsForNode(node).length > 0;
}

export function placeholderForInlineField(block, path) {
  try {
    const def = definitionForBlock(block);
    const field = def?.schema?.editor?.fields?.[path];
    if (field && typeof field.placeholder === "string" && field.placeholder.trim() !== "") {
      return field.placeholder;
    }
  } catch (_) {}
  return "";
}

export function isInlineEditableField(node, path) {
  const fields = inlineFieldsForNode(node);
  return fields.includes(path);
}
