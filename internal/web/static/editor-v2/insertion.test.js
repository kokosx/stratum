// insertion.test.js — run with: node --test internal/web/static/editor-v2/insertion.test.js
import { describe, it, beforeEach } from "node:test";
import assert from "node:assert/strict";

// Mock DOM bootstrap for V2
const catalog = [
  { block: "core/section", version: 1, displayName: "Section", description: "Section", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "any", min: 0 }, editor: { category: "layout" } } },
  { block: "core/stack", version: 1, displayName: "Stack", description: "Stack", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "any", min: 0 }, editor: { category: "layout" } } },
  { block: "core/grid", version: 1, displayName: "Grid", description: "Grid", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "any", min: 0 }, editor: { category: "layout" } } },
  { block: "core/heading", version: 1, displayName: "Heading", description: "Heading descr", schema: { props: { type: "object", properties: { text: { type: "string", default: "Hello" }, level: { type: "integer", enum: [1, 2, 3], default: 2 } } }, settings: { type: "object", properties: { align: { type: "string", enum: ["left", "center"], default: "left" } } }, children: { mode: "none" }, editor: { category: "text" } } },
  { block: "core/text", version: 1, displayName: "Text", description: "Text descr", schema: { props: { type: "object", properties: { text: { type: "string", default: "" } } }, settings: { type: "object", properties: {} }, children: { mode: "none" }, editor: { category: "text" } } },
  { block: "core/button", version: 1, displayName: "Button", description: "Button", schema: { props: { type: "object", properties: { label: { type: "string", default: "Click" }, url: { type: "string", default: "#" } } }, settings: { type: "object", properties: { variant: { type: "string", enum: ["primary", "secondary"], default: "primary" } } }, children: { mode: "none" }, editor: { category: "design" } } },
  { block: "core/accordion", version: 1, displayName: "Accordion", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "allowed", blocks: ["core/accordion-item"], min: 1, max: 3 }, editor: { category: "design" } } },
  { block: "core/accordion-item", version: 1, displayName: "Accordion Item", schema: { props: { type: "object", properties: { title: { type: "string", default: "Item" } } }, settings: { type: "object", properties: {} }, children: { mode: "any", min: 0 }, editor: { category: "design" } } },
  { block: "core/button-group", version: 1, displayName: "Button Group", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "allowed", blocks: ["core/button"], min: 1, max: 1 }, editor: { category: "design", starterChildren: [{ block: "core/button", version: 1 }] } } },
  { block: "core/image", version: 1, displayName: "Image", schema: { props: { type: "object", properties: { mediaId: { type: "string", default: "" } } }, settings: { type: "object", properties: {} }, children: { mode: "none" }, editor: { category: "media" } } },
];

globalThis.document = {
  getElementById: (id) => {
    if (id === "editor-v2-bootstrap") {
      return { textContent: JSON.stringify({ document: { version: 1, nodes: [] }, catalog, definitions: catalog, resource: { id: "e1", type: "entry" }, previewUrl: "/admin/editor/preview" }) };
    }
    return null;
  },
  createElement: () => ({ style: {}, setAttribute: () => {}, append: () => {} }),
  querySelector: () => null,
  querySelectorAll: () => [],
};
globalThis.window = globalThis;
globalThis.window.location = { origin: "http://localhost", search: "" };
globalThis.location = globalThis.window.location;
try {
  if (!globalThis.crypto || !globalThis.crypto.getRandomValues) {
    globalThis.crypto = { getRandomValues(arr) { for (let i = 0; i < arr.length; i++) arr[i] = Math.floor(Math.random() * 256); return arr; } };
  }
} catch (_) {
  // Node 24 crypto is read-only; polyfill via assignment to global crypto sub
  try { globalThis.crypto.getRandomValues = (arr) => { for (let i = 0; i < arr.length; i++) arr[i] = Math.floor(Math.random() * 256); return arr; }; } catch (_) {}
}

const stateMod = await import("./state.js");
const insertionMod = await import("./insertion.js");
const commandsMod = await import("./commands.js");

const { state, findDocumentNode } = stateMod;
const { canInsert, hasLegalInsertion, legalBlocksFor, getInsertionTarget, setInsertionTarget, clearInsertionTarget, resolveGlobalInsertion } = insertionMod;
const { createNode, insertBlock, randomID, defaultValue } = commandsMod;

function resetDoc(nodes = []) {
  state.document = { version: 1, nodes: JSON.parse(JSON.stringify(nodes)) };
  // ensure nodes have children array
  const walk = (list) => { for (const n of list) { n.children ||= []; walk(n.children); } };
  walk(state.document.nodes);
  clearInsertionTarget();
  state.selection = null;
  state.__pendingSelectionId = null;
}

describe("randomID + defaultValue", () => {
  it("randomID format blk_<hex> and unique", () => {
    const a = randomID();
    const b = randomID();
    assert.match(a, /^blk_[0-9a-f]{32}$/);
    assert.notEqual(a, b);
  });
  it("defaultValue uses schema defaults", () => {
    const def = catalog.find(c => c.block === "core/heading");
    const props = defaultValue(def.schema.props);
    assert.equal(props.text, "Hello");
    assert.equal(props.level, 2);
  });
  it("createNode uses schema defaults and stable IDs", () => {
    const def = catalog.find(c => c.block === "core/button");
    const node = createNode(def);
    assert.equal(node.block, "core/button");
    assert.match(node.id, /^blk_/);
    assert.equal(node.props.label, "Click");
    assert.equal(node.settings.variant, "primary");
    assert.equal(node.children.length, 0);
  });
  it("starterChildren generic: Button Group creates one Button", () => {
    const def = catalog.find(c => c.block === "core/button-group");
    const node = createNode(def);
    assert.equal(node.children.length, 1);
    assert.equal(node.children[0].block, "core/button");
    assert.match(node.children[0].id, /^blk_/);
  });
  it("min>0 single allowed fallback: Accordion creates item without starterChildren misconfig", () => {
    const defNoStarter = { block: "core/accordion", version: 1, displayName: "Accordion", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "allowed", blocks: ["core/accordion-item"], min: 1 }, editor: { category: "design" } } };
    const node = createNode(defNoStarter);
    assert.ok(node.children.length >= 1);
    assert.equal(node.children[0].block, "core/accordion-item");
  });
});

describe("insertBlock schema legality", () => {
  beforeEach(() => resetDoc([]));
  it("root insertion allowed for catalog blocks, index 0", () => {
    const heading = catalog.find(c => c.block === "core/heading");
    const res = insertBlock({ definition: heading, parentId: null, index: 0 });
    assert.equal(res.ok, true);
    assert.equal(state.document.nodes.length, 1);
    assert.equal(state.document.nodes[0].block, "core/heading");
  });
  it("allowed children: Accordion only accepts accordion-item", () => {
    const accDef = catalog.find(c => c.block === "core/accordion");
    const accNode = createNode(accDef);
    resetDoc([accNode]);
    const parent = state.document.nodes[0];
    const wrong = catalog.find(c => c.block === "core/heading");
    const right = catalog.find(c => c.block === "core/accordion-item");
    const canWrong = canInsert(parent, wrong, 0);
    assert.equal(canWrong.ok, false);
    assert.match(canWrong.reason, /not allowed/);
    const canRight = canInsert(parent, right, 1);
    assert.equal(canRight.ok, true);
  });
  it("mode none blocks cannot contain children", () => {
    const heading = catalog.find(c => c.block === "core/heading");
    const hNode = createNode(heading);
    resetDoc([hNode]);
    const parent = state.document.nodes[0];
    const child = catalog.find(c => c.block === "core/text");
    const c = canInsert(parent, child, 0);
    assert.equal(c.ok, false);
    assert.match(c.reason, /does not allow/);
  });
  it("max children respects limit", () => {
    const accDef = catalog.find(c => c.block === "core/accordion");
    const accNode = createNode(accDef);
    // acc already has 1 item via factory, add 2 more to reach max 3
    const itemDef = catalog.find(c => c.block === "core/accordion-item");
    accNode.children.push(createNode(itemDef), createNode(itemDef));
    resetDoc([accNode]);
    const parent = state.document.nodes[0];
    assert.equal(parent.children.length, 3);
    assert.equal(hasLegalInsertion(parent, 3), false);
    const extra = catalog.find(c => c.block === "core/accordion-item");
    const c = canInsert(parent, extra, 3);
    assert.equal(c.ok, false);
    assert.match(c.reason, /at most/);
  });
  it("exact index insertion at root and nested", () => {
    const secDef = catalog.find(c => c.block === "core/section");
    const secA = createNode(secDef); secA.id = "secA";
    const secB = createNode(secDef); secB.id = "secB";
    resetDoc([secA, secB]);
    const heading = catalog.find(c => c.block === "core/heading");
    // insert between A and B at root index 1
    const res = insertBlock({ definition: heading, parentId: null, index: 1 });
    assert.equal(res.ok, true);
    assert.equal(state.document.nodes.length, 3);
    assert.equal(state.document.nodes[0].id, "secA");
    assert.equal(state.document.nodes[1].block, "core/heading");
    assert.equal(state.document.nodes[2].id, "secB");
  });
  it("nested insertion inside Stack", () => {
    const secDef = catalog.find(c => c.block === "core/section");
    const stackDef = catalog.find(c => c.block === "core/stack");
    const sec = createNode(secDef); sec.id = "sec1";
    const stack = createNode(stackDef); stack.id = "stack1";
    const heading = createNode(catalog.find(c => c.block === "core/heading")); heading.id = "h1";
    const button = createNode(catalog.find(c => c.block === "core/button")); button.id = "b1";
    stack.children = [heading, button];
    sec.children = [stack];
    resetDoc([sec]);
    const textDef = catalog.find(c => c.block === "core/text");
    const res = insertBlock({ definition: textDef, parentId: "stack1", index: 1 });
    assert.equal(res.ok, true);
    const stackNow = state.document.nodes[0].children[0];
    assert.equal(stackNow.children.length, 3);
    assert.equal(stackNow.children[0].id, "h1");
    assert.equal(stackNow.children[1].block, "core/text");
    assert.equal(stackNow.children[2].id, "b1");
  });
  it("invalid insertion leaves document unchanged", () => {
    const accDef = catalog.find(c => c.block === "core/accordion");
    const acc = createNode(accDef);
    resetDoc([acc]);
    const before = JSON.stringify(state.document);
    const wrong = catalog.find(c => c.block === "core/heading");
    const parent = state.document.nodes[0];
    const res = insertBlock({ definition: wrong, parentId: parent.id, index: 1 });
    assert.equal(res.ok, false);
    assert.equal(JSON.stringify(state.document), before);
  });
});

describe("legalBlocksFor + hasLegalInsertion + resolveGlobalInsertion", () => {
  beforeEach(() => resetDoc([]));
  it("legalBlocksFor filters only allowed", () => {
    const accDef = catalog.find(c => c.block === "core/accordion");
    const acc = createNode(accDef);
    resetDoc([acc]);
    const parent = state.document.nodes[0];
    const legal = legalBlocksFor(parent, 0);
    assert.ok(legal.length === 1);
    assert.equal(legal[0].block, "core/accordion-item");
  });
  it("hasLegalInsertion false for none/max", () => {
    const heading = createNode(catalog.find(c => c.block === "core/heading"));
    resetDoc([heading]);
    const hNode = state.document.nodes[0];
    assert.equal(hasLegalInsertion(hNode, 0), false);
  });
  it("resolveGlobalInsertion conservative: no teleport without selection", () => {
    const secDef = catalog.find(c => c.block === "core/section");
    resetDoc([createNode(secDef)]);
    state.selection = null;
    const heading = catalog.find(c => c.block === "core/heading");
    const fallback = resolveGlobalInsertion(heading);
    assert.equal(fallback, null);
  });
  it("resolveGlobalInsertion with selected container appends inside", () => {
    const stackDef = catalog.find(c => c.block === "core/stack");
    const stack = createNode(stackDef); stack.id = "s1";
    resetDoc([stack]);
    state.selection = { nodeId: "s1", instanceKey: "k1", editable: true };
    const text = catalog.find(c => c.block === "core/text");
    const fb = resolveGlobalInsertion(text);
    assert.deepEqual(fb, { parentId: "s1", index: 0 });
  });
  it("resolveGlobalInsertion after selection sibling", () => {
    const secDef = catalog.find(c => c.block === "core/section");
    const sec = createNode(secDef); sec.id = "secX";
    const h = createNode(catalog.find(c => c.block === "core/heading")); h.id = "hX";
    sec.children = [h];
    const rootSec = createNode(secDef); rootSec.id = "rootSec";
    resetDoc([sec, rootSec]);
    // select heading inside sec, try to insert button (any allowed) -> should append after heading in sec
    state.selection = { nodeId: "hX", instanceKey: "kH", editable: true };
    const btn = catalog.find(c => c.block === "core/button");
    const fb = resolveGlobalInsertion(btn);
    assert.equal(fb.parentId, "secX");
    assert.equal(fb.index, 1);
  });
});

describe("document subscription immutable", () => {
  it("setDocument notifies and dirty flag", async () => {
    // need to test via commands insert
    resetDoc([]);
    let notified = false;
    const unsub = stateMod.subscribeDocument(() => { notified = true; });
    const heading = catalog.find(c => c.block === "core/heading");
    insertBlock({ definition: heading, parentId: null, index: 0 });
    assert.equal(notified, true);
    assert.equal(state.dirty, true);
    unsub();
  });
});
