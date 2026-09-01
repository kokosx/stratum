// richtext-toolbar.js — floating selection toolbar + link popover for rich Text

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
.richtext-link-popover__input:focus{ outline: 2px solid #2563eb; outline-offset: 1px; border-color: #2563eb; }
.richtext-link-popover__actions{ display:flex; gap:8px; justify-content: space-between; }
.richtext-link-popover__btn{
  height: 30px;
  padding: 0 12px;
  border-radius: 6px;
  font: 600 12px/1 system-ui;
  cursor: pointer;
}
.richtext-link-popover__btn--remove{ background:#fff; border:1px solid #e2e8f0; color:#475569; }
.richtext-link-popover__btn--apply{ background:#2563eb; border:1px solid #2563eb; color:#fff; }
.richtext-link-popover__error{ color:#dc2626; font: 400 11px/1.2 system-ui; display:none; }
.richtext-link-popover__error.is-visible{ display:block; }
`;

let toolbarEl = null;
let popoverEl = null;
let inputEl = null;
let errorEl = null;
let activeFieldEl = null;
let activeCanvas = null;
let savedOffsets = null; // for link popover {start,end}
let savedToolbarOffsets = null; // for toolbar actions {start,end}
let toolbarInteraction = false;
let pointerActionHandled = false;
let callbacks = {
  toggleMark: null,
  openLink: null,
  applyLink: null,
  removeLink: null,
  closeLink: null,
};

function beginToolbarInteraction() {
  toolbarInteraction = true;
}

function endToolbarInteraction() {
  toolbarInteraction = false;
}

function performToolbarAction(mark) {
  if (!savedToolbarOffsets) return;
  const offsets = { start: savedToolbarOffsets.start, end: savedToolbarOffsets.end };
  if (mark === "link") {
    if (callbacks.openLink) callbacks.openLink();
  } else {
    if (callbacks.toggleMark) callbacks.toggleMark(mark, offsets);
  }
}

function ensureElements(canvas) {
  if (!canvas || !canvas.overlay || !canvas.overlay.shadow) return false;
  const shadow = canvas.overlay.shadow;
  if (toolbarEl && toolbarEl.parentNode === shadow) return true;
  try {
    const style = canvas.doc.createElement("style");
    style.setAttribute("data-richtext-toolbar", "true");
    style.textContent = TOOLBAR_CSS;
    shadow.appendChild(style);
  } catch (_) {}
  toolbarEl = canvas.doc.createElement("div");
  toolbarEl.className = "richtext-toolbar";
  toolbarEl.setAttribute("role", "toolbar");
  toolbarEl.setAttribute("data-stratum-editor-ui", "true");
  const btns = [
    { mark: "bold", label: "Bold", text: "B", title: "Bold (⌘B)" },
    { mark: "italic", label: "Italic", text: "I", title: "Italic (⌘I)", style: "font-style:italic" },
    { sep: true },
    { mark: "link", label: "Link", text: "🔗", title: "Link (⌘K)" },
    { sep: true },
    { mark: "strike", label: "Strikethrough", text: "S", title: "Strikethrough" },
    { mark: "code", label: "Inline code", text: "</>", title: "Code" },
  ];
  for (const b of btns) {
    if (b.sep) {
      const sep = canvas.doc.createElement("div");
      sep.className = "richtext-toolbar__sep";
      toolbarEl.appendChild(sep);
      continue;
    }
    const btn = canvas.doc.createElement("button");
    btn.type = "button";
    btn.className = "richtext-toolbar__btn";
    btn.setAttribute("data-mark", b.mark);
    btn.setAttribute("aria-label", b.label);
    btn.setAttribute("title", b.title);
    btn.textContent = b.text;
    if (b.style) btn.style.cssText += b.style;
    // Pointerdown is the primary action: happens before blur/selectionchange
    btn.addEventListener("pointerdown", (e) => {
      beginToolbarInteraction();
      pointerActionHandled = true;
      e.preventDefault();
      e.stopPropagation();
      performToolbarAction(b.mark);
      // keep toolbarInteraction true until click guard clears it
    });
    btn.addEventListener("mousedown", (e) => {
      // Fallback for devices without pointer events: same as pointerdown but only if not already handled
      if (pointerActionHandled) {
        e.preventDefault();
        e.stopPropagation();
        return;
      }
      beginToolbarInteraction();
      pointerActionHandled = true;
      e.preventDefault();
      e.stopPropagation();
      performToolbarAction(b.mark);
    });
    btn.addEventListener("click", (e) => {
      e.preventDefault();
      e.stopPropagation();
      if (pointerActionHandled) {
        // pointerdown already performed the action — consume click without double toggle
        pointerActionHandled = false;
        // End interaction on next microtask to allow blur/selectionchange guards to see true during this dispatch
        queueMicrotask(() => {
          // keep toolbar visible: do not hide, just end interaction state
          endToolbarInteraction();
        });
        return;
      }
      // Keyboard activation (Space/Enter) has no pointerdown — perform once here
      performToolbarAction(b.mark);
      queueMicrotask(() => endToolbarInteraction());
    });
    // Fallback if click never fires (drag away, pointercancel)
    btn.addEventListener("pointerup", () => {
      queueMicrotask(() => {
        if (pointerActionHandled) {
          pointerActionHandled = false;
          endToolbarInteraction();
        }
      });
    });
    btn.addEventListener("pointercancel", () => {
      queueMicrotask(() => {
        if (pointerActionHandled) {
          pointerActionHandled = false;
          endToolbarInteraction();
        }
      });
    });
    btn.addEventListener("mouseup", () => {
      queueMicrotask(() => {
        if (pointerActionHandled) {
          pointerActionHandled = false;
          endToolbarInteraction();
        }
      });
    });
    toolbarEl.appendChild(btn);
  }
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
      <button class="richtext-link-popover__btn richtext-link-popover__btn--apply" type="button">Apply</button>
    </div>
  `;
  shadow.appendChild(popoverEl);
  inputEl = popoverEl.querySelector("input");
  errorEl = popoverEl.querySelector(".richtext-link-popover__error");
  const removeBtn = popoverEl.querySelector(".richtext-link-popover__btn--remove");
  const applyBtn = popoverEl.querySelector(".richtext-link-popover__btn--apply");
  [removeBtn, applyBtn, inputEl].forEach(el => {
    if (!el) return;
    el.addEventListener("mousedown", (e) => { beginToolbarInteraction(); e.preventDefault(); e.stopPropagation(); });
    el.addEventListener("pointerdown", (e) => { beginToolbarInteraction(); e.preventDefault(); e.stopPropagation(); });
  });
  removeBtn.addEventListener("click", (e) => {
    e.preventDefault(); e.stopPropagation();
    if (callbacks.removeLink) callbacks.removeLink();
    hidePopover();
  });
  applyBtn.addEventListener("click", (e) => {
    e.preventDefault(); e.stopPropagation();
    const val = inputEl.value.trim();
    if (!val) {
      showError("Enter a URL");
      return;
    }
    if (!isSafeHref(val)) {
      showError("Invalid URL. Use /path, #anchor, https://, mailto:, tel:");
      return;
    }
    hideError();
    if (callbacks.applyLink) callbacks.applyLink(val);
    hidePopover();
  });
  inputEl.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      const val = inputEl.value.trim();
      if (!val) { showError("Enter a URL"); return; }
      if (!isSafeHref(val)) { showError("Invalid URL"); return; }
      hideError();
      if (callbacks.applyLink) callbacks.applyLink(val);
      hidePopover();
    }
    if (e.key === "Escape") {
      e.preventDefault(); e.stopPropagation();
      hidePopover();
      if (callbacks.closeLink) callbacks.closeLink();
    }
  });
  // Keep interaction true while popover focused; end only when popover hidden
  inputEl.addEventListener("focus", () => { beginToolbarInteraction(); });
  inputEl.addEventListener("blur", () => {
    // If popover is still visible, keep interaction true; otherwise microtask ends it.
    // Do not use 200ms timer — explicit lifecycle.
    if (isPopoverVisible()) return;
    queueMicrotask(() => {
      if (!isPopoverVisible()) endToolbarInteraction();
    });
  });
  popoverEl.addEventListener("mousedown", (e) => { beginToolbarInteraction(); e.stopPropagation(); });
  popoverEl.addEventListener("pointerdown", (e) => { beginToolbarInteraction(); e.stopPropagation(); });
  popoverEl.addEventListener("click", (e) => e.stopPropagation());
  // Also track toolbar itself
  toolbarEl.addEventListener("mousedown", () => { beginToolbarInteraction(); });
  toolbarEl.addEventListener("pointerdown", () => { beginToolbarInteraction(); });
  return true;
}

function showError(msg) {
  if (!errorEl) return;
  errorEl.textContent = msg;
  errorEl.classList.add("is-visible");
}
function hideError() {
  if (!errorEl) return;
  errorEl.textContent = "";
  errorEl.classList.remove("is-visible");
}

export function isPopoverVisible() {
  return !!(popoverEl && popoverEl.classList.contains("is-visible"));
}

export function isToolbarVisible() {
  return !!(toolbarEl && toolbarEl.classList.contains("is-visible"));
}

export function isToolbarInteraction() {
  return toolbarInteraction;
}

export function showToolbar(canvas, fieldEl, rect, activeMarks, offsets) {
  if (!ensureElements(canvas)) return;
  activeFieldEl = fieldEl;
  activeCanvas = canvas;
  if (offsets && typeof offsets.start === "number" && typeof offsets.end === "number") {
    savedToolbarOffsets = { start: offsets.start, end: offsets.end };
  }
  if (!rect) {
    hideToolbar();
    return;
  }
  const marks = new Set(activeMarks || []);
  for (const btn of toolbarEl.querySelectorAll("[data-mark]")) {
    const mark = btn.getAttribute("data-mark");
    const isActive = marks.has(mark);
    btn.classList.toggle("is-active", isActive);
    btn.setAttribute("aria-pressed", String(isActive));
  }
  toolbarEl.classList.add("is-visible");
  positionToolbar(rect, canvas);
}

export function hideToolbar() {
  if (toolbarEl) toolbarEl.classList.remove("is-visible");
  // Do not clear savedToolbarOffsets immediately; keep for click handling if toolbarInteraction
  if (!toolbarInteraction) savedToolbarOffsets = null;
}

export function showPopover(canvas, fieldEl, href, offsets) {
  if (!ensureElements(canvas)) return;
  activeFieldEl = fieldEl;
  activeCanvas = canvas;
  savedOffsets = offsets ? { ...offsets } : null;
  savedToolbarOffsets = offsets ? { ...offsets } : null;
  toolbarInteraction = true;
  pointerActionHandled = false;
  if (inputEl) {
    inputEl.value = href || "";
    hideError();
  }
  let rect = null;
  try {
    const sel = fieldEl.ownerDocument.getSelection();
    if (sel && sel.rangeCount > 0) {
      const r = sel.getRangeAt(0);
      rect = r.getBoundingClientRect();
    }
  } catch (_) {}
  if (!rect || (rect.width === 0 && rect.height === 0)) {
    try { rect = fieldEl.getBoundingClientRect(); } catch (_) {}
  }
  popoverEl.classList.add("is-visible");
  hideToolbar();
  requestAnimationFrame(() => positionPopover(rect, canvas));
  if (inputEl) {
    setTimeout(() => { try { inputEl.focus(); inputEl.select(); } catch (_) {} }, 20);
  }
  return savedOffsets;
}



export function hidePopover() {
  if (popoverEl) popoverEl.classList.remove("is-visible");
  hideError();
  savedOffsets = null;
  toolbarInteraction = false;
  pointerActionHandled = false;
}

export function getSavedOffsets() {
  return savedOffsets ? { ...savedOffsets } : null;
}

export function getSavedToolbarOffsets() {
  return savedToolbarOffsets ? { ...savedToolbarOffsets } : null;
}

function positionToolbar(rect, canvas) {
  if (!toolbarEl || !rect) return;
  const doc = canvas.doc;
  const vw = doc.documentElement.clientWidth || window.innerWidth;
  const vh = doc.documentElement.clientHeight || window.innerHeight;
  const tbRect = { width: 220, height: 36 };
  try {
    const r = toolbarEl.getBoundingClientRect();
    if (r.width > 0) { tbRect.width = r.width; tbRect.height = r.height; }
  } catch (_) {}
  let left = Math.round(rect.left + rect.width / 2 - tbRect.width / 2);
  let top = Math.round(rect.top - tbRect.height - 8);
  if (top < 4) top = Math.round(rect.bottom + 8);
  if (left < 4) left = 4;
  if (left + tbRect.width > vw - 4) left = Math.round(vw - tbRect.width - 4);
  if (top + tbRect.height > vh - 4) top = Math.max(4, vh - tbRect.height - 4);
  toolbarEl.style.left = left + "px";
  toolbarEl.style.top = top + "px";
}

function positionPopover(rect, canvas) {
  if (!popoverEl || !rect) return;
  const doc = canvas.doc;
  const vw = doc.documentElement.clientWidth || window.innerWidth;
  const vh = doc.documentElement.clientHeight || window.innerHeight;
  let w = 260, h = 110;
  try {
    const r = popoverEl.getBoundingClientRect();
    if (r.width > 0) { w = r.width; h = r.height; }
  } catch (_) {}
  let left = Math.round(rect.left + rect.width / 2 - w / 2);
  let top = Math.round(rect.bottom + 8);
  if (top + h > vh - 4) top = Math.round(rect.top - h - 8);
  if (top < 4) top = 4;
  if (left < 4) left = 4;
  if (left + w > vw - 4) left = Math.round(vw - w - 4);
  popoverEl.style.left = left + "px";
  popoverEl.style.top = top + "px";
}

export function setToolbarCallbacks(callbacksObj) {
  if (!callbacksObj || typeof callbacksObj !== "object") return;
  callbacks.toggleMark = callbacksObj.toggleMark || null;
  callbacks.openLink = callbacksObj.openLink || null;
  callbacks.applyLink = callbacksObj.applyLink || null;
  callbacks.removeLink = callbacksObj.removeLink || null;
  callbacks.closeLink = callbacksObj.closeLink || null;
}

export function destroyToolbar() {
  try { if (toolbarEl && toolbarEl.parentNode) toolbarEl.parentNode.removeChild(toolbarEl); } catch (_) {}
  try { if (popoverEl && popoverEl.parentNode) popoverEl.parentNode.removeChild(popoverEl); } catch (_) {}
  toolbarEl = null;
  popoverEl = null;
  inputEl = null;
  errorEl = null;
  activeFieldEl = null;
  activeCanvas = null;
  savedOffsets = null;
  savedToolbarOffsets = null;
  toolbarInteraction = false;
  pointerActionHandled = false;
  callbacks = { toggleMark: null, openLink: null, applyLink: null, removeLink: null, closeLink: null };
}
