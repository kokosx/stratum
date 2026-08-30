// inspector.js — schema-driven block inspector + document panel
import { state, bootstrap, definitions, definitionFor, maybePushHistory } from "./state.js";
import { findNode } from "./tree.js";

function element(tag, className, text) {
  const n = document.createElement(tag);
  if (className) n.className = className;
  if (text !== undefined) n.textContent = text;
  return n;
}
function humanize(v) { return v.replace(/([A-Z])/g, " $1").replace(/^./, (l) => l.toUpperCase()); }
function inferredControl(schema) {
  if (schema.enum?.length) return "select";
  if (schema.type === "boolean") return "checkbox";
  if (schema.type === "integer" || schema.type === "number") return "number";
  return "text";
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
  const sharedName = `field-${Math.random().toString(36).slice(2,9)}`;
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

function scopedContentType(node) {
  let current = node;
  while (current) {
    if (current.block === "core/collection" && current.settings?.contentType) return current.settings.contentType;
    current = findNode(current.id)?.parent || null;
  }
  return bootstrap.contentTypeId || bootstrap.resource?.contentTypeId || "page";
}
function dynamicOptions(source, node) {
  if (source === "content-types") return bootstrap.contentTypes || [];
  if (source === "forms") return bootstrap.forms || [];
  if (source === "entry-fields") {
    const options = bootstrap.fieldCatalogs?.[scopedContentType(node)] || [];
    return node.block === "core/entry-media" ? options.filter((o) => o.type === "media") : options.filter((o) => o.type !== "media");
  }
  if (source === "taxonomies") {
    const ct = scopedContentType(node);
    const cats = bootstrap.taxonomyCatalogs?.[ct] || [];
    return cats.map(t => ({ value: t.id, label: t.label }));
  }
  if (source === "taxonomy-terms") {
    const ct = scopedContentType(node);
    const taxonomyId = node.settings?.taxonomyId || "";
    const cats = bootstrap.taxonomyCatalogs?.[ct] || [];
    const tax = cats.find(t => t.id === taxonomyId);
    if (!tax) return [];
    return tax.terms.map(term => ({ value: term.id, label: term.label }));
  }
  return [];
}
function fieldTypeFor(field, ct) {
  const opts = bootstrap.fieldCatalogs?.[ct] || [];
  const found = opts.find(o => o.value === field);
  return found ? found.type : null;
}
function operatorsForType(type) {
  switch (type) {
    case "text": case "textarea": return ["equals","not_equals","contains","exists","not_exists"];
    case "number": return ["equals","not_equals","greater_than","greater_or_equal","less_than","less_or_equal","exists","not_exists"];
    case "boolean": return ["is_true","is_false","exists","not_exists"];
    case "date": case "datetime": return ["equals","before","after","exists","not_exists"];
    case "select": return ["equals","not_equals","exists","not_exists"];
    case "url": case "email": return ["equals","not_equals","contains","exists","not_exists"];
    case "media": return ["exists","not_exists","equals"];
    default: return ["equals","not_equals","contains","exists","not_exists"];
  }
}
function needsValue(op) { return !["exists","not_exists","is_true","is_false"].includes(op); }
function valueControlForType(fieldType, operator, value, fieldOptions, update) {
  if (!needsValue(operator)) {
    const span = element("span", "filter-value--none", "—");
    span.setAttribute("aria-hidden","true");
    return span;
  }
  if (fieldType === "number") {
    const inp = document.createElement("input");
    inp.type = "number"; inp.value = value ?? "";
    inp.step = "any";
    inp.addEventListener("input", () => update(inp.value));
    return inp;
  }
  if (fieldType === "date") {
    const inp = document.createElement("input");
    inp.type = "date"; inp.value = value ?? "";
    inp.addEventListener("input", () => update(inp.value));
    return inp;
  }
  if (fieldType === "datetime") {
    const inp = document.createElement("input");
    inp.type = "datetime-local"; inp.value = value ?? "";
    inp.addEventListener("input", () => update(inp.value));
    return inp;
  }
  if (fieldType === "boolean") {
    const inp = document.createElement("input");
    inp.type = "checkbox"; inp.checked = Boolean(value);
    inp.addEventListener("change", () => update(inp.checked));
    return inp;
  }
  if (fieldType === "select" && fieldOptions && fieldOptions.length) {
    const sel = document.createElement("select");
    const empty = element("option", "", "Select…");
    empty.value = ""; sel.append(empty);
    fieldOptions.forEach(opt => {
      const o = element("option", "", opt);
      o.value = opt; o.selected = opt === value; sel.append(o);
    });
    if (value && !fieldOptions.includes(value)) {
      const miss = element("option", "", `${value} (unavailable)`);
      miss.value = value; miss.selected = true; sel.append(miss);
    }
    sel.addEventListener("change", () => update(sel.value));
    return sel;
  }
  const inp = document.createElement("input");
  inp.type = "text"; inp.value = value ?? "";
  inp.placeholder = "Value";
  inp.addEventListener("input", () => update(inp.value));
  return inp;
}
function dynamicOptionControl(source, node, value, update) {
  const options = dynamicOptions(source, node);
  if (source === "forms" && options.length === 0) {
    const wrap = element("div", "inspector-empty-forms");
    const p = element("p", "", "Create a form first.");
    const a = element("a", "", "Go to Forms");
    a.href = "/admin/forms";
    a.style.textDecoration = "underline";
    wrap.append(p);
    wrap.append(a);
    return wrap;
  }
  const container = element("div", "inspector-select-wrap");
  const select = document.createElement("select");
  if (source === "forms" && (!value || value === "")) {
    const ph = element("option", "", "Select a form…");
    ph.value = "";
    ph.selected = true;
    select.append(ph);
  }
  if (value && !options.some((o) => o.value === value)) {
    const missing = element("option", "", `${value} (unavailable)`);
    missing.value = value; missing.selected = true; select.append(missing);
  }
  options.forEach((o) => {
    const item = element("option", "", o.label);
    item.value = o.value; item.selected = o.value === value; select.append(item);
  });
  select.addEventListener("change", () => { update(select.value); if (node.block === "core/collection") renderInspector(); else { if (window.__stratum_changed) window.__stratum_changed({ tree: false, inspector: false }); renderTreeFallback(); } });
  container.append(select);
  if (source === "forms" && (!value || value === "")) {
    const hint = element("p", "form-error", "Form configuration required. Choose a Form before publishing.");
    hint.style.color = "#8a1c1c";
    hint.style.fontSize = "0.85rem";
    hint.style.marginTop = "0.35rem";
    container.append(hint);
  }
  return container;
}
function renderTreeFallback() {
  if (window.__stratum_renderNavigator) window.__stratum_renderNavigator();
  // legacy tree fallback
  const treeEl = document.getElementById("document-tree");
  if (treeEl && window.__stratum_renderTree) window.__stratum_renderTree();
}

export function renderInspector(externalInfo = null) {
  const inspectorElement = document.getElementById("block-inspector");
  if (!inspectorElement) return;
  inspectorElement.replaceChildren();
  // Handle external boundary
  if (externalInfo && !externalInfo.editable) {
    const wrap = element("div", "inspector-external");
    const title = element("h3", "", externalInfo.ownerType ? externalInfo.ownerType : "External content");
    wrap.append(title);
    const note = element("p", "muted", "This content comes from another resource and is read-only here.");
    wrap.append(note);
    if (externalInfo.ownerType && externalInfo.ownerId) {
      const btn = element("button", "button button-primary", `Edit ${externalInfo.ownerType}`);
      btn.type = "button";
      btn.addEventListener("click", () => {
        let url = "";
        if (externalInfo.ownerType === "site-part" || externalInfo.ownerType === "sitepart") url = `/admin/appearance/site-parts/${externalInfo.ownerId}/edit`;
        else if (externalInfo.ownerType === "layout-template") url = `/admin/appearance/templates/${externalInfo.ownerId}/edit`;
        else if (externalInfo.ownerType === "entry") url = `/admin/pages/${externalInfo.ownerId}/edit`;
        if (url) window.location.href = url;
      });
      wrap.append(btn);
    } else {
      const info = element("p", "muted", "Edit the source to change this content.");
      wrap.append(info);
    }
    // Show node that was clicked for context
    const nodeId = externalInfo.nodeId;
    const found = findNode(nodeId);
    if (found) {
      const def = definitionFor(found.node);
      if (def) wrap.append(element("p", "inspector-title", def.displayName));
    }
    inspectorElement.append(wrap);
    return;
  }
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
  const control = metadata.control || inferredControl(schema);
  if (control === "richtext") {
    wrapper.append(buildRichTextControl(object[name], (value) => {
      object[name] = value;
      maybePushHistory();
      if (window.__stratum_changed) window.__stratum_changed({ tree: false, inspector: false });
      renderTreeFallback();
    }));
    return wrapper;
  }
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
  if (control === "media") {
    wrapper.append(buildMediaControl(node, object, name, updateFromObject(object, name)));
    return wrapper;
  }
  if (control === "media-multiple") {
    wrapper.append(buildMediaMultipleControl(node, object, name, updateFromObject(object, name)));
    return wrapper;
  }
  if (control === "select" && metadata.optionsSource) {
    wrapper.append(dynamicOptionControl(metadata.optionsSource, node, object[name], (value) => {
      object[name] = value;
      maybePushHistory();
      if (window.__stratum_changed) window.__stratum_changed({ tree: false, inspector: false });
      renderTreeFallback();
    }));
    return wrapper;
  }
  const factory = controlFactories[control] || controlFactories.text;
  wrapper.append(factory(schema, object[name], (value) => {
    object[name] = value;
    maybePushHistory();
    if (window.__stratum_changed) window.__stratum_changed({ tree: false, inspector: false });
    renderTreeFallback();
  }, path));
  return wrapper;
}
function updateFromObject(object, name) {
  return (value) => {
    object[name] = value;
    maybePushHistory();
    if (window.__stratum_changed) window.__stratum_changed({ tree: false, inspector: false });
    renderTreeFallback();
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
function buildMediaMultipleControl(node, object, name, update) {
  const container = element("div", "inspector-media-multiple");
  function render() {
    const ids = Array.isArray(object[name]) ? object[name] : [];
    container.replaceChildren();
    if (ids.length) {
      ids.forEach((mediaId, idx) => {
        const row = element("div", "inspector-media-multiple__row");
        const thumb = element("img");
        thumb.alt = "";
        thumb.src = "/media/" + mediaId + "/480";
        thumb.style.width = "48px";
        thumb.style.height = "48px";
        thumb.style.objectFit = "cover";
        thumb.style.borderRadius = "4px";
        thumb.onerror = () => { thumb.onerror = null; thumb.src = "/media/" + mediaId + "/original"; };
        row.append(thumb);
        const label = element("span", "", mediaId.slice(0,8));
        label.title = mediaId;
        label.style.fontSize = "11px";
        label.style.color = "#64748b";
        row.append(label);
        const actions = element("div", "inspector-media-multiple__actions");
        const up = element("button", "button", "↑");
        up.type = "button"; up.title = "Move up"; up.disabled = idx === 0;
        up.addEventListener("click", () => { if (idx===0) return; const nxt = [...ids]; [nxt[idx-1], nxt[idx]]=[nxt[idx], nxt[idx-1]]; update(nxt); render(); });
        const down = element("button", "button", "↓");
        down.type = "button"; down.title = "Move down"; down.disabled = idx === ids.length-1;
        down.addEventListener("click", () => { if (idx===ids.length-1) return; const nxt = [...ids]; [nxt[idx], nxt[idx+1]]=[nxt[idx+1], nxt[idx]]; update(nxt); render(); });
        const remove = element("button", "button button-danger", "Remove");
        remove.type = "button";
        remove.addEventListener("click", () => { const nxt = ids.filter((_,i)=>i!==idx); update(nxt); render(); });
        actions.append(up, down, remove);
        row.append(actions);
        container.append(row);
      });
    } else {
      container.append(element("div", "inspector-media__empty", "No images selected"));
    }
    const add = element("button", "button button-primary", "Add image");
    add.type = "button";
    add.style.marginTop = "8px";
    add.addEventListener("click", () => {
      if (!window.openMediaPicker) return;
      window.openMediaPicker({ onSelect: (asset) => { const cur = Array.isArray(object[name]) ? object[name] : []; const nxt = [...cur, asset.id]; update(nxt); render(); } });
    });
    container.append(add);
  }
  render();
  return container;
}
function buildArray(node, object, name, schema, path) {
  if (node.block === "core/collection" && path === "settings.filters") {
    const container = element("div", "collection-filters");
    const values = object[name] || (object[name] = []);
    if (values.length === 0) {
      container.append(element("p", "editor-empty", "No filters. Add a filter to narrow the collection."));
    }
    values.forEach((value, index) => {
      const row = element("div", "collection-filter-row");
      const ct = scopedContentType(node);
      const fieldVal = value.field || "entry.title";
      const ft = fieldTypeFor(fieldVal, ct) || "text";
      const allowedOps = operatorsForType(ft);
      const fieldWrap = element("label", "inspector-field");
      fieldWrap.append(element("span", "", "Field"));
      const fieldSel = document.createElement("select");
      const fieldOpts = dynamicOptions("entry-fields", node);
      if (fieldVal && !fieldOpts.some(o => o.value === fieldVal)) {
        const miss = element("option", "", `${fieldVal} (unavailable)`);
        miss.value = fieldVal; miss.selected = true; fieldSel.append(miss);
      }
      fieldOpts.forEach(o => {
        const opt = element("option", "", o.label);
        opt.value = o.value; opt.selected = o.value === fieldVal; fieldSel.append(opt);
      });
      const opWrap = element("label", "inspector-field");
      opWrap.append(element("span", "", "Operator"));
      const opSel = document.createElement("select");
      const curOp = value.operator || allowedOps[0];
      allowedOps.forEach(op => {
        const opt = element("option", "", op);
        opt.value = op; opt.selected = op === curOp; opSel.append(opt);
      });
      if (!allowedOps.includes(curOp)) {
        const miss = element("option", "", `${curOp} (unavailable)`);
        miss.value = curOp; miss.selected = true; opSel.append(miss);
      }
      const valWrap = element("label", "inspector-field");
      valWrap.append(element("span", "", "Value"));
      const fieldDef = bootstrap.fieldCatalogs?.[ct]?.find(o => o.value === fieldVal);
      const fieldOptions = fieldDef?.options || null;
      const valControl = valueControlForType(ft, curOp, value.value, fieldOptions, (next) => { value.value = next; maybePushHistory(); if (window.__stratum_changed) window.__stratum_changed({ tree:false, inspector:false }); renderTreeFallback(); });
      const valContainer = element("div", "filter-value-control");
      valContainer.append(valControl);
      if (!needsValue(curOp)) valWrap.hidden = true;
      fieldSel.addEventListener("change", () => {
        value.field = fieldSel.value;
        const newType = fieldTypeFor(fieldSel.value, ct) || "text";
        const newOps = operatorsForType(newType);
        if (!newOps.includes(value.operator)) value.operator = newOps[0];
        if (!needsValue(value.operator)) value.value = "";
        maybePushHistory();
        if (window.__stratum_changed) window.__stratum_changed({ tree:false });
        renderInspector();
        renderTreeFallback();
      });
      opSel.addEventListener("change", () => {
        value.operator = opSel.value;
        if (!needsValue(value.operator)) value.value = "";
        maybePushHistory();
        if (window.__stratum_changed) window.__stratum_changed({ tree:false });
        renderInspector();
        renderTreeFallback();
      });
      fieldWrap.append(fieldSel);
      opWrap.append(opSel);
      valWrap.append(valContainer);
      row.append(fieldWrap, opWrap, valWrap);
      const rm = element("button", "button button-danger", "✕");
      rm.type = "button"; rm.title = "Remove filter";
      rm.addEventListener("click", () => { values.splice(index, 1); if (window.__stratum_changed) window.__stratum_changed({ tree: false }); renderInspector(); });
      row.append(rm);
      container.append(row);
    });
    const add = element("button", "button button-secondary", "+ Add filter");
    add.type = "button";
    add.addEventListener("click", () => {
      const ct = scopedContentType(node);
      const defaultField = (bootstrap.fieldCatalogs?.[ct]?.[0]?.value) || "entry.title";
      values.push({ field: defaultField, operator: operatorsForType(fieldTypeFor(defaultField, ct) || "text")[0], value: "" });
      if (window.__stratum_changed) window.__stratum_changed({ tree: false });
      renderInspector();
    });
    container.append(add);
    return container;
  }
  const container = element("div", "array-items");
  const values = object[name] || [];
  values.forEach((value, index) => {
    const row = element("div", "array-item");
    if (schema.items.type === "object") {
      const fields = element("div");
      Object.entries(schema.items.properties || {}).forEach(([childName, childSchema]) => {
        const metadata = node.block === "core/collection" && path === "settings.filters" && childName === "field" ? { label: "Field", control: "select", optionsSource: "entry-fields" } : {};
        fields.append(buildField(node, value, childName, childSchema, metadata, `${path}[${index}].${childName}`));
      });
      row.append(fields);
    } else {
      const factory = controlFactories[inferredControl(schema.items)] || controlFactories.text;
      row.append(factory(schema.items, value, (next) => { values[index] = next; if (window.__stratum_changed) window.__stratum_changed({ tree: false }); }, `${path}[${index}]`));
    }
    const rm = element("button", "button button-danger", "✕");
    rm.type="button"; rm.title="Remove item";
    rm.addEventListener("click", () => { values.splice(index, 1); if (window.__stratum_changed) window.__stratum_changed({ tree: false }); });
    row.append(rm);
    container.append(row);
  });
  const add = element("button", "button", "Add item");
  add.type = "button";
  add.addEventListener("click", () => { values.push(JSON.parse(JSON.stringify((() => { const sv = {}; if (schema.items.type==="object"){Object.entries(schema.items.properties||{}).forEach(([k,v])=>{sv[k]= v.default!==undefined? JSON.parse(JSON.stringify(v.default)) : (v.type==="boolean"?false: v.type==="array"?[]: "" )});} return sv;})()))); if (window.__stratum_changed) window.__stratum_changed({ tree: false }); });
  container.append(add);
  return container;
}
function buildRichTextControl(value, update) {
  const container = element("div", "richtext-control");
  const toolbar = element("div", "richtext-control__toolbar");
  const editor = element("div", "richtext-control__editor");
  editor.contentEditable = "true";
  editor.setAttribute("role", "textbox");
  editor.setAttribute("aria-multiline", "true");
  let model = value && value.version === 1 && Array.isArray(value.content) ? JSON.parse(JSON.stringify(value)) : { version: 1, content: [] };
  function clone(v) { return JSON.parse(JSON.stringify(v)); }
  function isSafeLink(href) {
    if (!href) return false;
    if (href.startsWith("#") || href.startsWith("/") ) return !href.startsWith("//");
    try {
      const u = new URL(href, "http://example.com");
      const scheme = u.protocol.replace(":","").toLowerCase();
      return ["http","https","mailto","tel"].includes(scheme) && !href.startsWith("//");
    } catch { return false; }
  }
  function sameMarks(a,b){
    if (!a && !b) return true;
    if (!a || !b) return false;
    if (a.length !== b.length) return false;
    const sa = [...a].sort((x,y)=> x.type===y.type ? (x.href||"").localeCompare(y.href||"") : x.type.localeCompare(y.type));
    const sb = [...b].sort((x,y)=> x.type===y.type ? (x.href||"").localeCompare(y.href||"") : x.type.localeCompare(y.type));
    for (let i=0;i<sa.length;i++) if (sa[i].type!==sb[i].type || sa[i].href!==sb[i].href) return false;
    return true;
  }
  function getSelectionOffsets() {
    const sel = window.getSelection();
    if (!sel || sel.rangeCount===0) return null;
    const range = sel.getRangeAt(0);
    if (!editor.contains(range.commonAncestorContainer) && range.commonAncestorContainer!==editor) return null;
    const preStart = document.createRange();
    preStart.selectNodeContents(editor);
    preStart.setEnd(range.startContainer, range.startOffset);
    const start = preStart.toString().length;
    const preEnd = document.createRange();
    preEnd.selectNodeContents(editor);
    preEnd.setEnd(range.endContainer, range.endOffset);
    const end = preEnd.toString().length;
    return { start, end };
  }
  function setSelectionOffsets(start, end) {
    const sel = window.getSelection();
    if (!sel) return;
    let charIndex = 0;
    let startNode, startOffset, endNode, endOffset;
    const walker = document.createTreeWalker(editor, NodeFilter.SHOW_TEXT, null);
    let node;
    while ((node = walker.nextNode())) {
      const nextIndex = charIndex + node.textContent.length;
      if (startNode === undefined && start >= charIndex && start <= nextIndex) {
        startNode = node;
        startOffset = start - charIndex;
      }
      if (endNode === undefined && end >= charIndex && end <= nextIndex) {
        endNode = node;
        endOffset = end - charIndex;
      }
      charIndex = nextIndex;
      if (startNode && endNode) break;
    }
    if (startNode && endNode) {
      const range = document.createRange();
      range.setStart(startNode, startOffset);
      range.setEnd(endNode, endOffset);
      sel.removeAllRanges();
      sel.addRange(range);
    }
  }
  function renderEditorFromModel() {
    editor.replaceChildren();
    for (const run of model.content) {
      let node = document.createTextNode(run.text || "");
      const marks = [...(run.marks||[])].sort((a,b)=> a.type===b.type ? (a.href||"").localeCompare(b.href||"") : a.type.localeCompare(b.type));
      for (const mark of marks) {
        const wrapper = document.createElement(mark.type === "bold" ? "strong" : mark.type === "italic" ? "em" : mark.type === "strike" ? "s" : mark.type === "code" ? "code" : "a");
        if (mark.type === "link") wrapper.setAttribute("href", mark.href || "");
        wrapper.append(node);
        node = wrapper;
      }
      editor.append(node);
    }
  }
  function normalizeModel() {
    const out = [];
    for (const run of model.content) {
      if (!run.text) continue;
      if (out.length>0 && sameMarks(out[out.length-1].marks, run.marks)) {
        out[out.length-1].text += run.text;
      } else {
        if (run.marks && run.marks.length) {
          const uniq = {};
          for (const m of run.marks) uniq[m.type + "\x00" + (m.href||"")] = m;
          let marks = Object.values(uniq);
          const links = marks.filter(m=>m.type==="link");
          if (links.length>1) marks = marks.filter(m=>m.type!=="link").concat([links[0]]);
          marks.sort((a,b)=> a.type===b.type ? (a.href||"").localeCompare(b.href||"") : a.type.localeCompare(b.type));
          run.marks = marks;
        }
        out.push(run);
      }
    }
    model.content = out;
  }
  function applyMark(markType, href) {
    const sel = getSelectionOffsets();
    if (!sel || sel.start===sel.end) return;
    if (markType==="link" && !isSafeLink(href)) return;
    const newContent = [];
    let pos=0;
    for (const run of model.content) {
      const runStart = pos;
      const runEnd = pos + run.text.length;
      pos = runEnd;
      if (runEnd <= sel.start || runStart >= sel.end) {
        newContent.push(clone(run));
        continue;
      }
      const beforeLen = Math.max(0, sel.start - runStart);
      const afterLen = Math.max(0, runEnd - sel.end);
      const selectedLen = run.text.length - beforeLen - afterLen;
      if (beforeLen>0) {
        newContent.push({ text: run.text.slice(0, beforeLen), ...(run.marks?{marks: clone(run.marks)}:{}) });
      }
      let selectedText = run.text.slice(beforeLen, beforeLen+selectedLen);
      let marks = run.marks ? clone(run.marks) : [];
      if (markType==="link") {
        marks = marks.filter(m=>m.type!=="link");
        marks.push({type:"link", href});
      } else {
        const has = marks.some(m=>m.type===markType);
        if (has) marks = marks.filter(m=>m.type!==markType);
        else marks.push({type:markType});
      }
      const uniq={};
      for (const m of marks) uniq[m.type + "\x00" + (m.href||"")] = m;
      marks = Object.values(uniq);
      marks.sort((a,b)=> a.type===b.type ? (a.href||"").localeCompare(b.href||"") : a.type.localeCompare(b.type));
      const linkMarks = marks.filter(m=>m.type==="link");
      if (linkMarks.length>1) marks = marks.filter(m=>m.type!=="link").concat([linkMarks[0]]);
      newContent.push({ text: selectedText, ...(marks.length?{marks}:{}) });
      if (afterLen>0) {
        newContent.push({ text: run.text.slice(run.text.length-afterLen), ...(run.marks?{marks: clone(run.marks)}:{}) });
      }
    }
    model.content = newContent;
    normalizeModel();
    if (model.content.length>200) model.content = model.content.slice(0,200);
    let total=0; for (const r of model.content) total+=r.text.length;
    if (total>10000) {
      let excess = total-10000;
      for (let i=model.content.length-1;i>=0 && excess>0;i--) {
        if (model.content[i].text.length > excess) {
          model.content[i].text = model.content[i].text.slice(0, -excess);
          excess=0;
        } else {
          excess -= model.content[i].text.length;
          model.content.splice(i,1);
        }
      }
    }
    renderEditorFromModel();
    setSelectionOffsets(sel.start, sel.end);
    saveModel();
    updateToolbarState();
  }
  function saveModel() { update(clone(model)); }
  function serializeFromDOM() {
    function serialize(node, marks, content) {
      if (node.nodeType === Node.TEXT_NODE) {
        if (node.textContent) content.push({ text: node.textContent, ...(marks.length ? { marks: clone(marks) } : {}) });
        return;
      }
      const tag = node.nodeName.toLowerCase();
      let next = marks;
      if (tag === "strong" || tag === "b") next = [...marks, { type: "bold" }];
      if (tag === "em" || tag === "i") next = [...marks, { type: "italic" }];
      if (tag === "s" || tag === "strike") next = [...marks, { type: "strike" }];
      if (tag === "code") next = [...marks, { type: "code" }];
      if (tag === "a" && node.getAttribute("href")) next = [...marks, { type: "link", href: node.getAttribute("href") }];
      node.childNodes.forEach((child) => serialize(child, next, content));
    }
    const content = [];
    editor.childNodes.forEach((child) => serialize(child, [], content));
    model = { version: 1, content };
    normalizeModel();
    saveModel();
  }
  function updateToolbarState() {
    const sel = getSelectionOffsets();
    const buttons = toolbar.querySelectorAll("button");
    buttons.forEach(b=>b.classList.remove("is-active"));
    if (!sel || sel.start===sel.end) return;
    let pos=0;
    let selectedMarks = null;
    let first = true;
    for (const run of model.content) {
      const runStart=pos;
      const runEnd=pos+run.text.length;
      pos=runEnd;
      if (runEnd <= sel.start || runStart >= sel.end) continue;
      const marks = run.marks||[];
      if (first) { selectedMarks = new Set(marks.map(m=>m.type + (m.href? ":"+m.href : ""))); first=false; }
      else {
        const cur = new Set(marks.map(m=>m.type + (m.href? ":"+m.href:"")));
        for (const key of [...selectedMarks]) if (!cur.has(key)) selectedMarks.delete(key);
      }
    }
    if (!selectedMarks) return;
    for (const btn of buttons) {
      const type = btn.dataset.mark;
      if (!type) continue;
      if ([...selectedMarks].some(k=>k===type || k.startsWith(type+":"))) btn.classList.add("is-active");
    }
  }
  function createToolbarButton(label, markType, href) {
    const btn = element("button", "button", label);
    btn.type = "button";
    btn.dataset.mark = markType;
    btn.addEventListener("mousedown", (e)=> e.preventDefault());
    btn.addEventListener("click", () => {
      editor.focus();
      if (markType==="link") {
        const sel = getSelectionOffsets();
        if (!sel || sel.start===sel.end) return;
        const url = window.prompt("Link URL");
        if (!url) return;
        if (!isSafeLink(url)) {
          const msg = "Invalid URL. Use /path, #anchor, https://, http://, mailto:, tel:";
          if (window.stratumToast) window.stratumToast('error', msg);
          else window.alert(msg);
          return;
        }
        applyMark("link", url);
      } else {
        applyMark(markType);
      }
    });
    return btn;
  }
  toolbar.append(createToolbarButton("B","bold"));
  toolbar.append(createToolbarButton("I","italic"));
  toolbar.append(createToolbarButton("S","strike"));
  toolbar.append(createToolbarButton("Code","code"));
  toolbar.append(createToolbarButton("Link","link"));
  renderEditorFromModel();
  updateToolbarState();
  editor.addEventListener("input", () => { serializeFromDOM(); updateToolbarState(); });
  editor.addEventListener("keyup", updateToolbarState);
  editor.addEventListener("mouseup", updateToolbarState);
  document.addEventListener("selectionchange", () => { if (document.activeElement===editor || editor.contains(document.activeElement)) updateToolbarState(); });
  editor.addEventListener("paste", (e) => {
    e.preventDefault();
    const text = (e.clipboardData || window.clipboardData).getData("text/plain") || "";
    const normalized = text.replace(/\r\n/g, " ").replace(/\n/g, " ").replace(/\r/g, " ");
    const sel = getSelectionOffsets();
    if (!sel) return;
    const before = model.content.slice();
    let pos=0;
    const newContent=[];
    let inserted=false;
    for (const run of before) {
      const runStart=pos;
      const runEnd=pos+run.text.length;
      pos=runEnd;
      if (!inserted && sel.start>=runStart && sel.start<=runEnd) {
        const beforeLen = sel.start - runStart;
        const afterLen = runEnd - sel.end;
        if (beforeLen>0) newContent.push({text: run.text.slice(0, beforeLen), ...(run.marks?{marks: clone(run.marks)}:{})});
        if (normalized) newContent.push({text: normalized});
        if (afterLen>0) newContent.push({text: run.text.slice(run.text.length-afterLen), ...(run.marks?{marks: clone(run.marks)}:{})});
        inserted=true;
      } else if (inserted) {
        if (runEnd <= sel.start || runStart >= sel.end) newContent.push(clone(run));
      } else {
        if (runEnd <= sel.start) newContent.push(clone(run));
        else if (runStart >= sel.end) {
          if (!inserted) { if (normalized) newContent.push({text: normalized}); inserted=true; }
          newContent.push(clone(run));
        }
      }
    }
    if (!inserted && normalized) newContent.push({text: normalized});
    model.content = newContent;
    normalizeModel();
    renderEditorFromModel();
    const newPos = sel.start + normalized.length;
    setSelectionOffsets(newPos, newPos);
    saveModel();
  });
  editor.addEventListener("keydown", (e) => {
    const mod = e.metaKey || e.ctrlKey;
    if (mod && e.key.toLowerCase()==="b") { e.preventDefault(); applyMark("bold"); }
    if (mod && e.key.toLowerCase()==="i") { e.preventDefault(); applyMark("italic"); }
    if (mod && e.key.toLowerCase()==="k") { e.preventDefault(); const sel=getSelectionOffsets(); if (!sel||sel.start===sel.end) return; const url=window.prompt("Link URL"); if (!url) return; if (!isSafeLink(url)) { window.alert("Invalid URL"); return; } applyMark("link", url); }
    if (e.key==="Enter") { e.preventDefault(); }
  });
  container.append(toolbar, editor);
  return container;
}
