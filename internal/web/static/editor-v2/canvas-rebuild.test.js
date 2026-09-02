import { describe, it, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

// Behavioral regression for M6 Final Corrective #5-6:
// After structural move (Section → root) and preview morph, canvas marker index
// must be rebuilt so moved node is resolvable via nodeToKeys and hitForTarget.

const htmlBefore = `<!DOCTYPE html><html><body>
<!-- stratum-node-start:sec1:sec1:true:core/section:1 -->
<div data-stratum-root>
  <!-- stratum-node-start:txt1:txt1:true:core/text:1 -->
  <p>hello</p>
  <!-- stratum-node-end:txt1:txt1 -->
</div>
<!-- stratum-node-end:sec1:sec1 -->
</body></html>`;

const htmlAfter = `<!DOCTYPE html><html><body>
<!-- stratum-node-start:sec1:sec1:true:core/section:1 -->
<div data-stratum-root></div>
<!-- stratum-node-end:sec1:sec1 -->
<!-- stratum-node-start:txt1:txt1:true:core/text:1 -->
<p>hello</p>
<!-- stratum-node-end:txt1:txt1 -->
</body></html>`;

describe("canvas rebuild after morph", () => {
  it("rebuildIndex maps moved Text to root and hitForTarget resolves", async () => {
    // Setup JSDOM environment
    const dom = new JSDOM(htmlBefore, { pretendToBeVisual: true });
    globalThis.document = dom.window.document;
    globalThis.window = dom.window;
    globalThis.Node = dom.window.Node;
    globalThis.NodeFilter = dom.window.NodeFilter;
    globalThis.getComputedStyle = dom.window.getComputedStyle.bind(dom.window);

    // Import after globals are set (canvas imports state which reads document at import time)
    const { buildMarkerIndex } = await import("./markers.js");
    const { CanvasController } = await import("./canvas.js");

    // Build initial index via canvas
    const iframe = dom.window.document.createElement("iframe");
    const stage = dom.window.document.body;
    const ctrl = new CanvasController(iframe, stage);
    // Use the dom's document as iframe doc
    const doc = dom.window.document;
    // Stub state.document for SDT queries (not needed for hit test, but for nodeToKeys)
    const stateMod = await import("./state.js");
    stateMod.state.document = {
      nodes: [{ id: "sec1", block: "core/section", version: 1, children: [{ id: "txt1", block: "core/text", version: 1, children: [] }] }],
    };

    ctrl.attach(doc);
    assert(ctrl.nodeToKeys.has("txt1"), "before: txt1 should be in index");
    const keysBefore = ctrl.nodeToKeys.get("txt1");
    assert.equal(keysBefore.length, 1);
    const instBefore = ctrl.index.get(keysBefore[0]);
    assert(instBefore, "before instance exists");
    // hitForTarget on the <p> element should resolve to txt1
    const pBefore = doc.querySelector("p");
    const hitBefore = ctrl.hitForTarget(pBefore);
    assert(hitBefore && hitBefore.nodeId === "txt1", "before hit maps to txt1");

    // Simulate preview morph: replace document body with htmlAfter (same DOM instance for simplicity, replace inner)
    // Inject new markers as morph would
    const newDom = new JSDOM(htmlAfter, { pretendToBeVisual: true });
    // Replace body content in original doc to mimic patchPreviewDocument which morphs in place
    doc.body.innerHTML = newDom.window.document.body.innerHTML;

    // Before rebuild, canvas index still describes OLD DOM
    const hitStale = ctrl.hitForTarget(doc.querySelector("p"));
    // hitStale may still reference old instance but rootElements are detached (not in DOM) — should fail to resolve correctly
    // After rebuild, should be correct
    ctrl.rebuildIndex();

    assert(ctrl.nodeToKeys.has("txt1"), "after rebuild: txt1 should still be in index");
    const keysAfter = ctrl.nodeToKeys.get("txt1");
    assert.equal(keysAfter.length, 1);
    const instAfter = ctrl.index.get(keysAfter[0]);
    assert(instAfter, "after instance exists");
    const pAfter = doc.querySelector("p");
    const hitAfter = ctrl.hitForTarget(pAfter);
    assert(hitAfter && hitAfter.nodeId === "txt1", "after rebuild hit maps to txt1 at root");
    // Also sec1 should still exist but empty
    assert(ctrl.nodeToKeys.has("sec1"), "sec1 still indexed");
    // Verify moved element is selectable again (simulates selectInstance path)
    const selInst = ctrl.index.get(keysAfter[0]);
    assert(selInst.editable === true, "moved instance editable");
    // cleanup
    ctrl.destroy();
  });

  it("handle geometry uses new HANDLE_HEIGHT 28", async () => {
    const dom2 = new JSDOM(`<!DOCTYPE html><html><body></body></html>`, { pretendToBeVisual: true });
    globalThis.document = dom2.window.document;
    globalThis.window = dom2.window;
    globalThis.Node = dom2.window.Node;
    const { Overlay } = await import("./overlay.js");
    // Ensure HANDLE_HEIGHT is 28 by checking CSS contains 28px
    const src = await import("fs").then(m => m.readFileSync("internal/web/static/editor-v2/overlay.js", "utf8"));
    assert.match(src, /HANDLE_HEIGHT\s*=\s*28/);
    assert.match(src, /width:\s*32px/);
    assert.match(src, /height:\s*28px/);
    // At least one occurrence of grip width 32 and handle 28
    assert(src.includes("height: ${HANDLE_HEIGHT}px") || src.includes("height: 28px"));
  });

  it("app.js uses rebuildIndex not refresh and surfaces errors", async () => {
    const src = await import("fs").then(m => m.readFileSync("internal/web/static/editor-v2/app.js", "utf8"));
    assert(!src.includes("this.canvas.refresh()"), "should not call nonexistent refresh()");
    assert(src.includes("this.canvas.rebuildIndex()"), "should call rebuildIndex()");
    assert(src.includes('console.error("[editor-v2] canvas rebuild failed"'), "should surface rebuild failures");
    assert(!src.includes("catch (_) {}") || src.includes('console.error("[editor-v2]'), "critical failures should not be silently swallowed");
  });
});
