// preview-morph.test.js — tests for DOM morphing (head/body, keys, markers)
import { describe, it, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

// Setup global DOMParser for preview-morph
const dom = new JSDOM(`<!DOCTYPE html>`, { pretendToBeVisual: true });
globalThis.DOMParser = dom.window.DOMParser;
globalThis.document = dom.window.document;
globalThis.Node = dom.window.Node;
globalThis.NodeFilter = dom.window.NodeFilter;
globalThis.CustomEvent = dom.window.CustomEvent;

const {
  parsePreviewDocument,
  patchPreviewDocument,
  getNodeKey,
  annotatePreviewDocument,
} = await import("./preview-morph.js");

function makeDoc(html) {
  const d = parsePreviewDocument(html);
  assert.ok(d, "parse should succeed");
  return d;
}

describe("preview-morph attribute update", () => {
  it("preserves same DOM node and updates class", () => {
    const oldHtml = `<!doctype html><html><head><title>T</title></head><body><div data-stratum-key="a" class="old"></div></body></html>`;
    const newHtml = `<!doctype html><html><head><title>T</title></head><body><div data-stratum-key="a" class="new"></div></body></html>`;
    const oldDoc = makeDoc(oldHtml);
    const newDoc = makeDoc(newHtml);
    const oldNode = oldDoc.body.firstChild;
    assert.equal(oldNode.getAttribute("class"), "old");
    const patched = patchPreviewDocument(oldDoc, newDoc);
    assert.equal(patched, true);
    // Should be same node instance
    assert.equal(oldDoc.body.firstChild, oldNode, "same node preserved");
    assert.equal(oldNode.getAttribute("class"), "new");
  });
});

describe("preview-morph text update", () => {
  it("preserves element while text changes", () => {
    const oldHtml = `<!doctype html><html><head></head><body><p data-stratum-key="p1">Hello</p></body></html>`;
    const newHtml = `<!doctype html><html><head></head><body><p data-stratum-key="p1">Hello world</p></body></html>`;
    const oldDoc = makeDoc(oldHtml);
    const newDoc = makeDoc(newHtml);
    const oldP = oldDoc.body.firstChild;
    patchPreviewDocument(oldDoc, newDoc);
    assert.equal(oldDoc.body.firstChild, oldP);
    assert.equal(oldP.textContent, "Hello world");
  });
});

describe("preview-morph add/remove child", () => {
  it("handles added node", () => {
    const oldHtml = `<!doctype html><html><head></head><body><div data-stratum-key="a">A</div></body></html>`;
    const newHtml = `<!doctype html><html><head></head><body><div data-stratum-key="a">A</div><div data-stratum-key="b">B</div></body></html>`;
    const oldDoc = makeDoc(oldHtml);
    const newDoc = makeDoc(newHtml);
    patchPreviewDocument(oldDoc, newDoc);
    assert.equal(oldDoc.body.childNodes.length, 2);
    assert.equal(oldDoc.body.lastChild.getAttribute("data-stratum-key"), "b");
    assert.equal(oldDoc.body.lastChild.textContent, "B");
  });

  it("handles removed node", () => {
    const oldHtml = `<!doctype html><html><head></head><body><div data-stratum-key="a">A</div><div data-stratum-key="b">B</div></body></html>`;
    const newHtml = `<!doctype html><html><head></head><body><div data-stratum-key="a">A</div></body></html>`;
    const oldDoc = makeDoc(oldHtml);
    const newDoc = makeDoc(newHtml);
    patchPreviewDocument(oldDoc, newDoc);
    assert.equal(oldDoc.body.childNodes.length, 1);
    assert.equal(oldDoc.body.firstChild.getAttribute("data-stratum-key"), "a");
  });
});

describe("preview-morph reorder keyed children", () => {
  it("moves nodes, not recreates", () => {
    const oldHtml = `<!doctype html><html><head></head><body><div data-stratum-key="a">A</div><div data-stratum-key="b">B</div><div data-stratum-key="c">C</div></body></html>`;
    const newHtml = `<!doctype html><html><head></head><body><div data-stratum-key="c">C</div><div data-stratum-key="a">A</div><div data-stratum-key="b">B</div></body></html>`;
    const oldDoc = makeDoc(oldHtml);
    const newDoc = makeDoc(newHtml);
    const aNode = oldDoc.body.childNodes[0];
    const bNode = oldDoc.body.childNodes[1];
    const cNode = oldDoc.body.childNodes[2];
    patchPreviewDocument(oldDoc, newDoc);
    assert.equal(oldDoc.body.childNodes.length, 3);
    // Check that nodes were moved, not recreated
    assert.equal(oldDoc.body.childNodes[0], cNode);
    assert.equal(oldDoc.body.childNodes[1], aNode);
    assert.equal(oldDoc.body.childNodes[2], bNode);
    assert.equal(oldDoc.body.textContent.replace(/\s+/g, ""), "CAB");
  });
});

describe("preview-morph marker comments", () => {
  it("preserves stratum start/end comments and order", () => {
    const oldHtml = `<!doctype html><html><head></head><body><!-- stratum-node-start:blk1:root/node:blk1:true:core/text:1 --><p>Text</p><!-- stratum-node-end:blk1:root/node:blk1 --></body></html>`;
    const newHtml = `<!doctype html><html><head></head><body><!-- stratum-node-start:blk1:root/node:blk1:true:core/text:1 --><p>Text updated</p><!-- stratum-node-end:blk1:root/node:blk1 --></body></html>`;
    const oldDoc = makeDoc(oldHtml);
    const newDoc = makeDoc(newHtml);
    const oldCommentStart = Array.from(oldDoc.body.childNodes).find(n => n.nodeType === 8);
    patchPreviewDocument(oldDoc, newDoc);
    const newCommentStart = Array.from(oldDoc.body.childNodes).find(n => n.nodeType === 8);
    assert.equal(newCommentStart, oldCommentStart, "comment preserved");
    assert.equal(oldDoc.body.textContent.includes("Text updated"), true);
    // Ensure comments still present and ordered
    const comments = Array.from(oldDoc.body.childNodes).filter(n => n.nodeType === 8).map(n => n.data.trim());
    assert.ok(comments[0].startsWith("stratum-node-start:blk1"));
    assert.ok(comments[1].startsWith("stratum-node-end:blk1"));
  });

  it("preserves marker order after reorder", () => {
    const oldHtml = `<!doctype html><html><head></head><body><!-- stratum-node-start:a:root/node:a:true:core/text:1 --><p>A</p><!-- stratum-node-end:a:root/node:a --><!-- stratum-node-start:b:root/node:b:true:core/text:1 --><p>B</p><!-- stratum-node-end:b:root/node:b --></body></html>`;
    const newHtml = `<!doctype html><html><head></head><body><!-- stratum-node-start:b:root/node:b:true:core/text:1 --><p>B</p><!-- stratum-node-end:b:root/node:b --><!-- stratum-node-start:a:root/node:a:true:core/text:1 --><p>A</p><!-- stratum-node-end:a:root/node:a --></body></html>`;
    const oldDoc = makeDoc(oldHtml);
    const newDoc = makeDoc(newHtml);
    const oldStartA = Array.from(oldDoc.body.childNodes).find(n => n.nodeType===8 && String(n.data).includes("blk")===false && String(n.data).includes(":a:"));
    // Just check count
    patchPreviewDocument(oldDoc, newDoc);
    const html = oldDoc.body.innerHTML;
    // After reorder, b should come before a
    const idxB = html.indexOf("stratum-node-start:b");
    const idxA = html.indexOf("stratum-node-start:a");
    assert.ok(idxB < idxA, "b before a after reorder");
  });
});

describe("preview-morph stylesheet preservation", () => {
  it("preserves same link node", () => {
    const oldHtml = `<!doctype html><html><head><link rel="stylesheet" href="/theme.css"></head><body></body></html>`;
    const newHtml = `<!doctype html><html><head><link rel="stylesheet" href="/theme.css"></head><body></body></html>`;
    const oldDoc = makeDoc(oldHtml);
    const newDoc = makeDoc(newHtml);
    const oldLink = oldDoc.head.querySelector('link[href="/theme.css"]');
    patchPreviewDocument(oldDoc, newDoc);
    const newLink = oldDoc.head.querySelector('link[href="/theme.css"]');
    assert.equal(oldLink, newLink, "same link node preserved");
  });

  it("updates href when changed", () => {
    const oldHtml = `<!doctype html><html><head><link rel="stylesheet" href="/theme.css"></head><body></body></html>`;
    const newHtml = `<!doctype html><html><head><link rel="stylesheet" href="/theme2.css"></head><body></body></html>`;
    const oldDoc = makeDoc(oldHtml);
    const newDoc = makeDoc(newHtml);
    patchPreviewDocument(oldDoc, newDoc);
    assert.equal(oldDoc.head.querySelectorAll('link[rel="stylesheet"]').length, 1);
    assert.equal(oldDoc.head.querySelector('link').getAttribute("href"), "/theme2.css");
  });
});

describe("preview-morph dynamic style", () => {
  it("updates style content without duplicating", () => {
    const oldHtml = `<!doctype html><html><head><style id="stratum-preview-theme">.a{color:red}</style></head><body></body></html>`;
    const newHtml = `<!doctype html><html><head><style id="stratum-preview-theme">.a{color:blue}</style></head><body></body></html>`;
    const oldDoc = makeDoc(oldHtml);
    const newDoc = makeDoc(newHtml);
    const oldStyle = oldDoc.head.querySelector("style#stratum-preview-theme");
    patchPreviewDocument(oldDoc, newDoc);
    const styles = oldDoc.head.querySelectorAll("style#stratum-preview-theme");
    assert.equal(styles.length, 1, "no duplication");
    assert.equal(styles[0], oldStyle, "same node");
    assert.ok(styles[0].textContent.includes("blue"));
  });

  it("does not duplicate after many patches", () => {
    const base = `<!doctype html><html><head><link rel="stylesheet" href="/theme.css"><style id="stratum-preview-theme">.a{}</style><meta name="description" content="x"></head><body></body></html>`;
    let doc = makeDoc(base);
    for (let i = 0; i < 20; i++) {
      const next = makeDoc(`<!doctype html><html><head><link rel="stylesheet" href="/theme.css"><style id="stratum-preview-theme">.a{color:${i}}</style><meta name="description" content="x${i}"></head><body></body></html>`);
      patchPreviewDocument(doc, next);
    }
    assert.equal(doc.head.querySelectorAll('link[href="/theme.css"]').length, 1);
    assert.equal(doc.head.querySelectorAll('style#stratum-preview-theme').length, 1);
    assert.equal(doc.head.querySelectorAll('meta[name="description"]').length, 1);
  });
});

describe("preview-morph image preservation", () => {
  it("preserves same img node", () => {
    const oldHtml = `<!doctype html><html><head></head><body><figure><img src="/media/a.jpg" srcset="/media/a.jpg 1x"></figure></body></html>`;
    const newHtml = `<!doctype html><html><head></head><body><figure><img src="/media/a.jpg" srcset="/media/a.jpg 1x"></figure></body></html>`;
    const oldDoc = makeDoc(oldHtml);
    const newDoc = makeDoc(newHtml);
    const oldImg = oldDoc.body.querySelector("img");
    patchPreviewDocument(oldDoc, newDoc);
    const newImg = oldDoc.body.querySelector("img");
    assert.equal(oldImg, newImg, "img preserved when src unchanged");
  });

  it("updates img src when changed", () => {
    const oldHtml = `<!doctype html><html><head></head><body><img src="/a.jpg"></body></html>`;
    const newHtml = `<!doctype html><html><head></head><body><img src="/b.jpg"></body></html>`;
    const oldDoc = makeDoc(oldHtml);
    const newDoc = makeDoc(newHtml);
    const oldImg = oldDoc.body.querySelector("img");
    patchPreviewDocument(oldDoc, newDoc);
    // If same tag positionally, node preserved but src updated
    // Our morph will preserve node if same tag and update attributes
    const curImg = oldDoc.body.querySelector("img");
    // Either same node with updated src, or new node — both acceptable as long as src is /b.jpg and count 1
    assert.equal(curImg.getAttribute("src"), "/b.jpg");
    assert.equal(oldDoc.body.querySelectorAll("img").length, 1);
  });
});

describe("preview-morph annotation", () => {
  it("annotates block wrappers with data-stratum-key", () => {
    const html = `<!doctype html><html><head></head><body><!-- stratum-node-start:blk1:root/node:blk1:true:core/text:1 --><p>Hello</p><!-- stratum-node-end:blk1:root/node:blk1 --></body></html>`;
    const doc = makeDoc(html);
    annotatePreviewDocument(doc);
    const p = doc.body.querySelector("p");
    assert.ok(p.hasAttribute("data-stratum-key"), "annotated");
    assert.equal(p.getAttribute("data-stratum-key"), "blk1::root/node:blk1");
  });
});
