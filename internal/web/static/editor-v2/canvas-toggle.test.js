// canvas-toggle.test.js — allowsPreviewDefault unit tests
import { describe, it, before } from "node:test";
import assert from "node:assert/strict";

// Mock document before importing canvas/state
globalThis.document = {
  getElementById: (id) => {
    if (id === "editor-v2-bootstrap") return { textContent: JSON.stringify({ document: { version:1, nodes:[] }, catalog:[], definitions:[], resource:{id:"e1",type:"entry"}, previewUrl:"/admin/editor/preview" }) };
    return null;
  },
  createElement: () => ({ style:{}, setAttribute:()=>{}, append:()=>{}, getBoundingClientRect:()=>({left:0,top:0,width:0,height:0}) }),
  querySelector: ()=>null,
  querySelectorAll: ()=>[],
  body: { appendChild:()=>{} },
  documentElement: { clientWidth:1024, clientHeight:768 },
  createTreeWalker: ()=>({ nextNode:()=>null }),
};
globalThis.window = globalThis;
globalThis.window.location = { origin:"http://localhost", search:"" };
globalThis.location = globalThis.window.location;

let allowsPreviewDefault;
before(async () => {
  const mod = await import("./canvas.js");
  allowsPreviewDefault = mod.allowsPreviewDefault;
});

// Minimal DOM mock for closest/matches
function mockEl(tag, parent = null, closestFn = null) {
  const el = {
    tagName: tag.toUpperCase(),
    parentElement: parent,
    nodeType: 1,
    closest: closestFn || function (sel) {
      if (sel === "summary" && this.tagName.toLowerCase() === "summary") return this;
      if (parent) return parent.closest ? parent.closest(sel) : null;
      return null;
    },
    matches: function (sel) {
      return sel.toLowerCase() === this.tagName.toLowerCase();
    },
  };
  return el;
}

describe("allowsPreviewDefault", () => {
  it("summary inside details -> true", () => {
    const details = mockEl("details");
    const summary = mockEl("summary", details);
    summary.closest = (sel) => (sel === "summary" ? summary : null);
    summary.parentElement = details;
    details.matches = (sel) => sel === "details";
    assert.equal(allowsPreviewDefault(summary), true);
  });

  it("descendant inside summary (span) -> true", () => {
    const details = mockEl("details");
    const summary = mockEl("summary", details);
    const span = mockEl("span", summary);
    span.closest = (sel) => (sel === "summary" ? summary : null);
    summary.parentElement = details;
    details.matches = (s) => s === "details";
    assert.equal(allowsPreviewDefault(span), true);
  });

  it("text node inside summary -> true", () => {
    const details = mockEl("details");
    const summary = mockEl("summary", details);
    const text = { nodeType: 3, parentElement: summary };
    summary.closest = (sel) => (sel === "summary" ? summary : null);
    summary.parentElement = details;
    details.matches = (s) => s === "details";
    assert.equal(allowsPreviewDefault(text), true);
  });

  it("ordinary div -> false", () => {
    const div = mockEl("div");
    div.closest = () => null;
    assert.equal(allowsPreviewDefault(div), false);
  });

  it("anchor -> false", () => {
    const a = mockEl("a");
    a.closest = () => null;
    assert.equal(allowsPreviewDefault(a), false);
  });

  it("button -> false", () => {
    const btn = mockEl("button");
    btn.closest = () => null;
    assert.equal(allowsPreviewDefault(btn), false);
  });

  it("summary not inside details -> false", () => {
    const div = mockEl("div");
    const summary = mockEl("summary", div);
    summary.closest = (s) => (s === "summary" ? summary : null);
    summary.parentElement = div;
    div.matches = () => false;
    assert.equal(allowsPreviewDefault(summary), false);
  });

  it("null and non-element -> false", () => {
    assert.equal(allowsPreviewDefault(null), false);
    assert.equal(allowsPreviewDefault({}), false);
    assert.equal(allowsPreviewDefault({ nodeType: 3, parentElement: null }), false);
  });
});
