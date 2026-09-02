import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

const dom = new JSDOM(`<!DOCTYPE html>`, { pretendToBeVisual: true });
globalThis.DOMParser = dom.window.DOMParser;
globalThis.document = dom.window.document;
globalThis.Node = dom.window.Node;
globalThis.NodeFilter = dom.window.NodeFilter;
globalThis.CustomEvent = dom.window.CustomEvent;

const { parsePreviewDocument, patchPreviewDocument } = await import("./preview-morph.js");

function makeDoc(html) {
  const d = parsePreviewDocument(html);
  assert.ok(d);
  return d;
}

// Helper to emit html with markers for nested structure
function htmlForNested(moveChild) {
  // moveChild = false => original: Parent A [Child X[Grandchild Y]], Parent B empty
  // moveChild = true => Parent A empty, Parent B [Child X[Grandchild Y]]
  if (!moveChild) {
    return `<!doctype html><html><head></head><body>
<!-- stratum-node-start:parentA:root/node:parentA:true:core/section:1 --><div data-stratum-key="parentA::root/node:parentA" id="pa">ParentA
  <!-- stratum-node-start:childX:root/node:parentA/node:childX:true:core/stack:1 --><div data-stratum-key="childX::root/node:parentA/node:childX" id="cx">ChildX
    <!-- stratum-node-start:grandY:root/node:parentA/node:childX/node:grandY:true:core/text:1 --><p data-stratum-key="grandY::root/node:parentA/node:childX/node:grandY" id="gy">GrandY</p><!-- stratum-node-end:grandY:root/node:parentA/node:childX/node:grandY -->
  </div><!-- stratum-node-end:childX:root/node:parentA/node:childX -->
</div><!-- stratum-node-end:parentA:root/node:parentA -->
<!-- stratum-node-start:parentB:root/node:parentB:true:core/section:1 --><div data-stratum-key="parentB::root/node:parentB" id="pb">ParentB</div><!-- stratum-node-end:parentB:root/node:parentB -->
</body></html>`;
  } else {
    return `<!doctype html><html><head></head><body>
<!-- stratum-node-start:parentA:root/node:parentA:true:core/section:1 --><div data-stratum-key="parentA::root/node:parentA" id="pa">ParentA</div><!-- stratum-node-end:parentA:root/node:parentA -->
<!-- stratum-node-start:parentB:root/node:parentB:true:core/section:1 --><div data-stratum-key="parentB::root/node:parentB" id="pb">ParentB
  <!-- stratum-node-start:childX:root/node:parentB/node:childX:true:core/stack:1 --><div data-stratum-key="childX::root/node:parentB/node:childX" id="cx">ChildX
    <!-- stratum-node-start:grandY:root/node:parentB/node:childX/node:grandY:true:core/text:1 --><p data-stratum-key="grandY::root/node:parentB/node:childX/node:grandY" id="gy">GrandY</p><!-- stratum-node-end:grandY:root/node:parentB/node:childX/node:grandY -->
  </div><!-- stratum-node-end:childX:root/node:parentB/node:childX -->
</div><!-- stratum-node-end:parentB:root/node:parentB -->
</body></html>`;
  }
}

describe("preview-morph nested cross-parent subtree (M6 §16)", () => {
  it("reparents Child X with Grandchild Y from Parent A to Parent B", () => {
    const oldDoc = makeDoc(htmlForNested(false));
    const newDoc = makeDoc(htmlForNested(true));
    const oldPA = oldDoc.getElementById("pa");
    const oldPB = oldDoc.getElementById("pb");
    const oldCX = oldDoc.getElementById("cx");
    const oldGY = oldDoc.getElementById("gy");
    assert.ok(oldPA && oldPB && oldCX && oldGY);
    const patched = patchPreviewDocument(oldDoc, newDoc);
    assert.equal(patched, true);
    // After patch, ParentA should be empty (no ChildX)
    const newPA = oldDoc.getElementById("pa");
    const newPB = oldDoc.getElementById("pb");
    const newCX = oldDoc.getElementById("cx");
    const newGY = oldDoc.getElementById("gy");
    assert.ok(newPA && newPB && newCX && newGY);
    assert.equal(newPA.contains(newCX), false, "ParentA should not contain ChildX after move");
    assert.equal(newPB.contains(newCX), true, "ParentB should contain ChildX after move");
    assert.equal(newCX.contains(newGY), true, "ChildX should still contain GrandY");
    // Check order: ParentB's child should be ChildX
    assert.equal(newPB.firstElementChild, newCX);
  });

  it("moves Parent A itself with all descendants to new position", () => {
    const htmlBefore = `<!doctype html><html><head></head><body>
<!-- stratum-node-start:parentA:root/node:parentA:true:core/section:1 --><div data-stratum-key="parentA::root/node:parentA" id="pa">A<div data-stratum-key="childX::root/node:parentA/node:childX" id="cx"><!-- stratum-node-start:childX:root/node:parentA/node:childX:true:core/stack:1 -->ChildX<!-- stratum-node-end:childX:root/node:parentA/node:childX --></div></div><!-- stratum-node-end:parentA:root/node:parentA -->
<!-- stratum-node-start:parentB:root/node:parentB:true:core/section:1 --><div data-stratum-key="parentB::root/node:parentB" id="pb">B</div><!-- stratum-node-end:parentB:root/node:parentB -->
</body></html>`;
    const htmlAfter = `<!doctype html><html><head></head><body>
<!-- stratum-node-start:parentB:root/node:parentB:true:core/section:1 --><div data-stratum-key="parentB::root/node:parentB" id="pb">B</div><!-- stratum-node-end:parentB:root/node:parentB -->
<!-- stratum-node-start:parentA:root/node:parentA:true:core/section:1 --><div data-stratum-key="parentA::root/node:parentA" id="pa">A<div data-stratum-key="childX::root/node:parentA/node:childX" id="cx"><!-- stratum-node-start:childX:root/node:parentA/node:childX:true:core/stack:1 -->ChildX<!-- stratum-node-end:childX:root/node:parentA/node:childX --></div></div><!-- stratum-node-end:parentA:root/node:parentA -->
</body></html>`;
    const oldDoc = makeDoc(htmlBefore);
    const newDoc = makeDoc(htmlAfter);
    const oldPA = oldDoc.getElementById("pa");
    const oldCX = oldDoc.getElementById("cx");
    patchPreviewDocument(oldDoc, newDoc);
    const children = Array.from(oldDoc.body.children).filter(n=>n.id);
    assert.deepEqual(children.map(c=>c.id), ["pb","pa"]);
    assert.ok(oldDoc.getElementById("pa").contains(oldDoc.getElementById("cx")));
  });
});
