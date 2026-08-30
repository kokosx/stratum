// state.js — document state, selection, dirty, history
export const bootstrap = JSON.parse(document.getElementById("editor-bootstrap").textContent || "{}");

export const state = {
  document: bootstrap.document || { version: 1, nodes: [] },
  catalog: bootstrap.catalog || [],
  patterns: bootstrap.patterns || [],
  selectedNodeId: null,
  selectedInstanceKey: null,
  dirty: false,
  collapsed: new Set(),
  mode: "edit",
  previewWidth: "100%",
  libraryTab: "blocks",
};

export const definitions = new Map(
  [...state.catalog, ...(bootstrap.definitions || [])].map((item) => [`${item.block}@${item.version}`, item])
);

let persistedJSON = JSON.stringify(state.document);
let persistedMeta = captureMetaSnapshot();

export function getPersistedJSON() { return persistedJSON; }
export function setPersistedJSON(v) { persistedJSON = v; }
export function getPersistedMeta() { return persistedMeta; }
export function setPersistedMeta(v) { persistedMeta = v; }

// History
const history = [];
let historyIndex = -1;
const MAX_HISTORY = 50;
let lastPushTime = 0;

export function pushHistory() {
  history.splice(historyIndex + 1);
  const snap = JSON.stringify(state.document);
  if (historyIndex >= 0 && history[historyIndex] === snap) return;
  history.push(snap);
  if (history.length > MAX_HISTORY) history.shift();
  historyIndex = history.length - 1;
}

export function maybePushHistory() {
  const now = Date.now();
  if (now - lastPushTime < 500) return;
  const snap = JSON.stringify(state.document);
  if (historyIndex >= 0 && history[historyIndex] === snap) return;
  pushHistory();
  lastPushTime = now;
}

export function undo() {
  if (historyIndex <= 0) return false;
  historyIndex--;
  restoreSnapshot();
  return true;
}

export function redo() {
  if (historyIndex >= history.length - 1) return false;
  historyIndex++;
  restoreSnapshot();
  return true;
}

function restoreSnapshot() {
  state.document = JSON.parse(history[historyIndex]);
  state.document.nodes ||= [];
  state.document.nodes.forEach(hydrateNode);
  state.selectedNodeId = null;
  state.selectedInstanceKey = null;
  // caller will render
}

export function canUndo() { return historyIndex > 0; }
export function canRedo() { return historyIndex < history.length - 1; }

export function captureMetaSnapshot() {
  const ids = ["entry-title","entry-slug","entry-excerpt","entry-seo-title","entry-seo-description","entry-canonical-url","entry-seo-robots-index","entry-seo-robots-follow","entry-schema-mode","entry-layout-template","entry-featured-media-id","entry-social-media-id","template-name","site-part-name"];
  const snap = {};
  ids.forEach(id => {
    const el = document.getElementById(id);
    if (el) snap[id] = el.value ?? el.textContent ?? "";
  });
  const form = document.getElementById("stratum-editor-form");
  const formEls = form ? form.elements : null;
  if (formEls) {
    ["visibility","password","sticky","review_state","scheduled_at","comments_enabled","parent_entry_id","menu_order"].forEach(name => {
      const el = formEls[name];
      if (el) snap[name] = el.value ?? (el.checked ? "1" : "0");
    });
    Array.from(form.elements).forEach(el => {
      if (el.name && el.name.startsWith("field_")) snap[el.name] = el.value;
      if (el.name && el.name.startsWith("taxonomy_")) {
        if (el.type === "checkbox") {
          if (!snap[el.name]) snap[el.name] = [];
          if (el.checked) snap[el.name].push(el.value);
        } else snap[el.name] = el.value;
      }
    });
  }
  return JSON.stringify(snap);
}

export function isDirtyNow() {
  const cur = JSON.stringify(state.document);
  if (cur !== persistedJSON) return true;
  const meta = captureMetaSnapshot();
  return meta !== persistedMeta;
}

export function updateDirty() {
  const isDirty = isDirtyNow();
  state.dirty = isDirty;
  const dirtyElement = document.getElementById("editor-dirty");
  if (dirtyElement) {
    dirtyElement.textContent = isDirty ? "Unsaved changes" : "Saved";
    dirtyElement.className = isDirty ? "editor-status is-dirty" : "editor-status is-saved";
  }
  return isDirty;
}

export function syncBaseline() {
  persistedJSON = JSON.stringify(state.document);
  persistedMeta = captureMetaSnapshot();
  updateDirty();
}

export function initHistory() {
  pushHistory();
}

// Hydration helpers needed early
function definitionFor(node) {
  return definitions.get(`${node.block}@${node.version}`);
}
function defaultValue(schema) {
  if (schema.default !== null && schema.default !== undefined) return JSON.parse(JSON.stringify(schema.default));
  if (schema.type === "object") {
    const result = {};
    Object.entries(schema.properties || {}).forEach(([name, child]) => {
      result[name] = defaultValue(child);
    });
    return result;
  }
  if (schema.type === "array") return [];
  if (schema.type === "boolean") return false;
  if (schema.type === "integer" || schema.type === "number") return schema.minimum ?? 0;
  return schema.enum?.[0] ?? "";
}
function hydrateObject(value, schema) {
  Object.entries(schema.properties || {}).forEach(([name, child]) => {
    if (value[name] === undefined) value[name] = defaultValue(child);
  });
}
function hydrateNode(node) {
  const definition = definitionFor(node);
  node.props ||= {};
  node.settings ||= {};
  node.children ||= [];
  if (definition) {
    hydrateObject(node.props, definition.schema.props);
    hydrateObject(node.settings, definition.schema.settings);
  }
  node.children.forEach(hydrateNode);
}
export { hydrateNode, definitionFor, defaultValue, hydrateObject };

state.document.nodes ||= [];
state.document.nodes.forEach(hydrateNode);
