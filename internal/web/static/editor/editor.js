const bootstrap = JSON.parse(document.getElementById("editor-bootstrap").textContent);
const form = document.getElementById("stratum-editor-form");
const documentInput = document.getElementById("document-json");
const catalogElement = document.getElementById("block-catalog");
const treeElement = document.getElementById("document-tree");
const inspectorElement = document.getElementById("block-inspector");
const emptyElement = document.getElementById("empty-document");
const dirtyElement = document.getElementById("editor-dirty");
const previewElement = document.getElementById("editor-preview");
const errorElement = document.getElementById("editor-error");

const state = {
  document: bootstrap.document,
  catalog: bootstrap.catalog,
  selectedNodeId: null,
  dirty: false,
};
const definitions = new Map([...state.catalog, ...(bootstrap.definitions || [])].map((item) => [`${item.block}@${item.version}`, item]));
let draggedNodeId = null;
let previewTimer = null;

const element = (tag, className, text) => {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
};

const clone = (value) => JSON.parse(JSON.stringify(value));

function defaultValue(schema) {
  if (schema.default !== null && schema.default !== undefined) return clone(schema.default);
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

function hydrateObject(value, schema) {
  Object.entries(schema.properties || {}).forEach(([name, child]) => {
    if (value[name] === undefined) value[name] = defaultValue(child);
  });
}

state.document.nodes ||= [];
state.document.nodes.forEach(hydrateNode);

function definitionFor(node) {
  return definitions.get(`${node.block}@${node.version}`);
}

function findNode(id, nodes = state.document.nodes, parent = null) {
  for (let index = 0; index < nodes.length; index += 1) {
    if (nodes[index].id === id) return { node: nodes[index], siblings: nodes, index, parent };
    const nested = findNode(id, nodes[index].children || [], nodes[index]);
    if (nested) return nested;
  }
  return null;
}

function childrenAllow(definition, block, currentCount) {
  if (!definition) return false;
  const rule = definition.schema.children;
  if (rule.mode === "none") return false;
  if (rule.max !== undefined && rule.max !== null && currentCount >= rule.max) return false;
  return rule.mode === "any" || (rule.mode === "allowed" && rule.blocks.includes(block));
}

function insertionPoint(block) {
  const selected = state.selectedNodeId && findNode(state.selectedNodeId);
  if (!selected) return { siblings: state.document.nodes, index: state.document.nodes.length };
  const selectedDefinition = definitionFor(selected.node);
  if (childrenAllow(selectedDefinition, block, selected.node.children.length)) {
    return { siblings: selected.node.children, index: selected.node.children.length };
  }
  if (!selected.parent) return { siblings: selected.siblings, index: selected.index + 1 };
  const parentDefinition = definitionFor(selected.parent);
  if (childrenAllow(parentDefinition, block, selected.siblings.length)) {
    return { siblings: selected.siblings, index: selected.index + 1 };
  }
  return { siblings: state.document.nodes, index: state.document.nodes.length };
}

function randomID() {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
}

function addBlock(definition) {
  const point = insertionPoint(definition.block);
  const node = {
    id: randomID(),
    block: definition.block,
    version: definition.version,
    props: defaultValue(definition.schema.props),
    settings: defaultValue(definition.schema.settings),
  };
  if (definition.schema.children.mode !== "none") node.children = [];
  point.siblings.splice(point.index, 0, node);
  state.selectedNodeId = node.id;
  changed();
}

function changed(options = {}) {
  state.dirty = true;
  dirtyElement.textContent = "Unsaved";
  dirtyElement.classList.add("is-dirty");
  documentInput.value = JSON.stringify(state.document);
  if (options.tree !== false) renderTree();
  if (options.inspector !== false) renderInspector();
  schedulePreview();
}

function renderCatalog(filter = "") {
  catalogElement.replaceChildren();
  const query = filter.trim().toLowerCase();
  const matches = state.catalog.filter((definition) =>
    `${definition.displayName} ${definition.description || ""} ${definition.block}`.toLowerCase().includes(query),
  );
  let category = null;
  matches
    .sort((a, b) => (a.schema.editor.category || "other").localeCompare(b.schema.editor.category || "other") || a.displayName.localeCompare(b.displayName))
    .forEach((definition) => {
      const nextCategory = definition.schema.editor.category || "Other";
      if (nextCategory !== category) {
        category = nextCategory;
        catalogElement.append(element("h3", "catalog-category", category));
      }
      const button = element("button", "catalog-item");
      button.type = "button";
      const title = element("strong");
      title.append(element("span", "catalog-icon", definition.schema.editor.icon ? "◇" : "+"));
      title.append(document.createTextNode(definition.displayName));
      button.append(title);
      if (definition.description) button.append(element("small", "", definition.description));
      button.addEventListener("click", () => addBlock(definition));
      catalogElement.append(button);
    });
  if (!matches.length) catalogElement.append(element("p", "editor-empty", "No matching blocks."));
}

function renderTree() {
  treeElement.replaceChildren();
  state.document.nodes.forEach((node) => treeElement.append(renderNode(node)));
  emptyElement.hidden = state.document.nodes.length > 0;
}

function renderNode(node) {
  const definition = definitionFor(node);
  const wrapper = element("article", `document-node${node.id === state.selectedNodeId ? " is-selected" : ""}`);
  wrapper.dataset.nodeId = node.id;
  wrapper.draggable = true;
  wrapper.addEventListener("dragstart", (event) => {
    draggedNodeId = node.id;
    event.dataTransfer.effectAllowed = "move";
  });
  wrapper.addEventListener("dragover", (event) => event.preventDefault());
  wrapper.addEventListener("drop", (event) => {
    event.preventDefault();
    if (!draggedNodeId || draggedNodeId === node.id) return;
    const from = findNode(draggedNodeId);
    const to = findNode(node.id);
    if (!from || !to || from.siblings !== to.siblings) return;
    const [moved] = from.siblings.splice(from.index, 1);
    const target = from.index < to.index ? to.index - 1 : to.index;
    to.siblings.splice(target, 0, moved);
    changed();
  });
  const header = element("div", "node-header");
  header.addEventListener("click", () => {
    state.selectedNodeId = node.id;
    renderTree();
    renderInspector();
  });
  header.append(element("span", "node-drag", "⋮⋮"));
  header.append(element("span", "node-label", definition?.displayName || `${node.block}@${node.version}`));
  header.append(element("span", "node-summary", nodeSummary(node)));
  const actions = element("span", "node-actions");
  actions.append(actionButton("↑", "Move up", () => moveNode(node.id, -1)));
  actions.append(actionButton("↓", "Move down", () => moveNode(node.id, 1)));
  actions.append(actionButton("×", "Remove", () => removeNode(node.id)));
  actions.addEventListener("click", (event) => event.stopPropagation());
  header.append(actions);
  wrapper.append(header);
  if (definition?.schema.children.mode !== "none") {
    const children = element("div", "node-children");
    if (!node.children.length) children.append(element("div", "node-children-empty", "Select this container, then add a block from the library."));
    node.children.forEach((child) => children.append(renderNode(child)));
    wrapper.append(children);
  }
  return wrapper;
}

function nodeSummary(node) {
  const value = Object.values(node.props || {}).find((item) => typeof item === "string" && item);
  return value ? value.slice(0, 70) : node.block;
}

function actionButton(text, label, handler) {
  const button = element("button", "", text);
  button.type = "button";
  button.title = label;
  button.setAttribute("aria-label", label);
  button.addEventListener("click", handler);
  return button;
}

function moveNode(id, offset) {
  const found = findNode(id);
  if (!found) return;
  const next = found.index + offset;
  if (next < 0 || next >= found.siblings.length) return;
  [found.siblings[found.index], found.siblings[next]] = [found.siblings[next], found.siblings[found.index]];
  changed();
}

function removeNode(id) {
  const found = findNode(id);
  if (!found) return;
  found.siblings.splice(found.index, 1);
  if (state.selectedNodeId === id) state.selectedNodeId = found.parent?.id || null;
  changed();
}

function renderInspector() {
  inspectorElement.replaceChildren();
  const found = state.selectedNodeId && findNode(state.selectedNodeId);
  if (!found) {
    inspectorElement.append(element("p", "editor-empty", "Select a block to edit it."));
    return;
  }
  const definition = definitionFor(found.node);
  if (!definition) {
    inspectorElement.append(element("p", "editor-preview-error", `Definition ${found.node.block}@${found.node.version} is unavailable.`));
    return;
  }
  inspectorElement.append(element("p", "inspector-title", definition.displayName));
  const groups = new Map();
  collectFields(groups, found.node, definition, "props", definition.schema.props, found.node.props, "Content");
  collectFields(groups, found.node, definition, "settings", definition.schema.settings, found.node.settings, "Style");
  groups.forEach((fields, groupName) => {
    const fieldset = element("fieldset", "inspector-group");
    fieldset.append(element("legend", "", groupName));
    fields.forEach((field) => fieldset.append(field));
    inspectorElement.append(fieldset);
  });
}

function collectFields(groups, node, definition, prefix, schema, object, defaultGroup) {
  Object.entries(schema.properties || {}).forEach(([name, fieldSchema]) => {
    const path = `${prefix}.${name}`;
    const metadata = definition.schema.editor.fields?.[path] || {};
    const group = metadata.group || defaultGroup;
    if (!groups.has(group)) groups.set(group, []);
    groups.get(group).push(buildField(node, object, name, fieldSchema, metadata, path));
  });
}

function buildField(node, object, name, schema, metadata, path) {
  const wrapper = element("label", "inspector-field");
  wrapper.append(element("span", "", metadata.label || humanize(name)));
  if (schema.type === "array") {
    wrapper.append(buildArray(node, object, name, schema, path));
    return wrapper;
  }
  if (schema.type === "object") {
    const nested = element("div", "array-items");
    Object.entries(schema.properties || {}).forEach(([childName, childSchema]) => {
      nested.append(buildField(node, object[name], childName, childSchema, {}, `${path}.${childName}`));
    });
    wrapper.append(nested);
    return wrapper;
  }
  const control = metadata.control || inferredControl(schema);
  const factory = controlFactories[control] || controlFactories.text;
  wrapper.append(factory(schema, object[name], (value) => {
    object[name] = value;
    changed({ tree: false, inspector: false });
    renderTree();
  }, path));
  return wrapper;
}

const controlFactories = {
  text: (schema, value, update) => inputControl("text", value, update),
  textarea: (schema, value, update) => {
    const input = element("textarea");
    input.rows = 5;
    input.value = value ?? "";
    input.addEventListener("input", () => update(input.value));
    return input;
  },
  number: (schema, value, update) => numberControl("number", schema, value, update),
  range: (schema, value, update) => numberControl("range", schema, value, update),
  checkbox: (schema, value, update) => {
    const input = document.createElement("input");
    input.type = "checkbox";
    input.checked = Boolean(value);
    input.addEventListener("change", () => update(input.checked));
    return input;
  },
  select: (schema, value, update) => optionControl("select", schema, value, update),
  segmented: (schema, value, update) => optionControl("segmented", schema, value, update),
  radio: (schema, value, update) => optionControl("radio", schema, value, update),
};

function inputControl(type, value, update) {
  const input = document.createElement("input");
  input.type = type;
  input.value = value ?? "";
  input.addEventListener("input", () => update(input.value));
  return input;
}

function numberControl(type, schema, value, update) {
  const input = inputControl(type, value, () => update(input.valueAsNumber));
  if (schema.minimum !== undefined && schema.minimum !== null) input.min = schema.minimum;
  if (schema.maximum !== undefined && schema.maximum !== null) input.max = schema.maximum;
  if (schema.type === "integer") input.step = "1";
  return input;
}

function optionControl(control, schema, value, update) {
  if (control === "select") {
    const select = document.createElement("select");
    (schema.enum || []).forEach((option) => {
      const item = element("option", "", String(option));
      item.value = JSON.stringify(option);
      item.selected = Object.is(option, value);
      select.append(item);
    });
    select.addEventListener("change", () => update(JSON.parse(select.value)));
    return select;
  }
  const options = element("div", "inspector-options");
  (schema.enum || []).forEach((option) => {
    const label = element("label");
    const input = document.createElement("input");
    input.type = "radio";
    input.name = `field-${crypto.randomUUID()}`;
    input.checked = Object.is(option, value);
    input.addEventListener("change", () => input.checked && update(option));
    label.append(input, document.createTextNode(String(option)));
    options.append(label);
  });
  const names = options.querySelectorAll("input");
  const sharedName = `field-${crypto.randomUUID()}`;
  names.forEach((input) => { input.name = sharedName; });
  return options;
}

function buildArray(node, object, name, schema, path) {
  const container = element("div", "array-items");
  const values = object[name] || [];
  values.forEach((value, index) => {
    const row = element("div", "array-item");
    if (schema.items.type === "object") {
      const fields = element("div");
      Object.entries(schema.items.properties || {}).forEach(([childName, childSchema]) => {
        fields.append(buildField(node, value, childName, childSchema, {}, `${path}[${index}].${childName}`));
      });
      row.append(fields);
    } else {
      const factory = controlFactories[inferredControl(schema.items)] || controlFactories.text;
      row.append(factory(schema.items, value, (next) => { values[index] = next; changed({ tree: false }); }, `${path}[${index}]`));
    }
    row.append(actionButton("×", "Remove item", () => { values.splice(index, 1); changed({ tree: false }); }));
    container.append(row);
  });
  const add = element("button", "button", "Add item");
  add.type = "button";
  add.addEventListener("click", () => { values.push(defaultValue(schema.items)); changed({ tree: false }); });
  container.append(add);
  return container;
}

function inferredControl(schema) {
  if (schema.enum?.length) return "select";
  if (schema.type === "boolean") return "checkbox";
  if (schema.type === "integer" || schema.type === "number") return "number";
  return "text";
}

function humanize(value) {
  return value.replace(/([A-Z])/g, " $1").replace(/^./, (letter) => letter.toUpperCase());
}

function schedulePreview() {
  clearTimeout(previewTimer);
  previewTimer = setTimeout(updatePreview, 320);
}

async function updatePreview() {
  const body = new URLSearchParams({
    csrf_token: form.elements.csrf_token.value,
    document_json: JSON.stringify(state.document),
  });
  try {
    const response = await fetch(bootstrap.previewUrl, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Requested-With": "StratumEditor" },
      body,
    });
    const output = await response.text();
    if (!response.ok) throw new Error(output.trim() || "Preview failed");
    previewElement.classList.remove("editor-preview-error");
    previewElement.innerHTML = output;
    errorElement.hidden = true;
  } catch (error) {
    previewElement.classList.add("editor-preview-error");
    previewElement.textContent = error.message;
    errorElement.textContent = error.message;
    errorElement.hidden = false;
  }
}

document.getElementById("block-search").addEventListener("input", (event) => renderCatalog(event.target.value));
document.getElementById("refresh-preview").addEventListener("click", updatePreview);
form.addEventListener("submit", () => { documentInput.value = JSON.stringify(state.document); });

const title = document.getElementById("page-title");
const slug = document.getElementById("page-slug");
const normalizeSlug = (value) => value.toLowerCase().normalize("NFD").replace(/[\u0300-\u036f]/g, "").replace(/ł/g, "l").replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
let lastAutoSlug = normalizeSlug(title.value);
let manuallyEditedSlug = slug.value !== "" && slug.value !== lastAutoSlug;
title.addEventListener("input", () => {
  if (!manuallyEditedSlug || slug.value === lastAutoSlug) {
    slug.value = normalizeSlug(title.value);
    lastAutoSlug = slug.value;
    manuallyEditedSlug = false;
  }
});
slug.addEventListener("input", () => { manuallyEditedSlug = slug.value !== normalizeSlug(title.value); });

documentInput.value = JSON.stringify(state.document);
renderCatalog();
renderTree();
renderInspector();
updatePreview();
