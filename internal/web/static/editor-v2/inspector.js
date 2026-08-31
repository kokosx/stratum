import { bootstrap, definitionForBlock, displayNameForBlock, findDocumentNode, state } from "./state.js";

function createElement(tag, className, text) {
  const element = document.createElement(tag);
  if (className) element.className = className;
  if (text !== undefined) element.textContent = text;
  return element;
}

function technicalIdentifier(value) {
  if (!value || typeof value !== "string") return true;
  const text = value.trim();
  return /^(blk|entry|site|page)[-_]/i.test(text)
    || /^[0-9a-f]{8}-[0-9a-f]{4}-/i.test(text)
    || (/^[A-Za-z0-9_-]{16,}$/.test(text) && /[0-9_-]/.test(text));
}

function textValue(value) {
  if (typeof value === "string") return value.trim();
  if (value && Array.isArray(value.content)) {
    return value.content.map((run) => typeof run?.text === "string" ? run.text : "").join("").trim();
  }
  return "";
}

function appendFact(root, label, value) {
  if (value === undefined || value === null || value === "") return;
  const row = createElement("div", "editor-v2-summary__row");
  row.append(createElement("dt", "editor-v2-summary__label", label));
  row.append(createElement("dd", "editor-v2-summary__value", String(value)));
  root.append(row);
}

function nodeFacts(node, definition) {
  const facts = [];
  if (!node) return facts;
  const props = node.props || {};
  const preferred = ["text", "title", "label", "url", "caption", "citation"];
  for (const key of preferred) {
    const value = textValue(props[key]);
    if (!value) continue;
    const label = definition?.schema?.editor?.fields?.[`props.${key}`]?.label
      || key.charAt(0).toUpperCase() + key.slice(1);
    facts.push([label, value.length > 140 ? value.slice(0, 137) + "…" : value]);
    if (facts.length >= 2) break;
  }
  const childCount = (node.children || []).length;
  if (childCount > 0) facts.push(["Content", `${childCount} child block${childCount === 1 ? "" : "s"}`]);
  return facts;
}

export function inspectorTitle(selection) {
  const name = displayNameForBlock(selection?.block || "");
  if (selection && !selection.editable) {
    return `${name} · ${selection.ownerType === "site-part" ? "Site Part" : "Template"}`;
  }
  return name;
}

export function renderInspectorBody(root, selection) {
  root.replaceChildren();
  if (!selection) {
    root.append(createElement("p", "editor-v2-panel__empty", "Select a block to inspect it."));
    return;
  }

  const definition = definitionForBlock(selection.block, selection.version);
  if (!selection.editable) {
    const isSitePart = selection.ownerType === "site-part";
    const badge = createElement("div", "editor-v2-readonly-badge", "Read-only");
    root.append(badge);
    root.append(createElement(
      "p",
      "editor-v2-panel__description",
      isSitePart ? "This block belongs to a reusable site part." : "This block belongs to the active layout template.",
    ));
    const facts = createElement("dl", "editor-v2-summary");
    appendFact(facts, "Block type", displayNameForBlock(selection.block));
    if (selection.block && selection.version) appendFact(facts, "Definition", `${selection.block}@${selection.version}`);
    const owner = !technicalIdentifier(selection.ownerLabel)
      ? selection.ownerLabel
      : isSitePart ? "Reusable site part" : "Active layout template";
    appendFact(facts, "Owner", owner);
    root.append(facts);
    return;
  }

  const node = findDocumentNode(selection.nodeId);
  const facts = createElement("dl", "editor-v2-summary");
  for (const [label, value] of nodeFacts(node, definition)) appendFact(facts, label, value);
  appendFact(facts, "Block type", displayNameForBlock(selection.block));
  if (selection.block && selection.version) appendFact(facts, "Definition", `${selection.block}@${selection.version}`);
  root.append(facts);

  const propsCount = Object.keys(definition?.schema?.props?.properties || {}).length;
  const settingsCount = Object.keys(definition?.schema?.settings?.properties || {}).length;
  const schema = createElement("div", "editor-v2-schema-summary");
  schema.append(createElement("div", "editor-v2-schema-summary__title", "Configuration"));
  schema.append(createElement("p", "editor-v2-panel__description", "Block settings will appear here in a later milestone."));
  const counts = createElement("div", "editor-v2-schema-summary__counts");
  counts.append(createElement("span", "", `${propsCount} ${propsCount === 1 ? "property" : "properties"}`));
  counts.append(createElement("span", "", `${settingsCount} ${settingsCount === 1 ? "setting" : "settings"}`));
  schema.append(counts);
  root.append(schema);
}

function contentTypeLabel() {
  const value = state.contentTypeId || state.resource?.contentTypeId || state.resource?.kind || "";
  const option = (bootstrap.contentTypes || []).find((item) => item?.value === value);
  if (option?.label) return option.label;
  return value ? value.charAt(0).toUpperCase() + value.slice(1) : "Entry";
}

export function renderDocumentBody(root) {
  root.replaceChildren();

  const generalTitle = createElement("h3", "editor-v2-panel__section-title", "General");
  root.append(generalTitle);
  const general = createElement("dl", "editor-v2-summary");
  appendFact(general, "Title", state.title || state.resource?.label || "Untitled");
  appendFact(general, "Status", state.status || "Draft");
  appendFact(general, "Content type", contentTypeLabel());
  root.append(general);

  const locationTitle = createElement("h3", "editor-v2-panel__section-title", "Location");
  root.append(locationTitle);
  const location = createElement("dl", "editor-v2-summary");
  appendFact(location, "Slug", state.slug ? `/${state.slug}` : "/");
  appendFact(location, "URL", state.resourceUrl || "Not published");
  appendFact(location, "Template", state.templateName || "Default");
  root.append(location);
}
