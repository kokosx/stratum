// placement.test.js — M6 placement.parents generic check
import { describe, it, beforeEach } from "node:test";
import assert from "node:assert/strict";

const catalog = [
  { block: "core/section", version: 1, displayName: "Section", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "any", min: 0 }, editor: { category: "layout" } } },
  { block: "core/stack", version: 1, displayName: "Stack", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "any", min: 0 }, editor: { category: "layout" } } },
  { block: "core/accordion", version: 1, displayName: "Accordion", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "allowed", blocks: ["core/accordion-item"], min: 1 }, editor: { category: "design" } } },
  { block: "core/accordion-item", version: 1, displayName: "Accordion Item", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "any", min: 0 }, placement: { parents: ["core/accordion"] }, editor: { category: "design" } } },
  { block: "core/heading", version: 1, displayName: "Heading", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "none" }, editor: { category: "text" } } },
  { block: "test/parent-a", version: 1, displayName: "Parent A", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "any" }, editor: {} } },
  { block: "test/parent-b", version: 1, displayName: "Parent B", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "any" }, editor: {} } },
  { block: "test/parent-c", version: 1, displayName: "Parent C", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "any" }, editor: {} } },
  { block: "test/child", version: 1, displayName: "Child", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "none" }, placement: { parents: ["test/parent-a", "test/parent-b"] }, editor: {} } },
];

globalThis.document = {
  getElementById: (id) => {
    if (id === "editor-v2-bootstrap") return { textContent: JSON.stringify({ document: { version: 1, nodes: [] }, catalog, definitions: catalog, resource: { id: "e1", type: "entry" }, previewUrl: "/admin/editor/preview" }) };
    return null;
  },
  createElement: () => ({ style: {}, setAttribute: () => {}, append: () => {} }),
  querySelector: () => null,
  querySelectorAll: () => [],
};
globalThis.window = globalThis;
globalThis.window.location = { origin: "http://localhost", search: "" };
globalThis.location = globalThis.window.location;

const stateMod = await import("./state.js");
const insMod = await import("./insertion.js");
const cmdMod = await import("./commands.js");
const { state, findDocumentNode } = stateMod;
const { canInsert, canMove, legalBlocksFor } = insMod;
const { createNode, moveNode } = cmdMod;

function reset(nodes) {
  state.document = { version: 1, nodes: JSON.parse(JSON.stringify(nodes)) };
  const walk = (l) => { for (const n of l) { n.children ||= []; walk(n.children); } };
  walk(state.document.nodes);
  state.selection = null;
}

describe("M6 placement.parents generic", () => {
  beforeEach(() => reset([]));
  it("test/child: parent-a legal, parent-b legal, parent-c illegal, root illegal", () => {
    const pa = { id: "pa", block: "test/parent-a", version: 1, props: {}, settings: {}, children: [] };
    const pb = { id: "pb", block: "test/parent-b", version: 1, props: {}, settings: {}, children: [] };
    const pc = { id: "pc", block: "test/parent-c", version: 1, props: {}, settings: {}, children: [] };
    reset([pa, pb, pc]);
    const childDef = stateMod.definitionForBlock("test/child");
    assert.equal(canInsert(findDocumentNode("pa"), childDef, 0).ok, true);
    assert.equal(canInsert(findDocumentNode("pb"), childDef, 0).ok, true);
    assert.equal(canInsert(findDocumentNode("pc"), childDef, 0).ok, false);
    assert.equal(canInsert(null, childDef, 0).ok, false);
  });

  it("Accordion Item cannot be at root, Section, Stack", () => {
    const sec = createNode(stateMod.definitionForBlock("core/section")); sec.id = "sec1";
    const stack = createNode(stateMod.definitionForBlock("core/stack")); stack.id = "stk1";
    reset([sec, stack]);
    const itemDef = stateMod.definitionForBlock("core/accordion-item");
    assert.equal(canInsert(null, itemDef, 0).ok, false);
    assert.equal(canInsert(findDocumentNode("sec1"), itemDef, 0).ok, false);
    assert.equal(canInsert(findDocumentNode("stk1"), itemDef, 0).ok, false);
  });

  it("Accordion Item can be inside Accordion", () => {
    const acc = createNode(stateMod.definitionForBlock("core/accordion")); acc.id = "acc1"; acc.children = [];
    reset([acc]);
    const itemDef = stateMod.definitionForBlock("core/accordion-item");
    assert.equal(canInsert(findDocumentNode("acc1"), itemDef, 0).ok, true);
  });

  it("Blocks filtering follows placement (root excludes, accordion includes)", () => {
    reset([]);
    const rootLegal = legalBlocksFor(null, 0).map((b) => b.block);
    assert.equal(rootLegal.includes("core/accordion-item"), false);
    const acc = createNode(stateMod.definitionForBlock("core/accordion")); acc.id = "acc1"; acc.children = [];
    reset([acc]);
    const insideAcc = legalBlocksFor(findDocumentNode("acc1"), 0).map((b) => b.block);
    assert.equal(insideAcc.includes("core/accordion-item"), true);
    const sec = createNode(stateMod.definitionForBlock("core/section")); sec.id = "sec1";
    reset([sec]);
    const insideSec = legalBlocksFor(findDocumentNode("sec1"), 0).map((b) => b.block);
    assert.equal(insideSec.includes("core/accordion-item"), false);
  });

  it("canMove respects placement for accordion-item", () => {
    const acc = { id: "acc", block: "core/accordion", version: 1, props: {}, settings: {}, children: [
      { id: "a1", block: "core/accordion-item", version: 1, props: {}, settings: {}, children: [] },
      { id: "a2", block: "core/accordion-item", version: 1, props: {}, settings: {}, children: [] },
    ]};
    const sec = { id: "sec", block: "core/section", version: 1, props: {}, settings: {}, children: [] };
    reset([acc, sec]);
    assert.equal(canMove("a1", null, 1).ok, false);
    assert.equal(canMove("a1", "sec", 0).ok, false);
    const accB = { id: "accB", block: "core/accordion", version: 1, props: {}, settings: {}, children: [
      { id: "b1", block: "core/accordion-item", version: 1, props: {}, settings: {}, children: [] },
    ]};
    reset([acc, accB]);
    assert.equal(canMove("a1", "accB", 1).ok, true);
  });

  it("Accordion can move to Section/root, items reorder inside same accordion", () => {
    const sec = { id: "sec", block: "core/section", version: 1, props: {}, settings: {}, children: [] };
    const acc = { id: "acc", block: "core/accordion", version: 1, props: {}, settings: {}, children: [
      { id: "i1", block: "core/accordion-item", version: 1, props: {}, settings: {}, children: [] },
      { id: "i2", block: "core/accordion-item", version: 1, props: {}, settings: {}, children: [] },
      { id: "i3", block: "core/accordion-item", version: 1, props: {}, settings: {}, children: [] },
    ]};
    reset([sec, acc]);
    assert.equal(canMove("acc", "sec", 0).ok, true);
    assert.equal(canMove("acc", null, 0).ok, true);
    reset([acc]);
    assert.equal(canMove("i3", "acc", 0).ok, true);
    const res = moveNode({ nodeId: "i3", parentId: "acc", index: 0 });
    assert.equal(res.ok, true);
    assert.deepEqual(findDocumentNode("acc").children.map((c) => c.id), ["i3", "i1", "i2"]);
  });

  it("source min prevents moving last item out", () => {
    const acc = { id: "acc", block: "core/accordion", version: 1, props: {}, settings: {}, children: [
      { id: "only", block: "core/accordion-item", version: 1, props: {}, settings: {}, children: [] },
    ]};
    reset([acc]);
    assert.equal(canMove("only", null, 1).ok, false);
  });
});
