import { describe, it } from "node:test";
import assert from "node:assert/strict";

globalThis.document = {
  getElementById: (id) => {
    if (id === "editor-v2-bootstrap") return { textContent: JSON.stringify({ document: { version: 1, nodes: [] }, catalog: [], definitions: [], resource: { id: "e1", type: "entry" }, previewUrl: "/admin/editor/preview" }) };
    return null;
  },
  createElement: () => ({ style: {}, setAttribute: () => {}, append: () => {} }),
  querySelector: () => null,
  querySelectorAll: () => [],
  body: {},
  documentElement: {},
  createTreeWalker: () => ({ nextNode: () => null }),
};
globalThis.window = globalThis;
globalThis.window.location = { origin: "http://localhost", search: "" };
globalThis.location = globalThis.window.location;

const { parseStratumMarker, parseStartComment, parseEndComment } = await import("./markers.js");

describe("parseStratumMarker shared parser (M6 §7)", () => {
  it("START with instanceKey containing ':' is parsed correctly", () => {
    const data = "stratum-node-start:blk_123:root/node:blk_123:true:core/text:1";
    const parsed = parseStratumMarker(data);
    assert.ok(parsed);
    assert.equal(parsed.kind, "start");
    assert.equal(parsed.nodeId, "blk_123");
    assert.equal(parsed.instanceKey, "root/node:blk_123");
    assert.equal(parsed.editable, true);
    assert.equal(parsed.block, "core/text");
    assert.equal(parsed.version, 1);
    // via individual parsers
    const s = parseStartComment(data);
    assert.equal(s.instanceKey, "root/node:blk_123");
  });

  it("END with instanceKey containing ':' and multiple colons", () => {
    const data = "stratum-node-end:blk_123:root/node:blk_123";
    const parsed = parseStratumMarker(data);
    assert.ok(parsed);
    assert.equal(parsed.kind, "end");
    assert.equal(parsed.nodeId, "blk_123");
    assert.equal(parsed.instanceKey, "root/node:blk_123");
    const e = parseEndComment(data);
    assert.equal(e.instanceKey, "root/node:blk_123");
  });

  it("START+END with instanceKey containing multiple ':' characters (regression)", () => {
    const instanceKey = "root/node:blk_123:extra:part:with:colons";
    const encodedKey = encodeURIComponent(instanceKey);
    // Simulate Go's PathEscape: "/" -> %2F, ":" -> %3A etc. But test also checks raw colon handling for legacy unescaped.
    const startRaw = `stratum-node-start:blk_999:${instanceKey}:true:core/section:1`;
    const endRaw = `stratum-node-end:blk_999:${instanceKey}`;
    const start = parseStratumMarker(startRaw);
    const end = parseStratumMarker(endRaw);
    assert.ok(start);
    assert.ok(end);
    assert.equal(start.kind, "start");
    assert.equal(end.kind, "end");
    assert.equal(start.nodeId, "blk_999");
    assert.equal(end.nodeId, "blk_999");
    assert.equal(start.instanceKey, instanceKey);
    assert.equal(end.instanceKey, instanceKey);
    // Ensure shared parser does not truncate at first colon (bug would have returned "root/node")
    assert.notEqual(end.instanceKey, "root/node");
    assert.equal(end.instanceKey.split(":").length, 6); // root/node + 5 extra segments
  });

  it("START with multiple colons encoded via %3A", () => {
    const instanceKey = "root/node:blk:123:abc";
    const encoded = encodeURIComponent(instanceKey); // root%2Fnode%3Ablk%3A123%3Aabc
    const data = `stratum-node-start:blk_1:${encoded}:false:core/accordion:1`;
    const parsed = parseStratumMarker(data);
    assert.ok(parsed);
    assert.equal(parsed.instanceKey, instanceKey);
    assert.equal(parsed.editable, false);
  });

  it("END encoded with %3A preserves full key", () => {
    const instanceKey = "root/node:blk:999:extra:colon:test";
    const encoded = encodeURIComponent(instanceKey);
    const data = `stratum-node-end:blk_1:${encoded}`;
    const parsed = parseStratumMarker(data);
    assert.ok(parsed);
    assert.equal(parsed.kind, "end");
    assert.equal(parsed.instanceKey, instanceKey);
  });
});
