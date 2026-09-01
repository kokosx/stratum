// toolbar-interaction.test.js — behavioral tests for the stateless rich-text toolbar

import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

class FakeClassList {
  constructor(element) { this.element = element; }
  values() { return new Set(String(this.element.className || "").split(/\s+/).filter(Boolean)); }
  contains(name) { return this.values().has(name); }
  add(name) { const values = this.values(); values.add(name); this.element.className = [...values].join(" "); }
  remove(name) { const values = this.values(); values.delete(name); this.element.className = [...values].join(" "); }
  toggle(name, force) {
    const enabled = force === undefined ? !this.contains(name) : !!force;
    if (enabled) this.add(name); else this.remove(name);
    return enabled;
  }
}

class FakeElement {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.parentNode = null;
    this.children = [];
    this.attributes = new Map();
    this.listeners = new Map();
    this.className = "";
    this.classList = new FakeClassList(this);
    this.style = {};
    this.textContent = "";
    this.value = "";
  }

  appendChild(child) { child.parentNode = this; this.children.push(child); return child; }
  removeChild(child) { this.children = this.children.filter((entry) => entry !== child); child.parentNode = null; }
  remove() { this.parentNode?.removeChild(this); }
  setAttribute(name, value) { this.attributes.set(name, String(value)); }
  getAttribute(name) { return this.attributes.has(name) ? this.attributes.get(name) : null; }
  addEventListener(type, listener) {
    const listeners = this.listeners.get(type) || [];
    listeners.push(listener);
    this.listeners.set(type, listeners);
  }
  dispatch(type, init = {}) {
    const event = {
      type,
      key: init.key || "",
      defaultPrevented: false,
      propagationStopped: false,
      preventDefault() { this.defaultPrevented = true; },
      stopPropagation() { this.propagationStopped = true; },
    };
    for (const listener of this.listeners.get(type) || []) listener(event);
    return event;
  }
  contains(candidate) {
    if (candidate === this) return true;
    return this.children.some((child) => child.contains?.(candidate));
  }
  matches(selector) {
    if (selector === "input") return this.tagName === "INPUT";
    if (selector.startsWith(".")) return this.classList.contains(selector.slice(1));
    if (selector === "[data-mark]") return this.attributes.has("data-mark");
    const mark = selector.match(/^\[data-mark="([^"]+)"\]$/);
    return !!mark && this.getAttribute("data-mark") === mark[1];
  }
  querySelectorAll(selector) {
    const matches = [];
    for (const child of this.children) {
      if (child.matches(selector)) matches.push(child);
      matches.push(...child.querySelectorAll(selector));
    }
    return matches;
  }
  querySelector(selector) { return this.querySelectorAll(selector)[0] || null; }
  getBoundingClientRect() {
    if (this.classList.contains("richtext-toolbar")) return { left: 0, top: 0, width: 220, height: 36, right: 220, bottom: 36 };
    if (this.classList.contains("richtext-link-popover")) return { left: 0, top: 0, width: 260, height: 110, right: 260, bottom: 110 };
    return { left: 0, top: 0, width: 10, height: 10, right: 10, bottom: 10 };
  }
  focus() { this.ownerDocument.activeElement = this; }
  select() { this.selected = true; }

  set innerHTML(value) {
    this.children = [];
    if (!String(value).includes("richtext-link-popover__input")) return;
    const title = this.ownerDocument.createElement("div");
    title.className = "richtext-link-popover__title";
    const input = this.ownerDocument.createElement("input");
    input.className = "richtext-link-popover__input";
    const error = this.ownerDocument.createElement("div");
    error.className = "richtext-link-popover__error";
    const actions = this.ownerDocument.createElement("div");
    actions.className = "richtext-link-popover__actions";
    const remove = this.ownerDocument.createElement("button");
    remove.className = "richtext-link-popover__btn richtext-link-popover__btn--remove";
    const cancel = this.ownerDocument.createElement("button");
    cancel.className = "richtext-link-popover__btn richtext-link-popover__btn--cancel";
    const apply = this.ownerDocument.createElement("button");
    apply.className = "richtext-link-popover__btn richtext-link-popover__btn--apply";
    actions.appendChild(remove);
    actions.appendChild(cancel);
    actions.appendChild(apply);
    this.appendChild(title);
    this.appendChild(input);
    this.appendChild(error);
    this.appendChild(actions);
  }
}

class FakeDocument {
  constructor() {
    this.documentElement = { clientWidth: 1024, clientHeight: 768 };
    this.activeElement = null;
  }
  createElement(tagName) { return new FakeElement(tagName, this); }
}

const dirname = path.dirname(fileURLToPath(import.meta.url));
const toolbarPath = path.join(dirname, "richtext-toolbar.js");
const Toolbar = await import("./richtext-toolbar.js");

let doc;
let shadow;
let canvas;

function toolbarElement() { return shadow.querySelector(".richtext-toolbar"); }
function popoverElement() { return shadow.querySelector(".richtext-link-popover"); }

beforeEach(() => {
  doc = new FakeDocument();
  shadow = new FakeElement("shadow-root", doc);
  canvas = { doc, overlay: { shadow } };
  globalThis.requestAnimationFrame = (callback) => { callback(); return 1; };
});

afterEach(() => Toolbar.destroyToolbar());

describe("stateless toolbar actions", () => {
  it("performs one mark action on pointerdown and none on the following click", () => {
    const actions = [];
    Toolbar.setToolbarCallbacks({ toggleMark: (mark) => actions.push(mark) });
    Toolbar.showToolbar(canvas, { left: 20, top: 40, right: 80, bottom: 55, width: 60, height: 15 }, ["bold"]);

    const bold = toolbarElement().querySelector('[data-mark="bold"]');
    const pointer = bold.dispatch("pointerdown");
    const click = bold.dispatch("click");

    assert.deepEqual(actions, ["bold"]);
    assert.equal(pointer.defaultPrevented, true);
    assert.equal(click.defaultPrevented, true);
    assert.equal(bold.getAttribute("aria-pressed"), "true");
  });

  it("performs keyboard actions exactly once for Enter and Space", () => {
    const actions = [];
    Toolbar.setToolbarCallbacks({ toggleMark: (mark) => actions.push(mark) });
    Toolbar.showToolbar(canvas, { left: 20, top: 40, right: 80, bottom: 55, width: 60, height: 15 }, []);

    const italic = toolbarElement().querySelector('[data-mark="italic"]');
    italic.dispatch("keydown", { key: "Enter" });
    italic.dispatch("click");
    italic.dispatch("keydown", { key: " " });

    assert.deepEqual(actions, ["italic", "italic"]);
  });

  it("emits the Link action without requiring editor selection data", () => {
    let opened = 0;
    Toolbar.setToolbarCallbacks({ openLink: () => { opened += 1; } });
    Toolbar.showToolbar(canvas, { left: 20, top: 40, right: 80, bottom: 55, width: 60, height: 15 }, []);

    const link = toolbarElement().querySelector('[data-mark="link"]');
    link.dispatch("pointerdown");
    link.dispatch("click");

    assert.equal(opened, 1);
  });
});

describe("link popover actions", () => {
  it("focuses the URL input and applies a safe URL", () => {
    const values = [];
    Toolbar.setToolbarCallbacks({ applyLink: (href) => values.push(href) });
    Toolbar.showPopover(canvas, { left: 20, top: 40, right: 80, bottom: 55, width: 60, height: 15 }, "");

    const popover = popoverElement();
    const input = popover.querySelector("input");
    input.value = "/guide";
    popover.querySelector(".richtext-link-popover__btn--apply").dispatch("click");

    assert.equal(doc.activeElement, input);
    assert.deepEqual(values, ["/guide"]);
  });

  it("reports Escape through the close callback without applying", () => {
    let closed = 0;
    Toolbar.setToolbarCallbacks({ closeLink: () => { closed += 1; } });
    Toolbar.showPopover(canvas, { left: 20, top: 40, right: 80, bottom: 55, width: 60, height: 15 }, "/old");

    popoverElement().querySelector("input").dispatch("keydown", { key: "Escape" });

    assert.equal(closed, 1);
  });

  it("cancels without applying and removes invalid URLs", () => {
    let closed = 0;
    const applied = [];
    Toolbar.setToolbarCallbacks({ closeLink: () => { closed += 1; }, applyLink: (href) => applied.push(href) });
    Toolbar.showPopover(canvas, { left: 20, top: 40, right: 80, bottom: 55, width: 60, height: 15 }, "");
    popoverElement().querySelector(".richtext-link-popover__btn--cancel").dispatch("click");
    assert.equal(closed, 1);
    assert.equal(applied.length, 0);
    const input = popoverElement().querySelector("input");
    input.value = "javascript:alert(1)";
    popoverElement().querySelector(".richtext-link-popover__btn--apply").dispatch("click");
    assert.equal(applied.length, 0);
    assert.equal(popoverElement().querySelector(".richtext-link-popover__error").textContent.length > 0, true);
  });
});

describe("interaction architecture", () => {
  it("does not keep editor selection or patchwork interaction flags", () => {
    const source = fs.readFileSync(toolbarPath, "utf8");
    assert.doesNotMatch(source, /savedToolbarOffsets|savedOffsets|toolbarInteraction|suppressNextClick/);
    assert.doesNotMatch(source, /setTimeout|queueMicrotask/);
  });
});
