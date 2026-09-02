import { describe, it, before } from "node:test";
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

globalThis.document = {
  getElementById: (id) => {
    if (id === "editor-v2-bootstrap") return { textContent: JSON.stringify({ document: { version:1, nodes:[] }, catalog:[], definitions:[], resource:{id:"e1",type:"entry"}, previewUrl:"/admin/editor/preview" }) };
    return null;
  },
  createElement: () => ({ style:{}, setAttribute:()=>{}, append:()=>{} }),
  querySelector: ()=>null,
  querySelectorAll: ()=>[],
  body: {},
  documentElement: {},
  createTreeWalker: ()=>({ nextNode:()=>null }),
};
globalThis.window = globalThis;
globalThis.window.location = { origin:"http://localhost", search:"" };
globalThis.location = globalThis.window.location;

let buildMarkerIndex;
before(async () => {
  const mod = await import("./markers.js");
  buildMarkerIndex = mod.buildMarkerIndex;
});

describe("markers deepest wins for Accordion Item summary", () => {
  it("summary maps to accordion-item, not accordion", () => {
    const html = `<!DOCTYPE html><body>
      <!-- stratum-node-start:acc:scope%2Fnode%3Aacc:true:core%2Faccordion:1 -->
      <div class="stratum-accordion">
        <!-- stratum-node-start:item1:scope%2Fnode%3Aacc%2Fnode%3Aitem1:true:core%2Faccordion-item:1 -->
        <details class="stratum-accordion-item"><summary class="stratum-accordion-trigger">Title</summary><div class="stratum-accordion-content">Hello</div></details>
        <!-- stratum-node-end:item1:scope%2Fnode%3Aacc%2Fnode%3Aitem1 -->
        <!-- stratum-node-start:item2:scope%2Fnode%3Aacc%2Fnode%3Aitem2:true:core%2Faccordion-item:1 -->
        <details class="stratum-accordion-item"><summary class="stratum-accordion-trigger">Second</summary><div>World</div></details>
        <!-- stratum-node-end:item2:scope%2Fnode%3Aacc%2Fnode%3Aitem2 -->
      </div>
      <!-- stratum-node-end:acc:scope%2Fnode%3Aacc -->
    </body>`;
    const dom = new JSDOM(html);
    const doc = dom.window.document;
    // Polyfill Node/NodeFilter for markers.js which checks global Node
    globalThis.Node = dom.window.Node;
    globalThis.NodeFilter = dom.window.NodeFilter;
    global.Node = dom.window.Node;
    global.NodeFilter = dom.window.NodeFilter;

    const { index, elementToNode } = buildMarkerIndex(doc);
    const summary = doc.querySelector("summary.stratum-accordion-trigger");
    assert.ok(summary, "summary found");
    // Walk up to find mapping like hitForTarget does
    let el = summary;
    let hit = null;
    while (el) {
      if (elementToNode.has(el)) { hit = elementToNode.get(el); break; }
      if (el === doc.documentElement) break;
      el = el.parentElement;
    }
    assert.ok(hit, "hit found for summary");
    assert.equal(hit.block, "core/accordion-item");
    assert.equal(hit.nodeId, "item1");

    // Also test chevron area: text node inside summary
    const textNode = summary.firstChild;
    let el2 = textNode;
    if (el2.nodeType === 3) el2 = el2.parentElement;
    let hit2 = null;
    while (el2) {
      if (elementToNode.has(el2)) { hit2 = elementToNode.get(el2); break; }
      el2 = el2.parentElement;
    }
    assert.equal(hit2.block, "core/accordion-item");

    // Outer accordion div should map to accordion
    const accordionDiv = doc.querySelector("div.stratum-accordion");
    let hit3 = null;
    let el3 = accordionDiv;
    while (el3) {
      if (elementToNode.has(el3)) { hit3 = elementToNode.get(el3); break; }
      el3 = el3.parentElement;
    }
    assert.equal(hit3.block, "core/accordion");
    assert.equal(hit3.nodeId, "acc");

    // Details element itself should map to accordion-item
    const details = doc.querySelector("details");
    let hit4 = null;
    let el4 = details;
    while (el4) {
      if (elementToNode.has(el4)) { hit4 = elementToNode.get(el4); break; }
      el4 = el4.parentElement;
    }
    assert.equal(hit4.block, "core/accordion-item");

    // Clean up globals
    delete global.Node;
    delete global.NodeFilter;
    delete globalThis.Node;
    delete globalThis.NodeFilter;
  });
});
