import { bootstrap, displayNameForBlock, findDocumentNode, inspectorFactsFor, state } from "./state.js";

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

function appendFact(root, label, value) {
  if (value === undefined || value === null || value === "") return;
  const row = createElement("div", "editor-v2-summary__row");
  row.append(createElement("dt", "editor-v2-summary__label", label));
  row.append(createElement("dd", "editor-v2-summary__value", String(value)));
  root.append(row);
}

export function inspectorTitle(selection) {
  const name = displayNameForBlock(selection?.block || "");
  if (selection && !selection.editable) {
    return `${name} \u00b7 ${selection.ownerType === "site-part" ? "Site Part" : "Template"}`;
  }
  return name;
}

export function renderInspectorBody(root, selection) {
  root.replaceChildren();
  if (!selection) {
    root.append(createElement("p", "editor-v2-panel__empty", "Select a block to inspect it."));
    return;
  }

  if (!selection.editable) {
    const isSitePart = selection.ownerType === "site-part";
    const badge = createElement("div", "editor-v2-readonly-badge", "Read-only");
    root.append(badge);
    const owner = !technicalIdentifier(selection.ownerLabel)
      ? selection.ownerLabel
      : isSitePart ? "Reusable site part" : "Main Layout";
    const desc = owner ? `Belongs to ${owner}` : isSitePart ? "Belongs to a reusable site part." : "Belongs to the active layout template.";
    root.append(createElement("p", "editor-v2-panel__description", desc));
    return;
  }

  const node = findDocumentNode(selection.nodeId);
  if (!node) {
    root.append(createElement("p", "editor-v2-panel__empty", "Select a block to inspect it."));
    return;
  }
  const facts = inspectorFactsFor(node);
  if (facts.length === 0) {
    return;
  }
  const dl = createElement("dl", "editor-v2-summary");
  for (const [label, value] of facts) appendFact(dl, label, value);
  root.append(dl);
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
