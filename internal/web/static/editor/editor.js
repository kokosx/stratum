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

// currentDrag describes what is being dragged right now:
//   { type: "library", definition }  -> a new block type from the Block Library
//   { type: "node", nodeId }         -> an existing node being moved
let currentDrag = null;
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

function isWithin(ancestorId, node) {
  if (node.id === ancestorId) return true;
  return (node.children || []).some((child) => isWithin(ancestorId, child));
}

function childrenAllow(definition, block, currentCount) {
  if (!definition) return false;
  const rule = definition.schema.children;
  if (rule.mode === "none") return false;
  if (rule.max !== undefined && rule.max !== null && currentCount >= rule.max) return false;
  return rule.mode === "any" || (rule.mode === "allowed" && rule.blocks.includes(block));
}

// containerAccepts decides whether a block may be placed inside containerNode
// (null means the root Document, which accepts anything). drag is used to relax
// the max-count rule when the move is a pure reorder inside the same container.
function containerAccepts(containerNode, block, drag) {
  if (!containerNode) return true;
  const definition = definitionFor(containerNode);
  if (!definition) return false;
  const rule = definition.schema.children;
  if (rule.mode === "none") return false;
  if (rule.mode === "allowed" && !rule.blocks.includes(block)) return false;
  if (rule.max !== undefined && rule.max !== null) {
    const count = containerNode.children.length;
    const sameContainer = drag.type === "node" && findNode(drag.nodeId)?.parent === containerNode;
    if (!sameContainer && count >= rule.max) return false;
  }
  return true;
}

function dragBlock(drag) {
  if (drag.type === "library") return drag.definition.block;
  return definitionFor(findNode(drag.nodeId).node).block;
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
  return "blk_" + Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
}

function createNode(definition) {
  const node = {
    id: randomID(),
    block: definition.block,
    version: definition.version,
    props: defaultValue(definition.schema.props),
    settings: defaultValue(definition.schema.settings),
  };
  if (definition.schema.children.mode !== "none") node.children = [];
  return node;
}

function addBlock(definition) {
  const point = insertionPoint(definition.block);
  const node = createNode(definition);
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

function clearDropUI() {
  treeElement.querySelectorAll(".drop-slot--active, .drop-slot--invalid")
    .forEach((slot) => slot.classList.remove("drop-slot--active", "drop-slot--invalid"));
  treeElement.querySelectorAll(".node--droptarget").forEach((node) => node.classList.remove("node--droptarget"));
  treeElement.classList.remove("tree--droptarget");
  treeElement.classList.remove("tree--dragging");
}

function highlightContainer(containerNode) {
  if (!containerNode) {
    treeElement.classList.add("tree--droptarget");
    return;
  }
  const article = treeElement.querySelector(`.node[data-node-id="${containerNode.id}"]`);
  if (article) article.classList.add("node--droptarget");
}

function attachSlot(slot, containerNode, index) {
  slot.addEventListener("dragover", (event) => {
    if (!currentDrag) return;
    event.preventDefault();
    const block = dragBlock(currentDrag);
    const valid = containerAccepts(containerNode, block, currentDrag)
      && !(currentDrag.type === "node" && containerNode && isWithin(currentDrag.nodeId, containerNode));
    slot.classList.remove("drop-slot--active", "drop-slot--invalid");
    if (valid) {
      event.dataTransfer.dropEffect = currentDrag.type === "library" ? "copy" : "move";
      slot.classList.add("drop-slot--active");
      highlightContainer(containerNode);
    } else {
      event.dataTransfer.dropEffect = "none";
      slot.classList.add("drop-slot--invalid");
    }
  });
  slot.addEventListener("dragleave", () => {
    slot.classList.remove("drop-slot--active", "drop-slot--invalid");
  });
  slot.addEventListener("drop", (event) => {
    event.preventDefault();
    slot.classList.remove("drop-slot--active", "drop-slot--invalid");
    performDrop(currentDrag, containerNode, index);
    clearDropUI();
  });
}

function performDrop(drag, containerNode, index) {
  if (!drag) return;
  const block = dragBlock(drag);
  if (!containerAccepts(containerNode, block, drag)) return;
  const targetSiblings = containerNode ? containerNode.children : state.document.nodes;
  let node;
  if (drag.type === "library") {
    node = createNode(drag.definition);
    targetSiblings.splice(index, 0, node);
  } else {
    const found = findNode(drag.nodeId);
    if (!found) return;
    if (containerNode && isWithin(drag.nodeId, containerNode)) return;
    if (found.parent === containerNode && found.index === index) return;
    found.siblings.splice(found.index, 1);
    let targetIndex = index;
    if (found.parent === containerNode && found.index < index) targetIndex -= 1;
    targetSiblings.splice(targetIndex, 0, found.node);
    node = found.node;
  }
  state.selectedNodeId = node.id;
  changed();
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
      button.draggable = true;
      button.dataset.block = definition.block;
      button.dataset.version = String(definition.version);
      button.addEventListener("dragstart", (event) => {
        currentDrag = { type: "library", definition };
        clearDropUI();
        treeElement.classList.add("tree--dragging");
        event.dataTransfer.effectAllowed = "copy";
        event.dataTransfer.setData("text/plain", `${definition.block}@${definition.version}`);
      });
      button.addEventListener("dragend", () => {
        currentDrag = null;
        clearDropUI();
      });
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
  treeElement.append(renderDropZone(state.document.nodes, null));
  emptyElement.hidden = true;
}

function renderDropZone(siblings, containerNode) {
  const zone = element("div", containerNode ? "node__children" : "tree__root");
  if (siblings.length === 0) {
    const slot = element("div", "drop-slot drop-slot--empty");
    slot.append(element("div", "node__empty", containerNode ? "Drop blocks here" : "Drag blocks here to begin"));
    attachSlot(slot, containerNode, 0);
    zone.append(slot);
    return zone;
  }
  siblings.forEach((child, index) => {
    const slot = element("div", "drop-slot");
    attachSlot(slot, containerNode, index);
    zone.append(slot);
    zone.append(renderNode(child));
  });
  const tail = element("div", "drop-slot");
  attachSlot(tail, containerNode, siblings.length);
  zone.append(tail);
  return zone;
}

function renderNode(node) {
  const definition = definitionFor(node);
  const isContainer = definition?.schema.children.mode !== "none";
  const wrapper = element("article", `node ${isContainer ? "node--container" : "node--leaf"}${node.id === state.selectedNodeId ? " is-selected" : ""}`);
  wrapper.dataset.nodeId = node.id;
  wrapper.draggable = true;
  wrapper.addEventListener("dragstart", (event) => {
    currentDrag = { type: "node", nodeId: node.id };
    clearDropUI();
    treeElement.classList.add("tree--dragging");
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData("text/plain", node.id);
  });
  wrapper.addEventListener("dragend", () => {
    currentDrag = null;
    clearDropUI();
  });
  const header = element("div", "node__header");
  header.addEventListener("click", () => {
    state.selectedNodeId = node.id;
    renderTree();
    renderInspector();
  });
  header.append(element("span", "node__drag", "⋮⋮"));
  if (definition) header.append(element("span", "node__type", definition.displayName));
  header.append(element("span", "node__label", node.block));
  header.append(element("span", "node__summary", nodeSummary(node)));
  const actions = element("span", "node__actions");
  if (canOutdent(node.id)) actions.append(actionButton("↰", "Outdent", () => outdentNode(node.id)));
  if (canIndent(node.id)) actions.append(actionButton("↳", "Indent", () => indentNode(node.id)));
  actions.append(actionButton("↑", "Move up", () => moveNode(node.id, -1)));
  actions.append(actionButton("↓", "Move down", () => moveNode(node.id, 1)));
  actions.append(actionButton("×", "Remove", () => removeNode(node.id)));
  actions.addEventListener("click", (event) => event.stopPropagation());
  header.append(actions);
  wrapper.append(header);
  if (isContainer) wrapper.append(renderDropZone(node.children, node));
  return wrapper;
}

function nodeSummary(node) {
  const value = Object.values(node.props || {}).find((item) => typeof item === "string" && item);
  return value ? value.slice(0, 70) : "";
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

function canIndent(id) {
  const found = findNode(id);
  if (!found || found.index < 1) return false;
  const prev = found.siblings[found.index - 1];
  const prevDefinition = definitionFor(prev);
  if (!prevDefinition || prevDefinition.schema.children.mode === "none") return false;
  return childrenAllow(prevDefinition, found.node.block, prev.children.length);
}

function indentNode(id) {
  const found = findNode(id);
  if (!found || found.index < 1) return;
  const prev = found.siblings[found.index - 1];
  const prevDefinition = definitionFor(prev);
  if (!prevDefinition || prevDefinition.schema.children.mode === "none") return;
  if (!childrenAllow(prevDefinition, found.node.block, prev.children.length)) return;
  const [moved] = found.siblings.splice(found.index, 1);
  prev.children.push(moved);
  changed();
}

function canOutdent(id) {
  const found = findNode(id);
  if (!found || !found.parent) return false;
  const parentFound = findNode(found.parent.id);
  if (!parentFound) return false;
  const newContainer = parentFound.parent;
  if (!newContainer) return true;
  return childrenAllow(definitionFor(newContainer), found.node.block, newContainer.children.length);
}

function outdentNode(id) {
  const found = findNode(id);
  if (!found || !found.parent) return;
  const parentFound = findNode(found.parent.id);
  if (!parentFound) return;
  const newContainer = parentFound.parent;
  if (newContainer && !childrenAllow(definitionFor(newContainer), found.node.block, newContainer.children.length)) return;
  const [moved] = found.siblings.splice(found.index, 1);
  const targetSiblings = newContainer ? newContainer.children : state.document.nodes;
  targetSiblings.splice(parentFound.index + 1, 0, moved);
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
  if (control === "media") {
    wrapper.append(buildMediaControl(node, object, name, updateFromObject(object, name)));
    return wrapper;
  }
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

// updateFromObject builds the standard change handler for a single prop/setting.
function updateFromObject(object, name) {
  return (value) => {
    object[name] = value;
    changed({ tree: false, inspector: false });
    renderTree();
  };
}

// buildMediaControl renders the "media" inspector control: a preview, choose /
// replace / remove actions, and an accessibility hint when a meaningful image
// lacks alt text. It opens the shared Media Picker for selection.
function buildMediaControl(node, object, name, update) {
  const container = element("div", "inspector-media");

  function openPicker() {
    if (!window.openMediaPicker) return;
    window.openMediaPicker({ onSelect: (asset) => { update(asset.id); render(); } });
  }

  function render() {
    const mediaId = object[name] || "";
    container.replaceChildren();
    if (mediaId) {
      const preview = element("div", "inspector-media__preview");
      const img = element("img");
      img.alt = "";
      img.src = "/media/" + mediaId + "/480";
      img.onerror = () => { img.onerror = null; img.src = "/media/" + mediaId + "/original"; };
      preview.append(img);
      container.append(preview);

      const actions = element("div", "inspector-media__actions");
      const replace = element("button", "button", "Replace");
      replace.type = "button";
      replace.addEventListener("click", openPicker);
      const remove = element("button", "button button-danger", "Remove");
      remove.type = "button";
      remove.addEventListener("click", () => { update(""); render(); });
      actions.append(replace, remove);
      container.append(actions);

      const alt = (node.props && node.props.alt) || object.alt || "";
      const decorative = !!(node.settings && node.settings.decorative);
      if (!decorative && !alt) {
        container.append(element("p", "inspector-media__warning", "No alt text — add one for accessibility."));
      }
    } else {
      container.append(element("div", "inspector-media__empty", "No image selected"));
      const choose = element("button", "button button-primary", "Choose image");
      choose.type = "button";
      choose.addEventListener("click", openPicker);
      container.append(choose);
    }
  }

  render();
  return container;
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
    previewElement.replaceChildren();
    const frame = document.createElement("iframe");
    frame.className = "editor-preview-frame";
    frame.title = "Live preview";
    frame.setAttribute("sandbox", "allow-same-origin");
    frame.srcdoc = output;
    frame.addEventListener("load", () => {
      try {
        const doc = frame.contentDocument;
        if (doc && doc.body) frame.style.height = `${Math.max(240, doc.body.scrollHeight + 32)}px`;
      } catch (error) { /* cross-origin guard */ }
    });
    previewElement.append(frame);
    errorElement.textContent = "";
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

const title = document.getElementById("entry-title");
const slug = document.getElementById("entry-slug");
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
