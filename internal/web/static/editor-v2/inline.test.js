// inline.test.js — M5A updateNodeField + inline helpers
import { describe, it, beforeEach } from "node:test";
import assert from "node:assert/strict";

const catalog = JSON.parse('[{"block": "core/heading", "version": 2, "displayName": "Heading", "description": "Heading", "schema": {"props": {"type": "object", "required": ["text"], "properties": {"text": {"type": "object", "default": {"version": 1, "content": []}, "required": ["version", "content"], "properties": {"version": {"type": "integer", "enum": [1]}, "content": {"type": "array", "items": {"type": "object", "required": ["text"], "properties": {"text": {"type": "string"}}}}}}, "level": {"type": "integer", "enum": [1, 2, 3], "default": 2}}}, "settings": {"type": "object", "properties": {"align": {"type": "string", "enum": ["left", "center"], "default": "left"}}}, "children": {"mode": "none"}, "editor": {"category": "text", "fields": {"props.text": {"label": "Text", "control": "richtext", "inline": true, "inlineMode": "plain"}, "props.level": {"label": "Level", "control": "select"}}, "summaryFields": ["props.text"]}}}, {"block": "core/text", "version": 2, "displayName": "Text", "schema": {"props": {"type": "object", "required": ["text"], "properties": {"text": {"type": "object", "default": {"version": 1, "content": []}, "required": ["version", "content"], "properties": {"version": {"type": "integer", "enum": [1]}, "content": {"type": "array", "items": {"type": "object", "required": ["text"], "properties": {"text": {"type": "string"}}}}}}}}, "settings": {"type": "object", "properties": {}}, "children": {"mode": "none"}, "editor": {"category": "text", "fields": {"props.text": {"label": "Text", "control": "richtext", "inline": true, "inlineMode": "plain"}}, "summaryFields": ["props.text"]}}}, {"block": "core/button", "version": 1, "displayName": "Button", "schema": {"props": {"type": "object", "required": ["label", "url"], "properties": {"label": {"type": "string", "default": "Button"}, "url": {"type": "string", "default": "#"}}}, "settings": {"type": "object", "properties": {"variant": {"type": "string", "enum": ["primary", "secondary"], "default": "primary"}}}, "children": {"mode": "none"}, "editor": {"category": "design", "fields": {"props.label": {"label": "Label", "control": "text", "inline": true, "inlineMode": "plain"}, "props.url": {"label": "URL", "control": "text"}}}}}, {"block": "core/section", "version": 1, "displayName": "Section", "schema": {"props": {"type": "object", "properties": {}}, "settings": {"type": "object", "properties": {}}, "children": {"mode": "any"}, "editor": {"category": "layout"}}}, {"block": "core/image", "version": 1, "displayName": "Image", "schema": {"props": {"type": "object", "properties": {"mediaId": {"type": "string", "default": ""}}}, "settings": {"type": "object", "properties": {}}, "children": {"mode": "none"}, "editor": {"category": "media", "fields": {"props.mediaId": {"label": "Image", "control": "media"}}}}}]');

globalThis.document = {
  getElementById: (id) => {
    if (id === "editor-v2-bootstrap") {
      return { textContent: JSON.stringify({ document: { version: 1, nodes: [] }, catalog, definitions: catalog, resource: { id: "e1", type: "entry" }, previewUrl: "/admin/editor/preview" }) };
    }
    return null;
  },
  createElement: () => ({ style: {}, setAttribute: () => {}, append: () => {}, getAttribute: () => null }),
  querySelector: () => null,
  querySelectorAll: () => [],
};
globalThis.window = globalThis;
globalThis.window.location = { origin: "http://localhost", search: "" };
globalThis.location = globalThis.window.location;
try {
  if (!globalThis.crypto || !globalThis.crypto.getRandomValues) {
    globalThis.crypto = { getRandomValues(arr) { for (let i=0;i<arr.length;i++) arr[i]=Math.floor(Math.random()*256); return arr; } };
  }
} catch (_) {
  try { globalThis.crypto.getRandomValues = (arr) => { for (let i=0;i<arr.length;i++) arr[i]=Math.floor(Math.random()*256); return arr; }; } catch (_) {}
}

const stateMod = await import("./state.js");
const commandsMod = await import("./commands.js");
const { state, isPlainRichTextValue, plainTextFromRichText, inlineFieldsForNode, primaryInlineFieldForNode, isInlineEditableNode } = stateMod;
const { updateNodeField, createNode, insertBlock } = commandsMod;

function resetDoc(nodes = []) {
  state.document = { version: 1, nodes: JSON.parse(JSON.stringify(nodes)) };
  const walk = (list) => { for (const n of list) { n.children ||= []; walk(n.children); } };
  walk(state.document.nodes);
  state.selection = null;
  state.editing = null;
  stateMod.syncDirtyBaseline();
}

describe("updateNodeField command", () => {
  beforeEach(() => resetDoc([]));
  it("update valid props field (button label)", () => {
    const btnDef = catalog.find(c => c.block === "core/button");
    const node = createNode(btnDef);
    node.id = "b1";
    resetDoc([node]);
    const res = updateNodeField({ nodeId: "b1", path: "props.label", value: "View guide" });
    assert.equal(res.ok, true);
    assert.equal(res.value, "View guide");
    assert.equal(state.document.nodes[0].props.label, "View guide");
    assert.equal(state.dirty, true);
  });
  it("update valid richtext heading plain", () => {
    const hDef = catalog.find(c => c.block === "core/heading");
    const node = createNode(hDef);
    node.id = "h1";
    node.props.text = { version: 1, content: [{ text: "Old" }] };
    resetDoc([node]);
    const res = updateNodeField({ nodeId: "h1", path: "props.text", value: "New heading" });
    assert.equal(res.ok, true);
    assert.equal(res.value.content[0].text, "New heading");
    assert.equal(state.document.nodes[0].props.text.content[0].text, "New heading");
  });
  it("unknown node", () => {
    const btnDef = catalog.find(c => c.block === "core/button");
    const node = createNode(btnDef); node.id="b1"; resetDoc([node]);
    const res = updateNodeField({ nodeId: "missing", path: "props.label", value: "X" });
    assert.equal(res.ok, false);
    assert.match(res.reason, /Block not found/);
  });
  it("unknown field path", () => {
    const btnDef = catalog.find(c => c.block === "core/button");
    const node = createNode(btnDef); node.id="b1"; resetDoc([node]);
    const res = updateNodeField({ nodeId: "b1", path: "props.unknown", value: "X" });
    assert.equal(res.ok, false);
    assert.match(res.reason, /Unknown field/);
  });
  it("dangerous prototype path rejected", () => {
    const btnDef = catalog.find(c => c.block === "core/button");
    const node = createNode(btnDef); node.id="b1"; resetDoc([node]);
    const res = updateNodeField({ nodeId: "b1", path: "__proto__.x", value: "polluted" });
    assert.equal(res.ok, false);
    assert.match(res.reason, /Invalid field/);
  });
  it("value unchanged does not dirty", () => {
    const btnDef = catalog.find(c => c.block === "core/button");
    const node = createNode(btnDef); node.id="b1"; node.props.label="Hello"; resetDoc([node]);
    const res = updateNodeField({ nodeId: "b1", path: "props.label", value: "Hello" });
    assert.equal(res.ok, true);
    assert.equal(res.unchanged, true);
    assert.equal(state.dirty, false);
  });
  it("sibling nodes unchanged", () => {
    const hDef = catalog.find(c => c.block === "core/heading");
    const bDef = catalog.find(c => c.block === "core/button");
    const h = createNode(hDef); h.id="h1"; h.props.text={version:1,content:[{text:"H"}]};
    const b = createNode(bDef); b.id="b1"; b.props.label="Btn";
    resetDoc([h,b]);
    const res = updateNodeField({ nodeId: "h1", path: "props.text", value: "New" });
    assert.equal(res.ok, true);
    assert.equal(state.document.nodes[1].props.label, "Btn");
  });
  it("single-line newline stripping", () => {
    const bDef = catalog.find(c => c.block === "core/button");
    const node = createNode(bDef); node.id="b1"; resetDoc([node]);
    const res = updateNodeField({ nodeId: "b1", path: "props.label", value: "Hello\nWorld\r\nTest" });
    assert.equal(res.ok, true);
    assert.equal(res.value, "Hello World Test");
  });
  it("empty richtext commit", () => {
    const hDef = catalog.find(c => c.block === "core/heading");
    const node = createNode(hDef); node.id="h1"; node.props.text={version:1,content:[{text:"Old"}]}; resetDoc([node]);
    const res = updateNodeField({ nodeId: "h1", path: "props.text", value: "" });
    assert.equal(res.ok, true);
    assert.equal(res.value.content.length, 0);
  });
});
describe("inline helpers", () => {
  it("isPlainRichTextValue plain vs rich", () => {
    assert.equal(isPlainRichTextValue({version:1, content:[]}), true);
    assert.equal(isPlainRichTextValue({version:1, content:[{text:"hello"}]}), true);
    assert.equal(isPlainRichTextValue({version:1, content:[{text:"hi", marks:[{type:"bold"}]}]}), false);
  });
  it("plainTextFromRichText join", () => {
    assert.equal(plainTextFromRichText({version:1, content:[{text:"a"}, {text:"b"}]}), "ab");
  });
  it("inlineFieldsForNode heading plain", () => {
    const hDef = catalog.find(c=>c.block==="core/heading");
    const node = createNode(hDef); node.props.text={version:1, content:[{text:"hi"}]};
    assert.deepEqual(inlineFieldsForNode(node), ["props.text"]);
  });
  it("inlineFieldsForNode heading with marks not editable", () => {
    const hDef = catalog.find(c=>c.block==="core/heading");
    const node = createNode(hDef); node.props.text={version:1, content:[{text:"hi", marks:[{type:"bold"}]}]};
    assert.deepEqual(inlineFieldsForNode(node), []);
  });
  it("primaryInlineFieldForNode single", () => {
    const bDef = catalog.find(c=>c.block==="core/button");
    const node = createNode(bDef);
    assert.equal(primaryInlineFieldForNode(node), "props.label");
  });
  it("isInlineEditableNode false for image", () => {
    const imgDef = catalog.find(c=>c.block==="core/image");
    const node = createNode(imgDef);
    assert.equal(isInlineEditableNode(node), false);
  });
  it("isInlineEditableNode true for button", () => {
    const bDef = catalog.find(c=>c.block==="core/button");
    const node = createNode(bDef);
    assert.equal(isInlineEditableNode(node), true);
  });
});
describe("document change metadata", () => {
  it("inline update can emit deferred render hint", () => {
    const hDef = catalog.find(c=>c.block==="core/heading");
    const node = createNode(hDef); node.id="h1"; node.props.text={version:1,content:[{text:"Old"}]}; resetDoc([node]);
    let receivedHint = null;
    let receivedDoc = null;
    const unsub = stateMod.subscribeDocument((doc, meta) => { receivedDoc = doc; receivedHint = meta?.renderHint; });
    const res = updateNodeField({nodeId:"h1", path:"props.text", value:"New", renderHint:"defer"});
    assert.equal(res.ok, true);
    assert.equal(receivedHint, "defer");
    assert.ok(receivedDoc);
    assert.equal(receivedDoc.nodes[0].props.text.content[0].text, "New");
    unsub();
  });
  it("insertion still requests normal render", () => {
    const hDef = catalog.find(c=>c.block==="core/heading");
    resetDoc([]);
    let hint = null;
    const unsub = stateMod.subscribeDocument((doc, meta) => { hint = meta?.renderHint; });
    const res = insertBlock({definition: hDef, parentId: null, index:0});
    assert.equal(res.ok, true);
    assert.equal(hint, "refresh");
    unsub();
  });
  it("document subscribers still receive inline update", () => {
    const hDef = catalog.find(c=>c.block==="core/heading");
    const node = createNode(hDef); node.id="h1"; node.props.text={version:1,content:[{text:"A"}]}; resetDoc([node]);
    let notified = false;
    let hint = null;
    const unsub = stateMod.subscribeDocument((doc, meta) => { notified = true; hint = meta?.renderHint; });
    const res = updateNodeField({nodeId:"h1", path:"props.text", value:"B", renderHint:"defer"});
    assert.equal(notified, true);
    assert.equal(hint, "defer");
    unsub();
  });
  it("unchanged commit causes no refresh", () => {
    const bDef = catalog.find(c=>c.block==="core/button");
    const node = createNode(bDef); node.id="b1"; node.props.label="Same"; resetDoc([node]);
    let notified = false;
    const unsub = stateMod.subscribeDocument(() => { notified = true; });
    const res = updateNodeField({nodeId:"b1", path:"props.label", value:"Same"});
    assert.equal(res.ok, true);
    assert.equal(res.unchanged, true);
    assert.equal(notified, false);
    unsub();
  });
});
