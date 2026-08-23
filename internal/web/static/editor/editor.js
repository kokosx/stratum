// Stratum Block Editor
// Copyright (c) StratumCMS. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

(() => {
  "use strict";

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
  const publishStateEl = document.getElementById("editor-publish-state");
  const publicUrlEl = document.getElementById("editor-public-url");

  // --- State ---------------------------------------------------------------

  const state = {
    document: bootstrap.document,
    catalog: bootstrap.catalog,
    selectedNodeId: null,
    dirty: false,
    collapsed: new Set(),
    mode: "edit", // "edit" | "preview"
    previewWidth: "100%",
  };
  const definitions = new Map(
    [...state.catalog, ...(bootstrap.definitions || [])].map((item) => [`${item.block}@${item.version}`, item])
  );

  let currentDrag = null;
  let previewTimer = null;
  let metadataTimer = null;

  // --- History (P1.32) -----------------------------------------------------

  const history = [];
  let historyIndex = -1;
  const MAX_HISTORY = 50;
  let lastPushTime = 0;

  function pushHistory() {
    history.splice(historyIndex + 1);
    const snap = JSON.stringify(state.document);
    if (historyIndex >= 0 && history[historyIndex] === snap) return;
    history.push(snap);
    if (history.length > MAX_HISTORY) history.shift();
    historyIndex = history.length - 1;
  }

  function maybePushHistory() {
    const now = Date.now();
    if (now - lastPushTime < 500) return;
    const snap = JSON.stringify(state.document);
    if (historyIndex >= 0 && history[historyIndex] === snap) return;
    pushHistory();
    lastPushTime = now;
  }

  function undo() {
    if (historyIndex <= 0) return;
    historyIndex--;
    restoreSnapshot();
  }

  function redo() {
    if (historyIndex >= history.length - 1) return;
    historyIndex++;
    restoreSnapshot();
  }

  function restoreSnapshot() {
    state.document = JSON.parse(history[historyIndex]);
    state.document.nodes ||= [];
    state.document.nodes.forEach(hydrateNode);
    state.selectedNodeId = null;
    renderAll({ preview: true });
    markSaved();
  }

  // --- Utilities -----------------------------------------------------------

  const clone = (value) => JSON.parse(JSON.stringify(value));

  function randomID() {
    const bytes = new Uint8Array(16);
    crypto.getRandomValues(bytes);
    return "blk_" + Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
  }

  function element(tag, className, text) {
    const node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined) node.textContent = text;
    return node;
  }

  function humanize(value) {
    return value.replace(/([A-Z])/g, " $1").replace(/^./, (l) => l.toUpperCase());
  }

  // --- Schema helpers ------------------------------------------------------

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

  // --- Document queries ----------------------------------------------------

  function definitionFor(node) {
    return definitions.get(`${node.block}@${node.version}`);
  }

  function findNode(id, nodes = state.document.nodes, parent = null) {
    for (let i = 0; i < nodes.length; i++) {
      if (nodes[i].id === id) return { node: nodes[i], siblings: nodes, index: i, parent };
      const nested = findNode(id, nodes[i].children || [], nodes[i]);
      if (nested) return nested;
    }
    return null;
  }

  function isContainer(node) {
    const def = definitionFor(node);
    return def?.schema.children.mode !== "none";
  }

  function subtreeHasChildren(node) {
    return (node.children || []).length > 0 || (node.children || []).some(subtreeHasChildren);
  }

  // --- Drag & drop ---------------------------------------------------------

  function childrenAllow(definition, block, currentCount) {
    if (!definition) return false;
    const rule = definition.schema.children;
    if (rule.mode === "none") return false;
    if (rule.max !== undefined && rule.max !== null && currentCount >= rule.max) return false;
    return rule.mode === "any" || (rule.mode === "allowed" && rule.blocks.includes(block));
  }

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

  function isWithin(ancestorId, node) {
    if (node.id === ancestorId) return true;
    return (node.children || []).some((child) => isWithin(ancestorId, child));
  }

  function insertionPoint(block) {
    const selected = state.selectedNodeId && findNode(state.selectedNodeId);
    if (!selected) return { siblings: state.document.nodes, index: state.document.nodes.length };
    const def = definitionFor(selected.node);
    if (childrenAllow(def, block, selected.node.children.length)) {
      return { siblings: selected.node.children, index: selected.node.children.length };
    }
    if (!selected.parent) return { siblings: selected.siblings, index: selected.index + 1 };
    const parentDef = definitionFor(selected.parent);
    if (childrenAllow(parentDef, block, selected.siblings.length)) {
      return { siblings: selected.siblings, index: selected.index + 1 };
    }
    return { siblings: state.document.nodes, index: state.document.nodes.length };
  }

  function clearDropUI() {
    treeElement.querySelectorAll(".drop-slot--active, .drop-slot--invalid")
      .forEach((s) => s.classList.remove("drop-slot--active", "drop-slot--invalid"));
    treeElement.querySelectorAll(".node--droptarget").forEach((n) => n.classList.remove("node--droptarget"));
    treeElement.classList.remove("tree--droptarget", "tree--dragging");
  }

  function highlightContainer(containerNode) {
    if (!containerNode) { treeElement.classList.add("tree--droptarget"); return; }
    const el = treeElement.querySelector(`.node[data-node-id="${containerNode.id}"]`);
    if (el) el.classList.add("node--droptarget");
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
    const target = containerNode ? containerNode.children : state.document.nodes;
    let node;
    pushHistory();
    if (drag.type === "library") {
      node = createNode(drag.definition);
      target.splice(index, 0, node);
    } else {
      const found = findNode(drag.nodeId);
      if (!found) return;
      if (containerNode && isWithin(drag.nodeId, containerNode)) return;
      if (found.parent === containerNode && found.index === index) return;
      found.siblings.splice(found.index, 1);
      let ti = index;
      if (found.parent === containerNode && found.index < index) ti -= 1;
      target.splice(ti, 0, found.node);
      node = found.node;
    }
    state.selectedNodeId = node.id;
    changed();
  }

  // --- Node operations -----------------------------------------------------

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
    pushHistory();
    const point = insertionPoint(definition.block);
    const node = createNode(definition);
    point.siblings.splice(point.index, 0, node);
    state.selectedNodeId = node.id;
    changed();
  }

  function moveNode(id, offset) {
    const found = findNode(id);
    if (!found) return;
    const next = found.index + offset;
    if (next < 0 || next >= found.siblings.length) return;
    pushHistory();
    [found.siblings[found.index], found.siblings[next]] = [found.siblings[next], found.siblings[found.index]];
    changed();
  }

  function canIndent(id) {
    const found = findNode(id);
    if (!found || found.index < 1) return false;
    const prev = found.siblings[found.index - 1];
    const prevDef = definitionFor(prev);
    if (!prevDef || prevDef.schema.children.mode === "none") return false;
    return childrenAllow(prevDef, found.node.block, prev.children.length);
  }

  function indentNode(id) {
    const found = findNode(id);
    if (!found || found.index < 1) return;
    const prev = found.siblings[found.index - 1];
    const prevDef = definitionFor(prev);
    if (!prevDef || prevDef.schema.children.mode === "none") return;
    if (!childrenAllow(prevDef, found.node.block, prev.children.length)) return;
    pushHistory();
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
    pushHistory();
    const [moved] = found.siblings.splice(found.index, 1);
    const target = newContainer ? newContainer.children : state.document.nodes;
    target.splice(parentFound.index + 1, 0, moved);
    changed();
  }

  function removeNode(id) {
    const found = findNode(id);
    if (!found) return;
    // P1.33: confirm for container subtree removal
    if (subtreeHasChildren(found.node)) {
      const def = definitionFor(found.node);
      const label = def?.displayName || found.node.block;
      if (!confirm(`Remove "${label}" and all its children?`)) return;
    }
    pushHistory();
    found.siblings.splice(found.index, 1);
    state.collapsed.delete(id);
    if (state.selectedNodeId === id) state.selectedNodeId = found.parent?.id || null;
    changed();
  }

  // P1.39: duplicate with fresh IDs for entire subtree
  function duplicateSubtree(node) {
    const clone = JSON.parse(JSON.stringify(node));
    assignNewIDs(clone);
    return clone;
  }

  function assignNewIDs(node) {
    node.id = randomID();
    (node.children || []).forEach(assignNewIDs);
  }

  function duplicateNode(id) {
    const found = findNode(id);
    if (!found) return;
    pushHistory();
    const dupe = duplicateSubtree(found.node);
    found.siblings.splice(found.index + 1, 0, dupe);
    state.selectedNodeId = dupe.id;
    changed();
  }

  // --- Render cycle --------------------------------------------------------

  function markSaved() {
    state.dirty = false;
    if (dirtyElement) {
      dirtyElement.textContent = "Saved";
      dirtyElement.className = "editor-status is-saved";
    }
  }

  function changed(options = {}) {
    state.dirty = true;
    if (dirtyElement) {
      dirtyElement.textContent = "Unsaved";
      dirtyElement.className = "editor-status is-dirty";
    }
    documentInput.value = JSON.stringify(state.document);
    if (options.tree !== false) renderTree();
    if (options.inspector !== false) renderInspector();
    schedulePreview();
  }

  function renderAll(options = {}) {
    documentInput.value = JSON.stringify(state.document);
    renderTree();
    renderInspector();
    if (options.preview) schedulePreview();
  }

  // --- Catalog (P1.36: remove fake icon) -----------------------------------

  function renderCatalog(filter = "") {
    catalogElement.replaceChildren();
    const query = filter.trim().toLowerCase();
    const matches = state.catalog.filter((d) =>
      `${d.displayName} ${d.description || ""} ${d.block}`.toLowerCase().includes(query)
    );
    let category = null;
    matches
      .sort((a, b) =>
        (a.schema.editor.category || "other").localeCompare(b.schema.editor.category || "other") ||
        a.displayName.localeCompare(b.displayName)
      )
      .forEach((d) => {
        const nextCategory = d.schema.editor.category || "Other";
        if (nextCategory !== category) {
          category = nextCategory;
          catalogElement.append(element("h3", "catalog-category", category));
        }
        const button = element("button", "catalog-item");
        button.type = "button";
        button.draggable = true;
        button.dataset.block = d.block;
        button.dataset.version = String(d.version);
        button.addEventListener("dragstart", (event) => {
          currentDrag = { type: "library", definition: d };
          clearDropUI();
          treeElement.classList.add("tree--dragging");
          event.dataTransfer.effectAllowed = "copy";
          event.dataTransfer.setData("text/plain", `${d.block}@${d.version}`);
        });
        button.addEventListener("dragend", () => { currentDrag = null; clearDropUI(); });
        const title = element("strong");
        title.append(document.createTextNode(d.displayName));
        button.append(title);
        if (d.description) button.append(element("small", "", d.description));
        button.addEventListener("click", () => addBlock(d));
        catalogElement.append(button);
      });
    if (!matches.length) catalogElement.append(element("p", "editor-empty", "No matching blocks."));
  }

  // --- Tree ----------------------------------------------------------------

  function renderTree() {
    treeElement.replaceChildren();
    treeElement.append(renderDropZone(state.document.nodes, null));
    emptyElement.hidden = state.document.nodes.length > 0;
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

  // SummaryFields is the single source for editor node previews.
  // Blocks declare e.g. summaryFields: ["props.text"] and the editor uses
  // that instead of hard-coded branches. Hard-coded fallbacks remain for
  // legacy blocks without metadata.
  function nodeSummary(node) {
    const def = definitionFor(node);
    const fields = def?.schema?.editor?.summaryFields || def?.summaryFields;
    if (fields && fields.length) {
      for (const path of fields) {
        const parts = path.split(".");
        const scope = parts[0] === "props" ? node.props : parts[0] === "settings" ? node.settings : null;
        if (!scope) continue;
        const key = parts.slice(1).join(".");
        const val = scope[key];
        if (typeof val === "string" && val.trim()) return val.slice(0, 70);
        if (val) return String(val).slice(0, 70);
      }
    }
    const block = def?.block || node.block;
    const p = node.props || {};
    const s = node.settings || {};
    if (block === "core/heading" || block === "core/entry-title") return p.text || "";
    if (block === "core/text") return (p.text || "").slice(0, 70);
    if (block === "core/button") return p.label || "";
    if (block === "core/image") return p.alt || p.caption || (p.mediaId ? "" : "(no image)");
    if (block === "core/section") return s.width || "content";
    if (block === "core/stack") return `${s.direction || "vertical"} · ${s.gap || "md"}`;
    if (block === "core/grid") return `${s.columns || 2} cols`;
    if (block === "core/card") return s.variant || "default";
    if (block === "core/accordion") return s.variant || "minimal";
    if (block === "core/quote") return (p.text || "").slice(0, 50);
    if (block === "core/divider") return "";
    if (block === "core/icon") return p.name || "";
    if (block === "core/entry-excerpt") return "";
    if (block === "core/site-name") return "";
    // Fallback: first string prop value
    const value = Object.values(p).find((v) => typeof v === "string" && v);
    return value ? value.slice(0, 70) : "";
  }

  // P1.35: don't expose raw block identifier; displayName is the primary label.
  function renderNode(node) {
    const definition = definitionFor(node);
    const container = isContainer(node);
    const collapsed = state.collapsed.has(node.id);
    const wrapper = element("article",
      `node ${container ? "node--container" : "node--leaf"}${node.id === state.selectedNodeId ? " is-selected" : ""}${collapsed ? " is-collapsed" : ""}`
    );
    wrapper.dataset.nodeId = node.id;
    wrapper.draggable = true;
    wrapper.addEventListener("dragstart", (event) => {
      currentDrag = { type: "node", nodeId: node.id };
      clearDropUI();
      treeElement.classList.add("tree--dragging");
      event.dataTransfer.effectAllowed = "move";
      event.dataTransfer.setData("text/plain", node.id);
    });
    wrapper.addEventListener("dragend", () => { currentDrag = null; clearDropUI(); });

    const header = element("div", "node__header");
    header.addEventListener("click", () => {
      state.selectedNodeId = node.id;
      renderTree();
      renderInspector();
    });
    header.append(element("span", "node__drag", "⋮⋮"));
    if (definition) {
      const typeBadge = element("span", "node__type", definition.displayName);
      typeBadge.title = `${node.block}@${node.version}`;
      header.append(typeBadge);
    }
    header.append(element("span", "node__summary", nodeSummary(node)));

    const actions = element("span", "node__actions");

    // Collapse toggle for containers (P1.38)
    if (container && node.children.length > 0) {
      const collapseBtn = actionButton(collapsed ? "▸" : "▾", collapsed ? "Expand" : "Collapse", () => {
        if (state.collapsed.has(node.id)) state.collapsed.delete(node.id);
        else state.collapsed.add(node.id);
        renderTree();
      });
      collapseBtn.classList.add("node__collapse");
      actions.append(collapseBtn);
    }

    // Duplicate (P1.39)
    actions.append(actionButton("⧉", "Duplicate", () => duplicateNode(node.id)));

    if (canIndent(node.id)) actions.append(actionButton("↳", "Indent", () => indentNode(node.id)));
    if (canOutdent(node.id)) actions.append(actionButton("↰", "Outdent", () => outdentNode(node.id)));
    actions.append(actionButton("↑", "Move up", () => moveNode(node.id, -1)));
    actions.append(actionButton("↓", "Move down", () => moveNode(node.id, 1)));
    actions.append(actionButton("✕", "Remove", () => removeNode(node.id), true));
    actions.addEventListener("click", (event) => event.stopPropagation());
    header.append(actions);
    wrapper.append(header);

    if (container && !collapsed) wrapper.append(renderDropZone(node.children, node));
    if (container && collapsed) {
      const placeholder = element("div", "node__collapsed-hint", `${(node.children || []).length} items`);
      wrapper.append(placeholder);
    }
    return wrapper;
  }

  function actionButton(text, label, handler, danger) {
    const button = element("button", danger ? "node__action--danger" : "", text);
    button.type = "button";
    button.title = label;
    button.setAttribute("aria-label", label);
    button.addEventListener("click", (e) => { e.stopPropagation(); handler(); });
    return button;
  }

  // --- Inspector -----------------------------------------------------------

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
      fields.forEach((f) => fieldset.append(f));
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
      maybePushHistory();
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
    const sharedName = `field-${crypto.randomUUID()}`;
    (schema.enum || []).forEach((option) => {
      const label = element("label");
      const input = document.createElement("input");
      input.type = "radio";
      input.name = sharedName;
      input.checked = Object.is(option, value);
      input.addEventListener("change", () => input.checked && update(option));
      label.append(input, document.createTextNode(String(option)));
      options.append(label);
    });
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
      row.append(actionButton("✕", "Remove item", () => { values.splice(index, 1); changed({ tree: false }); }, true));
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

  function updateFromObject(object, name) {
    return (value) => {
      object[name] = value;
      maybePushHistory();
      changed({ tree: false, inspector: false });
      renderTree();
    };
  }

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

  // --- Preview -------------------------------------------------------------

  function schedulePreview() {
    clearTimeout(previewTimer);
    previewTimer = setTimeout(updatePreview, 400);
  }

  // P1.30: send title/excerpt/slug/seo in preview POST
  async function updatePreview() {
    const titleEl = document.getElementById("entry-title");
    const excerptEl = document.getElementById("entry-excerpt");
    const slugEl = document.getElementById("entry-slug");
    const seoTitleEl = document.getElementById("entry-seo-title");
    const seoDescEl = document.getElementById("entry-seo-description");
    const entryIdEl = document.getElementById("entry-id");
    const layoutEl = document.getElementById("entry-layout-template");
    const ctEl = document.getElementById("entry-content-type");

    const params = {
      csrf_token: form.elements.csrf_token.value,
      document_json: JSON.stringify(state.document),
      title: titleEl?.value || "",
      excerpt: excerptEl?.value || "",
      slug: slugEl?.value || "",
      seo_title: seoTitleEl?.value || "",
      seo_description: seoDescEl?.value || "",
      entry_id: entryIdEl?.value || "",
      layout_template_id: layoutEl?.value || "",
      content_type_id: ctEl?.value || "",
    };
    try {
      const response = await fetch(bootstrap.previewUrl, {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Requested-With": "StratumEditor" },
        body: new URLSearchParams(params),
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
      frame.style.width = state.previewWidth;
      frame.addEventListener("load", () => {
        try {
          const doc = frame.contentDocument;
          if (doc && doc.body) frame.style.height = `${Math.max(240, doc.body.scrollHeight + 32)}px`;
        } catch (_) { /* cross-origin */ }
      });
      previewElement.append(frame);
      if (errorElement) { errorElement.textContent = ""; errorElement.hidden = true; }
    } catch (error) {
      previewElement.classList.add("editor-preview-error");
      previewElement.textContent = error.message;
      if (errorElement) { errorElement.textContent = error.message; errorElement.hidden = false; }
    }
  }

  // P1.41: preview toggle + device width
  function setMode(mode) {
    state.mode = mode;
    const workspace = document.querySelector(".editor-workspace");
    const previewPanel = document.querySelector(".editor-preview-panel");
    if (mode === "preview") {
      if (workspace) workspace.style.display = "none";
      if (previewPanel) previewPanel.style.display = "block";
      schedulePreview();
    } else {
      if (workspace) workspace.style.display = "";
      if (previewPanel) previewPanel.style.display = "";
    }
    document.querySelectorAll(".editor-mode-btn").forEach((btn) => {
      btn.classList.toggle("is-active", btn.dataset.mode === mode);
    });
  }

  function setPreviewWidth(width) {
    state.previewWidth = width;
    document.querySelectorAll(".editor-device-btn").forEach((btn) => {
      btn.classList.toggle("is-active", btn.dataset.width === width);
    });
    const frame = previewElement.querySelector("iframe");
    if (frame) frame.style.width = width;
  }

  // --- Metadata change listeners (P1.29/30/31) -----------------------------

  function setupMetadataListeners() {
    const metadataFields = [
      "entry-title", "entry-slug", "entry-excerpt",
      "entry-seo-title", "entry-seo-description", "entry-canonical-url",
    ];
    metadataFields.forEach((id) => {
      const el = document.getElementById(id);
      if (!el) return;
      el.addEventListener("input", () => {
        state.dirty = true;
        if (dirtyElement) {
          dirtyElement.textContent = "Unsaved";
          dirtyElement.className = "editor-status is-dirty";
        }
        clearTimeout(metadataTimer);
        metadataTimer = setTimeout(updatePreview, 600);
      });
    });
    const layoutEl = document.getElementById("entry-layout-template");
    if (layoutEl) {
      layoutEl.addEventListener("change", () => {
        state.dirty = true;
        if (dirtyElement) {
          dirtyElement.textContent = "Unsaved";
          dirtyElement.className = "editor-status is-dirty";
        }
        schedulePreview();
      });
    }
  }

  // --- Auto-slug from title ------------------------------------------------

  function setupSlug() {
    const title = document.getElementById("entry-title");
    const slug = document.getElementById("entry-slug");
    if (!title || !slug) return;
    const normalizeSlug = (v) => v.toLowerCase().normalize("NFD")
      .replace(/[\u0300-\u036f]/g, "").replace(/ł/g, "l")
      .replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
    let lastAuto = normalizeSlug(title.value);
    let manualEdit = slug.value !== "" && slug.value !== lastAuto;
    title.addEventListener("input", () => {
      if (!manualEdit || slug.value === lastAuto) {
        slug.value = normalizeSlug(title.value);
        lastAuto = slug.value;
        manualEdit = false;
      }
    });
    slug.addEventListener("input", () => { manualEdit = slug.value !== normalizeSlug(title.value); });
  }

  // --- Keyboard shortcuts (P1.44) ------------------------------------------

  document.addEventListener("keydown", (event) => {
    const mod = event.metaKey || event.ctrlKey;
    if (mod && event.key === "s") {
      event.preventDefault();
      documentInput.value = JSON.stringify(state.document);
      form.requestSubmit();
    }
    if (mod && event.key === "z" && !event.shiftKey) {
      event.preventDefault();
      undo();
    }
    if (mod && (event.key === "Z" || (event.key === "z" && event.shiftKey) || event.key === "y")) {
      event.preventDefault();
      redo();
    }
  });

  // --- beforeunload (P1.31) ------------------------------------------------

  window.addEventListener("beforeunload", (event) => {
    if (state.dirty) {
      event.preventDefault();
      event.returnValue = "";
    }
  });

  // --- Bootstrap ------------------------------------------------------------

  document.getElementById("block-search").addEventListener("input", (event) => renderCatalog(event.target.value));
  form.addEventListener("submit", () => {
    documentInput.value = JSON.stringify(state.document);
    state.dirty = false;
  });

  // Mode + device toolbar event delegation
  document.addEventListener("click", (event) => {
    if (event.target.matches(".editor-mode-btn")) setMode(event.target.dataset.mode);
    if (event.target.matches(".editor-device-btn")) setPreviewWidth(event.target.dataset.width);
  });

  documentInput.value = JSON.stringify(state.document);
  pushHistory();
  renderCatalog();
  renderTree();
  renderInspector();
  updatePreview();
  setupMetadataListeners();
  setupSlug();

  // Expose for Datastar / toast integration
  window.stratumEditor = { undo, redo, setMode, setPreviewWidth };
})();
