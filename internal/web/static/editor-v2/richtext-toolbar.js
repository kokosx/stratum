// richtext-toolbar.js — stateless floating rich-text controls

import { isSafeHref } from "./richtext-editor.js";

const TOOLBAR_CSS = `
.richtext-toolbar{
  position: fixed;
  display: none;
  align-items: center;
  gap: 2px;
  padding: 4px 6px;
  height: 32px;
  background: #1e293b;
  color: #fff;
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0,0,0,0.2), 0 2px 8px rgba(0,0,0,0.15);
  z-index: 2147483647;
  font-family: system-ui, -apple-system, sans-serif;
  pointer-events: auto;
}
.richtext-toolbar.is-visible{ display: inline-flex; }
.richtext-toolbar__btn{
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: #e2e8f0;
  font: 700 13px/1 system-ui, sans-serif;
  cursor: pointer;
  pointer-events: auto;
}
.richtext-toolbar__btn:hover{ background: rgba(255,255,255,0.15); color: #fff; }
.richtext-toolbar__btn.is-active{ background: #334155; color: #fff; }
.richtext-toolbar__btn[aria-pressed="true"]{ background: #475569; color: #fff; }
.richtext-toolbar__sep{ width:1px; height:18px; background: #334155; margin: 0 2px; }
.richtext-link-popover{
  position: fixed;
  display: none;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  min-width: 240px;
  background: #fff;
  color: #0f172a;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  box-shadow: 0 12px 32px rgba(0,0,0,0.15);
  z-index: 2147483647;
  font-family: system-ui, -apple-system, sans-serif;
  pointer-events: auto;
}
.richtext-link-popover.is-visible{ display: flex; }
.richtext-link-popover__title{ font: 600 12px/1 system-ui; color:#475569; }
.richtext-link-popover__input{
  width: 100%;
  box-sizing: border-box;
  height: 32px;
  padding: 0 8px;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  font: 400 13px/1 system-ui;
}
.richtext-link-popover__input:focus{ outline: 2px solid #2563eb; outline-offset: 1px; border-color:#2563eb; }
.richtext-link-popover__actions{ display:flex; gap:8px; justify-content:flex-end; }
.richtext-link-popover__btn{
  height: 30px;
  padding: 0 12px;
  border-radius: 6px;
  font: 600 12px/1 system-ui;
  cursor: pointer;
}
.richtext-link-popover__btn--remove,
.richtext-link-popover__btn--cancel{ background:#fff; border:1px solid #e2e8f0; color:#475569; }
.richtext-link-popover__btn--apply{ background:#2563eb; border:1px solid #2563eb; color:#fff; }
.richtext-link-popover__error{ color:#dc2626; font:400 11px/1.2 system-ui; display:none; }
.richtext-link-popover__error.is-visible{ display:block; }
`;

let styleEl = null;
let toolbarEl = null;
let popoverEl = null;
let inputEl = null;
let errorEl = null;
let callbacks = emptyCallbacks();

function emptyCallbacks() {
  return {
    toggleMark: null,
    openLink: null,
    applyLink: null,
    removeLink: null,
    closeLink: null,
  };
}

function scheduleFrame(canvas, callback) {
  const win = canvas?.doc?.defaultView;
  const frame = win?.requestAnimationFrame || globalThis.requestAnimationFrame;
  if (typeof frame === "function") frame.call(win || globalThis, callback);
  else callback();
}

function emitMark(mark) {
  if (mark === "link") callbacks.openLink?.();
  else callbacks.toggleMark?.(mark);
}

function bindToolbarButton(button, mark) {
  button.addEventListener("pointerdown", (event) => {
    event.preventDefault();
    event.stopPropagation();
    emitMark(mark);
  });
  button.addEventListener("click", (event) => {
    event.preventDefault();
    event.stopPropagation();
  });
  button.addEventListener("keydown", (event) => {
    if (event.key !== "Enter" && event.key !== " ") return;
    event.preventDefault();
    event.stopPropagation();
    emitMark(mark);
  });
}

function stopOwnedEvent(event) {
  event.stopPropagation();
}

function ensureElements(canvas) {
  if (!canvas?.overlay?.shadow || !canvas.doc) return false;
  const shadow = canvas.overlay.shadow;
  if (toolbarEl?.parentNode === shadow && popoverEl?.parentNode === shadow) return true;
  if (toolbarEl && toolbarEl.parentNode !== shadow) {
    try { toolbarEl.remove(); } catch (_) { try { toolbarEl.parentNode?.removeChild(toolbarEl); } catch (_) {} }
    toolbarEl = null;
  }
  if (popoverEl && popoverEl.parentNode !== shadow) {
    try { popoverEl.remove(); } catch (_) { try { popoverEl.parentNode?.removeChild(popoverEl); } catch (_) {} }
    popoverEl = null;
  }
  if (styleEl && styleEl.parentNode !== shadow) {
    try { styleEl.remove(); } catch (_) { try { styleEl.parentNode?.removeChild(styleEl); } catch (_) {} }
    styleEl = null;
  }
  if (!styleEl) {
    styleEl = canvas.doc.createElement("style");
    styleEl.setAttribute("data-richtext-toolbar", "true");
    styleEl.textContent = TOOLBAR_CSS;
    shadow.appendChild(styleEl);
  }

  toolbarEl = canvas.doc.createElement("div");
  toolbarEl.className = "richtext-toolbar";
  toolbarEl.setAttribute("role", "toolbar");
  toolbarEl.setAttribute("aria-label", "Text formatting");
  toolbarEl.setAttribute("data-stratum-editor-ui", "true");
  const buttons = [
    { mark: "bold", label: "Bold", text: "B", title: "Bold (⌘B)" },
    { mark: "italic", label: "Italic", text: "I", title: "Italic (⌘I)", style: "font-style:italic" },
    { separator: true },
    { mark: "link", label: "Link", text: "🔗", title: "Link (⌘K)" },
    { separator: true },
    { mark: "strike", label: "Strikethrough", text: "S", title: "Strikethrough" },
    { mark: "code", label: "Inline code", text: "</>", title: "Code" },
  ];
  for (const definition of buttons) {
    if (definition.separator) {
      const separator = canvas.doc.createElement("div");
      separator.className = "richtext-toolbar__sep";
      separator.setAttribute("aria-hidden", "true");
      toolbarEl.appendChild(separator);
      continue;
    }
    const button = canvas.doc.createElement("button");
    button.type = "button";
    button.className = "richtext-toolbar__btn";
    button.setAttribute("data-mark", definition.mark);
    button.setAttribute("aria-label", definition.label);
    button.setAttribute("aria-pressed", "false");
    button.setAttribute("title", definition.title);
    button.textContent = definition.text;
    if (definition.style) button.style.cssText = definition.style;
    bindToolbarButton(button, definition.mark);
    toolbarEl.appendChild(button);
  }
  toolbarEl.addEventListener("pointerdown", stopOwnedEvent);
  toolbarEl.addEventListener("click", stopOwnedEvent);
  shadow.appendChild(toolbarEl);

  popoverEl = canvas.doc.createElement("div");
  popoverEl.className = "richtext-link-popover";
  popoverEl.setAttribute("data-stratum-editor-ui", "true");
  popoverEl.innerHTML = `
    <div class="richtext-link-popover__title">Link</div>
    <input class="richtext-link-popover__input" type="text" placeholder="/path or https://..." aria-label="Link URL">
    <div class="richtext-link-popover__error"></div>
    <div class="richtext-link-popover__actions">
      <button class="richtext-link-popover__btn richtext-link-popover__btn--remove" type="button">Remove</button>
      <button class="richtext-link-popover__btn richtext-link-popover__btn--cancel" type="button">Cancel</button>
      <button class="richtext-link-popover__btn richtext-link-popover__btn--apply" type="button">Apply</button>
    </div>
  `;
  popoverEl.addEventListener("pointerdown", stopOwnedEvent);
  popoverEl.addEventListener("click", stopOwnedEvent);
  shadow.appendChild(popoverEl);

  inputEl = popoverEl.querySelector("input");
  errorEl = popoverEl.querySelector(".richtext-link-popover__error");
  const removeButton = popoverEl.querySelector(".richtext-link-popover__btn--remove");
  const cancelButton = popoverEl.querySelector(".richtext-link-popover__btn--cancel");
  const applyButton = popoverEl.querySelector(".richtext-link-popover__btn--apply");

  removeButton?.addEventListener("click", (event) => {
    event.preventDefault();
    event.stopPropagation();
    callbacks.removeLink?.();
  });
  cancelButton?.addEventListener("click", (event) => {
    event.preventDefault();
    event.stopPropagation();
    callbacks.closeLink?.();
  });
  applyButton?.addEventListener("click", (event) => {
    event.preventDefault();
    event.stopPropagation();
    applyInputLink();
  });
  inputEl?.addEventListener("keydown", (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      event.stopPropagation();
      applyInputLink();
    } else if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      callbacks.closeLink?.();
    }
  });
  return true;
}

function applyInputLink() {
  const href = inputEl?.value?.trim() || "";
  if (!href) {
    showError("Enter a URL");
    return;
  }
  if (!isSafeHref(href)) {
    showError("Invalid URL. Use /path, #anchor, https://, mailto:, tel:");
    return;
  }
  hideError();
  callbacks.applyLink?.(href);
}

function showError(message) {
  if (!errorEl) return;
  errorEl.textContent = message;
  errorEl.classList.add("is-visible");
}

function hideError() {
  if (!errorEl) return;
  errorEl.textContent = "";
  errorEl.classList.remove("is-visible");
}

export function isPopoverVisible() {
  return !!popoverEl?.classList.contains("is-visible");
}

export function isToolbarVisible() {
  return !!toolbarEl?.classList.contains("is-visible");
}

export function showToolbar(canvas, rect, activeMarks) {
  if (!ensureElements(canvas) || !rect) {
    hideToolbar();
    return;
  }
  const marks = new Set(activeMarks || []);
  for (const button of toolbarEl.querySelectorAll("[data-mark]")) {
    const active = marks.has(button.getAttribute("data-mark"));
    button.classList.toggle("is-active", active);
    button.setAttribute("aria-pressed", String(active));
  }
  toolbarEl.classList.add("is-visible");
  positionToolbar(rect, canvas);
}

export function updateMarks(activeMarks) {
  if (!toolbarEl) return;
  const marks = new Set(activeMarks || []);
  for (const button of toolbarEl.querySelectorAll("[data-mark]")) {
    const active = marks.has(button.getAttribute("data-mark"));
    button.classList.toggle("is-active", active);
    button.setAttribute("aria-pressed", String(active));
  }
  // Do NOT reposition — toolbar stays at frozen anchor
}

export function hideToolbar() {
  toolbarEl?.classList.remove("is-visible");
}

export function showPopover(canvas, rect, href) {
  if (!ensureElements(canvas)) return false;
  if (inputEl) inputEl.value = href || "";
  hideError();
  hideToolbar();
  popoverEl.classList.add("is-visible");
  scheduleFrame(canvas, () => {
    positionPopover(rect, canvas);
    try {
      inputEl?.focus({ preventScroll: true });
      inputEl?.select();
    } catch (_) {
      try { inputEl?.focus(); } catch (_) {}
    }
  });
  return true;
}

export function hidePopover() {
  popoverEl?.classList.remove("is-visible");
  hideError();
}

function positionToolbar(rect, canvas) {
  if (!toolbarEl || !rect) return;
  const viewportWidth = canvas.doc.documentElement.clientWidth || canvas.doc.defaultView?.innerWidth || 1024;
  const viewportHeight = canvas.doc.documentElement.clientHeight || canvas.doc.defaultView?.innerHeight || 768;
  let width = 220;
  let height = 36;
  try {
    const measured = toolbarEl.getBoundingClientRect();
    if (measured.width > 0) { width = measured.width; height = measured.height; }
  } catch (_) {}
  let left = Math.round(rect.left + rect.width / 2 - width / 2);
  let top = Math.round(rect.top - height - 8);
  if (top < 4) top = Math.round(rect.bottom + 8);
  left = Math.max(4, Math.min(left, viewportWidth - width - 4));
  top = Math.max(4, Math.min(top, viewportHeight - height - 4));
  toolbarEl.style.left = `${left}px`;
  toolbarEl.style.top = `${top}px`;
}

function positionPopover(rect, canvas) {
  if (!popoverEl) return;
  const anchor = rect || { left: 8, top: 8, right: 8, bottom: 8, width: 0, height: 0 };
  const viewportWidth = canvas.doc.documentElement.clientWidth || canvas.doc.defaultView?.innerWidth || 1024;
  const viewportHeight = canvas.doc.documentElement.clientHeight || canvas.doc.defaultView?.innerHeight || 768;
  let width = 260;
  let height = 110;
  try {
    const measured = popoverEl.getBoundingClientRect();
    if (measured.width > 0) { width = measured.width; height = measured.height; }
  } catch (_) {}
  let left = Math.round(anchor.left + anchor.width / 2 - width / 2);
  let top = Math.round(anchor.bottom + 8);
  if (top + height > viewportHeight - 4) top = Math.round(anchor.top - height - 8);
  left = Math.max(4, Math.min(left, viewportWidth - width - 4));
  top = Math.max(4, Math.min(top, viewportHeight - height - 4));
  popoverEl.style.left = `${left}px`;
  popoverEl.style.top = `${top}px`;
}

export function setToolbarCallbacks(nextCallbacks) {
  callbacks = {
    ...emptyCallbacks(),
    ...(nextCallbacks && typeof nextCallbacks === "object" ? nextCallbacks : {}),
  };
}

export function destroyToolbar() {
  try { toolbarEl?.remove(); } catch (_) { try { toolbarEl?.parentNode?.removeChild(toolbarEl); } catch (_) {} }
  try { popoverEl?.remove(); } catch (_) { try { popoverEl?.parentNode?.removeChild(popoverEl); } catch (_) {} }
  try { styleEl?.remove(); } catch (_) { try { styleEl?.parentNode?.removeChild(styleEl); } catch (_) {} }
  styleEl = null;
  toolbarEl = null;
  popoverEl = null;
  inputEl = null;
  errorEl = null;
  callbacks = emptyCallbacks();
}
