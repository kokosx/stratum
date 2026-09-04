// history.test.js — M7 semantic document snapshot history
import { beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";

const catalog = [
  { block: "test/container", version: 1, displayName: "Container", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "any", min: 0 }, editor: { category: "layout" } } },
  { block: "test/limited-container", version: 1, displayName: "Limited Container", schema: { props: { type: "object", properties: {} }, settings: { type: "object", properties: {} }, children: { mode: "any", min: 0, max: 1 }, editor: { category: "layout" } } },
  { block: "test/leaf", version: 1, displayName: "Leaf", schema: { props: { type: "object", properties: { text: { type: "string", default: "" } } }, settings: { type: "object", properties: {} }, children: { mode: "none" }, editor: { category: "text" } } },
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
const historyModule = await import("./history.js");
const { state, setDocument, syncDirtyBaseline } = stateModule;
const { insertBlock, moveNode, duplicateNode, deleteNode } = commandModule;
const { initializeHistory, resetHistory, canUndo, canRedo, undo, redo, subscribeHistory } = historyModule;

function leaf(id, text = id) {
  return { id, block: "test/leaf", version: 1, props: { text }, settings: {}, children: [] };
}

function reset(nodes) {
  state.document = { version: 1, nodes: structuredClone(nodes) };
  state.selection = null;
  delete state.__pendingSelectionIds;
  syncDirtyBaseline();
  resetHistory();
}

initializeHistory();

describe("document history", () => {
  beforeEach(() => reset([]));

  it("undoes and redoes one semantic mutation while dirty follows the persisted baseline", () => {
    reset([leaf("A")]);
    const definition = catalog.find((item) => item.block === "test/leaf");

    assert.equal(state.dirty, false);
    assert.equal(insertBlock({ definition, parentId: null, index: 1 }).ok, true);
    assert.equal(state.document.nodes.length, 2);
    assert.equal(state.dirty, true);
    assert.equal(canUndo(), true);
    assert.equal(canRedo(), false);

    assert.equal(undo(), true);
    assert.deepEqual(state.document.nodes.map((node) => node.id), ["A"]);
    assert.equal(state.dirty, false);
    assert.equal(canRedo(), true);

    assert.equal(redo(), true);
    assert.equal(state.document.nodes.length, 2);
    assert.equal(state.dirty, true);
  });

  it("restores an exact move, duplicate, delete chain and preserves duplicate IDs on redo", () => {
    reset([leaf("A"), leaf("B"), leaf("C")]);
    const originalJSON = JSON.stringify(state.document);

    assert.equal(moveNode({ nodeId: "C", parentId: null, index: 0 }).ok, true);
    const duplicateResult = duplicateNode({ nodeId: "B" });
    assert.equal(duplicateResult.ok, true);
    const duplicatedId = duplicateResult.node.id;
    assert.equal(deleteNode({ nodeId: "A" }).ok, true);
    const finalJSON = JSON.stringify(state.document);

    assert.equal(undo(), true);
    assert.equal(undo(), true);
    assert.equal(undo(), true);
    assert.equal(JSON.stringify(state.document), originalJSON);
    assert.equal(canUndo(), false);

    assert.equal(redo(), true);
    assert.equal(redo(), true);
    assert.equal(redo(), true);
    assert.equal(JSON.stringify(state.document), finalJSON);
    assert.equal(state.document.nodes.some((node) => node.id === duplicatedId), true);
  });

  it("invalidates redo after a new edit", () => {
    reset([leaf("A")]);
    const definition = catalog.find((item) => item.block === "test/leaf");
    assert.equal(insertBlock({ definition, parentId: null, index: 1 }).ok, true);
    assert.equal(undo(), true);
    assert.equal(canRedo(), true);

    const inserted = insertBlock({ definition, parentId: null, index: 1 });
    assert.equal(inserted.ok, true);
    assert.equal(canRedo(), false);
    assert.equal(redo(), false);
    assert.deepEqual(state.document.nodes.map((node) => node.id), ["A", inserted.node.id]);
  });

  it("does not record no-ops or rejected commands", () => {
    reset([{ id: "parent", block: "test/limited-container", version: 1, props: {}, settings: {}, children: [leaf("only")] }]);
    const before = JSON.stringify(state.document);
    assert.equal(duplicateNode({ nodeId: "only" }).ok, false);
    assert.equal(canUndo(), false);

    setDocument(structuredClone(state.document), { renderHint: "refresh" });
    assert.equal(canUndo(), false);
    assert.equal(JSON.stringify(state.document), before);
  });

  it("notifies subscribers when undo and redo availability changes", () => {
    reset([leaf("A")]);
    const states = [];
    const unsubscribe = subscribeHistory((snapshot) => states.push(snapshot));
    const definition = catalog.find((item) => item.block === "test/leaf");
    insertBlock({ definition, parentId: null, index: 1 });
    undo();
    redo();
    unsubscribe();

    assert.deepEqual(states, [
      { canUndo: false, canRedo: false },
      { canUndo: true, canRedo: false },
      { canUndo: false, canRedo: true },
      { canUndo: true, canRedo: false },
    ]);
  });

  it("keeps at most fifty undo steps", () => {
    reset([]);
    for (let index = 1; index <= 55; index++) {
      setDocument({ version: 1, nodes: Array.from({ length: index }, (_, item) => leaf(`node-${item}`)) });
    }

    for (let index = 0; index < 50; index++) assert.equal(undo(), true);
    assert.equal(undo(), false);
    assert.equal(state.document.nodes.length, 5);
  });
});
