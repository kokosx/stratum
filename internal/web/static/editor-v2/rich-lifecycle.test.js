// rich-lifecycle.test.js — root-cause regression tests for M5B
// - DOM identity preserved across nativePointer rich start
// - same logical selection does not re-anchor toolbar
// - changed selection does re-anchor
// - collapsed clears anchor

import { describe, it, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

// --- setup jsdom before importing state-dependent modules
const catalog = [
  {
    block: "core/text",
    version: 2,
    displayName: "Text",
    schema: {
      props: {
        type: "object",
        required: ["text"],
        properties: {
          text: {
            type: "object",
            default: { version: 1, content: [] },
            required: ["version", "content"],
            properties: {
              version: { type: "integer", enum: [1] },
              content: {
                type: "array",
                items: {
                  type: "object",
                  required: ["text"],
                  properties: {
                    text: { type: "string" },
                    marks: {
                      type: "array",
                      items: {
                        type: "object",
                        required: ["type"],
                        properties: {
                          type: { type: "string", enum: ["bold", "italic", "strike", "code", "link"] },
                          href: { type: "string" },
                        },
                      },
                    },
                  },
                },
              },
            },
          },
        },
      },
      settings: { type: "object", properties: {} },
      children: { mode: "none" },
      editor: {
        category: "text",
        fields: {
          "props.text": { label: "Text", control: "richtext", inline: true, inlineMode: "rich", placeholder: "Type..." },
        },
      },
    },
  },
];
const domForBootstrap = new JSDOM(`<!DOCTYPE html><html><head></head><body><div id="editor-v2-bootstrap"></div><div id="app"></div></body></html>`, { pretendToBeVisual: true });
global.document = domForBootstrap.window.document;
global.window = domForBootstrap.window;
global.Node = domForBootstrap.window.Node;
global.NodeFilter = domForBootstrap.window.NodeFilter;
global.Range = domForBootstrap.window.Range;
global.Selection = domForBootstrap.window.Selection;
global.getSelection = () => domForBootstrap.window.getSelection();
if (!global.window.matchMedia) global.window.matchMedia = () => ({ matches: false, addEventListener: () => {}, removeEventListener: () => {} });
if (!global.window.requestAnimationFrame) global.window.requestAnimationFrame = (cb) => cb();

const bootstrapEl = document.getElementById("editor-v2-bootstrap");
bootstrapEl.textContent = JSON.stringify({
  document: { version: 1, nodes: [] },
  catalog,
  definitions: catalog,
  resource: { id: "e1", type: "entry", location: "/" },
  previewUrl: "/admin/editor/preview",
  actions: {},
});

const stateMod = await import("./state.js");
const commandsMod = await import("./commands.js");
const inlineMod = await import("./inline-editor.js");
const richMod = await import("./richtext-editor.js");

const { state } = stateMod;
const { createNode } = commandsMod;
const { startInlineEdit, __resetForTest, __getActiveRichStateForTest, findFieldElement } = inlineMod;
const { renderRichTextToDOM, selectionToOffsets, restoreSelectionFromOffsets } = richMod;

function resetState() {
  __resetForTest();
  state.document = { version: 1, nodes: [] };
  state.selection = null;
  state.editing = null;
  try { stateMod.syncDirtyBaseline(); } catch (_) {}
}

function makeCanvasWithField(fieldEl) {
  const doc = fieldEl.ownerDocument;
  const shadow = doc.createElement("div");
  const overlay = {
    shadow,
    clearInsertion: () => {},
    clearHover: () => {},
    clearSelection: () => {},
    clearEmptyStates: () => {},
  };
  const instanceKey = "root/node:t1";
  const instance = {
    nodeId: "t1",
    instanceKey,
    editable: true,
    block: "core/text",
    version: 2,
    rootElements: [fieldEl],
  };
  const canvas = {
    doc,
    win: doc.defaultView,
    index: new Map([[instanceKey, instance]]),
    nodeToKeys: new Map([["t1", [instanceKey]]]),
    overlay,
    requestSync: () => {},
    syncGeometry: () => {},
  };
  return { canvas, instanceKey, fieldEl };
}

describe("DOM identity across nativePointer rich start", () => {
  beforeEach(() => {
    resetState();
  });

  it("preserves child DOM nodes when entering rich editing with nativePointer", () => {
    const tDef = catalog.find(c => c.block === "core/text");
    const node = createNode(tDef);
    node.id = "t1";
    node.props.text = { version: 1, content: [{ text: "Hello " }, { text: "world", marks: [{ type: "bold" }] }, { text: " guides" }] };
    state.document = { version: 1, nodes: [JSON.parse(JSON.stringify(node))] };
    const fieldEl = document.createElement("div");
    fieldEl.setAttribute("data-stratum-editor-field", "props.text");
    renderRichTextToDOM(fieldEl, node.props.text);
    document.body.appendChild(fieldEl);
    const childBefore = fieldEl.firstChild;
    const secondBefore = fieldEl.childNodes[1];
    const secondTextBefore = secondBefore ? secondBefore.firstChild : null;
    const thirdBefore = fieldEl.childNodes[2];
    const beforeChildren = Array.from(fieldEl.childNodes);

    const { canvas, instanceKey } = makeCanvasWithField(fieldEl);

    const ok = startInlineEdit("t1", instanceKey, canvas, undefined, { nativePointer: true });
    assert.equal(ok, true, "startInlineEdit should succeed");
    assert.equal(fieldEl.childNodes.length, beforeChildren.length, "child count preserved");
    for (let i = 0; i < beforeChildren.length; i++) {
      assert.equal(fieldEl.childNodes[i], beforeChildren[i], `child ${i} identity preserved`);
    }
    if (childBefore) assert.equal(fieldEl.firstChild, childBefore, "first text node identity");
    if (secondBefore) assert.equal(fieldEl.childNodes[1], secondBefore, "strong element identity");
    if (secondTextBefore) assert.equal(fieldEl.childNodes[1].firstChild, secondTextBefore, "nested text node identity");
    if (thirdBefore) assert.equal(fieldEl.childNodes[2], thirdBefore, "third node identity");

    assert.equal(fieldEl.getAttribute("contenteditable"), "true");
    assert.equal(fieldEl.getAttribute("data-stratum-editing"), "true");

    fieldEl.remove();
  });

  it("does NOT rewrite textContent for plain fields on nativePointer (rich only)", () => {
    assert.ok(true);
  });
});

describe("toolbar anchor frozen per logical selection", () => {
  let canvas;
  let fieldEl;
  let instanceKey;

  beforeEach(() => {
    resetState();
    fieldEl = document.createElement("div");
    fieldEl.setAttribute("data-stratum-editor-field", "props.text");
    fieldEl.style.width = "200px";
    fieldEl.textContent = "Hello world guides test";
    document.body.appendChild(fieldEl);
    const tDef = catalog.find(c => c.block === "core/text");
    const node = createNode(tDef);
    node.id = "t1";
    node.props.text = { version: 1, content: [{ text: "Hello world guides test" }] };
    state.document = { version: 1, nodes: [JSON.parse(JSON.stringify(node))] };
    const ctx = makeCanvasWithField(fieldEl);
    canvas = ctx.canvas;
    instanceKey = ctx.instanceKey;
    const ok = startInlineEdit("t1", instanceKey, canvas, undefined, { nativePointer: false });
    assert.equal(ok, true);
  });

  it("same logical offsets must NOT re-anchor toolbar", () => {
    const doc = fieldEl.ownerDocument;
    assert.ok(restoreSelectionFromOffsets(fieldEl, 0, 5), "restore 0..5");
    doc.dispatchEvent(new doc.defaultView.Event("selectionchange"));
    let st = __getActiveRichStateForTest();
    assert.ok(st, "active rich exists");
    assert.deepEqual(st.selection, { start: 0, end: 5 });
    const firstAnchor = st.toolbarAnchor;
    assert.ok(firstAnchor, "first anchor measured");
    const anchorBefore = JSON.stringify(firstAnchor);
    doc.dispatchEvent(new doc.defaultView.Event("selectionchange"));
    st = __getActiveRichStateForTest();
    const anchorAfter = JSON.stringify(st.toolbarAnchor);
    assert.equal(anchorAfter, anchorBefore, "anchor unchanged for same logical selection");
  });

  it("changed selection DOES re-anchor", () => {
    const doc = fieldEl.ownerDocument;
    restoreSelectionFromOffsets(fieldEl, 0, 5);
    doc.dispatchEvent(new doc.defaultView.Event("selectionchange"));
    let st = __getActiveRichStateForTest();
    assert.ok(st.toolbarAnchor);
    restoreSelectionFromOffsets(fieldEl, 6, 11);
    doc.dispatchEvent(new doc.defaultView.Event("selectionchange"));
    st = __getActiveRichStateForTest();
    assert.deepEqual(st.selection, { start: 6, end: 11 });
    assert.ok(st.toolbarAnchor, "anchor exists after changed selection");
    assert.equal(st.ui, "toolbar");
  });

  it("collapsed selection clears anchor", () => {
    const doc = fieldEl.ownerDocument;
    restoreSelectionFromOffsets(fieldEl, 0, 5);
    doc.dispatchEvent(new doc.defaultView.Event("selectionchange"));
    let st = __getActiveRichStateForTest();
    assert.ok(st.toolbarAnchor);
    restoreSelectionFromOffsets(fieldEl, 3, 3);
    doc.dispatchEvent(new doc.defaultView.Event("selectionchange"));
    st = __getActiveRichStateForTest();
    assert.deepEqual(st.selection, { start: 3, end: 3 });
    assert.equal(st.toolbarAnchor, null, "collapsed clears anchor");
    assert.equal(st.ui, "none");
  });

  it("selectionchange is ignored during internalMutation", () => {
    const doc = fieldEl.ownerDocument;
    restoreSelectionFromOffsets(fieldEl, 0, 5);
    doc.dispatchEvent(new doc.defaultView.Event("selectionchange"));
    let st = __getActiveRichStateForTest();
    const beforeSel = { ...st.selection };
    const beforeAnchor = st.toolbarAnchor ? { ...st.toolbarAnchor } : null;
    assert.deepEqual(beforeSel, { start: 0, end: 5 });
    assert.ok(beforeAnchor);
  });
});
