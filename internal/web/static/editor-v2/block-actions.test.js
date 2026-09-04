// block-actions.test.js — M7 duplicate/delete command behavior
import { beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";

const catalog = [
  { block: "test/container", version: 1, displayName: "Container", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "any", min: 0 }, editor: { category: "layout" } } },
  { block: "test/required-container", version: 1, displayName: "Required Container", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "any", min: 1 }, editor: { category: "layout" } } },
  { block: "test/limited-container", version: 1, displayName: "Limited Container", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "any", min: 0, max: 2 }, editor: { category: "layout" } } },
  { block: "test/leaf", version: 1, displayName: "Leaf", schema: { props: { type: "object", properties: { text: { type: "string", default: "" }, data: { type: "object", properties: { nested: { type: "string", default: "" } } } } }, settings: { type: "object", properties: { align: { type: "string", default: "left" } } }, children: { mode: "none" }, editor: { category: "text" } } },
];

globalThis.document = {
  getElementById(id) {
    if (id !== "editor-v2-bootstrap") return null;
    return { textContent: JSON.stringify({ document: { version: 1, nodes: [] }, catalog, definitions: catalog, resource: { id: "e1", type: "entry" } }) };
  },
};
globalThis.window = globalThis;
globalThis.window.location = { origin: "http://localhost", search: "" };
globalThis.location = globalThis.window.location;

const stateModule = await import("./state.js");
const commandModule = await import("./commands.js");
const { state } = stateModule;
const { duplicateNode, deleteNode } = commandModule;

function leaf(id, text = id) {
  return { id, block: "test/leaf", version: 1, props: { text, data: { nested: "value" } }, settings: { align: "center" }, children: [] };
}

function container(id, children = [], block = "test/container") {
  return { id, block, version: 1, props: {}, settings: {}, children };
}

function reset(nodes) {
  state.document = { version: 1, nodes: structuredClone(nodes) };
  state.selection = null;
  delete state.__pendingSelectionIds;
}

function collectIds(node, result = []) {
  result.push(node.id);
  for (const child of node.children || []) collectIds(child, result);
  return result;
}

describe("duplicateNode", () => {
  beforeEach(() => reset([]));

  it("inserts a semantic subtree copy immediately after the original with fresh IDs", () => {
    const nested = container("B", [container("B-stack", [leaf("B-text", "World")])]);
    reset([leaf("A"), nested, leaf("C")]);
    const originalJSON = JSON.stringify(state.document.nodes[1]);

    const result = duplicateNode({ nodeId: "B" });

    assert.equal(result.ok, true);
    assert.deepEqual(state.document.nodes.map((node) => node.id), ["A", "B", result.node.id, "C"]);
    const copy = state.document.nodes[2];
    const original = state.document.nodes[1];
    assert.equal(copy.block, original.block);
    assert.equal(copy.version, original.version);
    assert.deepEqual(copy.props, original.props);
    assert.deepEqual(copy.settings, original.settings);
    assert.deepEqual(
      copy.children.map((child) => ({ block: child.block, version: child.version, props: child.props, settings: child.settings })),
      original.children.map((child) => ({ block: child.block, version: child.version, props: child.props, settings: child.settings })),
    );
    const originalIds = new Set(collectIds(original));
    for (const id of collectIds(copy)) {
      assert.match(id, /^blk_[0-9a-f]{32}$/);
      assert.equal(originalIds.has(id), false);
    }
    assert.equal(JSON.stringify(state.document.nodes[1]), originalJSON);
    assert.equal(state.selection.nodeId, copy.id);
    assert.equal(state.selection.logical, true);
  });

  it("rejects duplication when the exact parent is at max children", () => {
    reset([container("parent", [leaf("A"), leaf("B")], "test/limited-container")]);
    const before = JSON.stringify(state.document);

    const result = duplicateNode({ nodeId: "A" });

    assert.equal(result.ok, false);
    assert.match(result.reason, /at most 2 child blocks/);
    assert.equal(JSON.stringify(state.document), before);
    assert.equal(state.selection, null);
  });
});

describe("deleteNode", () => {
  beforeEach(() => reset([]));

  it("removes an entire subtree and selects the next sibling", () => {
    reset([leaf("A"), container("B", [leaf("B-child")]), leaf("C")]);
    state.selection = { nodeId: "B", instanceKey: "instance-B", editable: true };

    const result = deleteNode({ nodeId: "B" });

    assert.equal(result.ok, true);
    assert.deepEqual(state.document.nodes.map((node) => node.id), ["A", "C"]);
    assert.equal(state.selection.nodeId, "C");
    assert.equal(state.document.nodes.some((node) => node.id === "B-child"), false);
  });

  it("uses previous sibling, parent, then clear as deterministic fallbacks", () => {
    reset([leaf("A"), leaf("B")]);
    assert.equal(deleteNode({ nodeId: "B" }).ok, true);
    assert.equal(state.selection.nodeId, "A");

    reset([container("parent", [leaf("only")])]);
    assert.equal(deleteNode({ nodeId: "only" }).ok, true);
    assert.equal(state.selection.nodeId, "parent");

    reset([leaf("only-root")]);
    assert.equal(deleteNode({ nodeId: "only-root" }).ok, true);
    assert.equal(state.selection, null);
  });

  it("respects generic parent minimum children constraints", () => {
    reset([container("parent", [leaf("only")], "test/required-container")]);
    const before = JSON.stringify(state.document);
    const rejected = deleteNode({ nodeId: "only" });
    assert.equal(rejected.ok, false);
    assert.match(rejected.reason, /requires at least 1 child block/);
    assert.equal(JSON.stringify(state.document), before);

    reset([container("parent", [leaf("A"), leaf("B")], "test/required-container")]);
    assert.equal(deleteNode({ nodeId: "B" }).ok, true);
    assert.deepEqual(state.document.nodes[0].children.map((node) => node.id), ["A"]);
  });

  it("rejects IDs that do not identify persisted SDT nodes", () => {
    reset([leaf("A")]);
    const before = JSON.stringify(state.document);
    const result = deleteNode({ nodeId: "rendered-occurrence-only" });
    assert.equal(result.ok, false);
    assert.equal(JSON.stringify(state.document), before);
  });
});
