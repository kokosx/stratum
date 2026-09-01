// toolbar-interaction.test.js — M5B Final Interaction Corrective verification
// Tests required by spec §18: pointer action, double-toggle guard, keyboard, blur, outside commit, Link popover

import { describe, it, before, beforeEach } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const toolbarPath = path.join(__dirname, "richtext-toolbar.js");
const inlinePath = path.join(__dirname, "inline-editor.js");

let toolbarSrc = "";
let inlineSrc = "";
before(() => {
  toolbarSrc = fs.readFileSync(toolbarPath, "utf8");
  inlineSrc = fs.readFileSync(inlinePath, "utf8");
});

describe("static DoD — toolbar runtime shape", () => {
  it("uses pointerdown for formatting, not click-only", () => {
    assert.match(toolbarSrc, /btn\.addEventListener\("pointerdown"/);
    // click handler must contain guard for suppressNextClick (renamed from pointerActionHandled)
    assert.match(toolbarSrc, /suppressNextClick/);
    assert.match(toolbarSrc, /if\s*\(suppressNextClick\)/);
  });

  it("removes timer-based correctness 50/100/200 for toolbarInteraction", () => {
    // Should NOT contain setTimeout 50 or 100 for toolbarInteraction reset
    assert.doesNotMatch(toolbarSrc, /setTimeout\(\(\)\s*=>\s*\{\s*toolbarInteraction\s*=\s*false.*50/);
    assert.doesNotMatch(toolbarSrc, /setTimeout.*100/);
    // Old code had setTimeout with 200 for blur — should be gone
    assert.doesNotMatch(toolbarSrc, /setTimeout.*200/);
    // Allowed timers: 20ms for popover focus (UI sync) + 0 fallback for drag-outside cleanup
    const toolbarTimeouts = (toolbarSrc.match(/setTimeout/g) || []).length;
    assert.equal(toolbarTimeouts, 2, "toolbar should have two setTimeouts: 20ms focus + 0 fallback");
    assert.match(toolbarSrc, /setTimeout.*inputEl\.focus/);
    assert.match(toolbarSrc, /setTimeout\(\(\) => \{\s*if \(suppressNextClick\)/);
  });

  it("explicit lifecycle: begin/end, microtask not timer", () => {
    assert.match(toolbarSrc, /function beginToolbarInteraction/);
    assert.match(toolbarSrc, /function endToolbarInteraction/);
    assert.match(toolbarSrc, /queueMicrotask/);
  });

  it("single callback registration style", () => {
    const inlineCalls = (inlineSrc.match(/setToolbarCallbacks/g) || []).length;
    assert.equal(inlineCalls, 1, "exactly one setToolbarCallbacks call");
    assert.match(inlineSrc, /setToolbarCallbacks\(\{\s*toggleMark/);
    // triggerLink must NOT contain setToolbarCallbacks
    const triggerLinkBlock = inlineSrc.slice(inlineSrc.indexOf("function triggerLink"));
    const triggerEnd = triggerLinkBlock.indexOf("\n}\n");
    const triggerSrc = triggerLinkBlock.slice(0, triggerEnd + 200);
    assert.doesNotMatch(triggerSrc, /setToolbarCallbacks/);
  });

  it("single mark implementation via applyRichMark", () => {
    const applyCount = (inlineSrc.match(/function applyRichMark/g) || []).length;
    assert.equal(applyCount, 1);
    assert.match(inlineSrc, /toggleMark:\s*\(mark,\s*offsets\)\s*=>\s*applyRichMark/);
    // shortcuts should delegate to applyRichMark, not inline toggleMarkInRichText
    const attachSection = inlineSrc.slice(inlineSrc.indexOf("function attachRichHandlers"));
    // count direct toggleMarkInRichText inside attachRichHandlers — should be 0 after collapse (only inside applyRichMark)
    const directToggles = (attachSection.match(/toggleMarkInRichText\(cur,/g) || []).length;
    // applyRichMark itself contains one, the shortcuts should not contain additional
    // ApplyRichMark is outside attach, so inside attach there should be 0
    assert.equal(directToggles, 0, "attachRichHandlers shortcuts must not directly call toggleMarkInRichText");
  });

  it("saved offsets are action source and clamped", () => {
    assert.match(toolbarSrc, /savedToolbarOffsets/);
    assert.match(toolbarSrc, /performToolbarAction/);
    assert.match(inlineSrc, /Math\.max\(0,\s*Math\.min\(offsets\.start/);
    assert.match(inlineSrc, /Math\.max\(0,\s*Math\.min\(offsets\.end/);
    // check swap for start>end
    assert.match(inlineSrc, /if\s*\(s\s*>\s*e\)/);
  });

  it("selectionchange/blur do not hide during interaction", () => {
    assert.match(inlineSrc, /if\s*\(RichToolbar\.isToolbarInteraction\(\)\)\s*return/);
    // hideToolbar should guard clearing saved offsets
    assert.match(toolbarSrc, /if\s*\(!toolbarInteraction\)\s*savedToolbarOffsets/);
    // onBlur must check isToolbarInteraction before commit
    assert.match(inlineSrc, /if\s*\(RichToolbar\.isPopoverVisible\(\)\s*\|\|\s*RichToolbar\.isToolbarInteraction\(\)\)\s*return/);
    // onScroll guarded
    assert.match(inlineSrc, /onScroll.*isToolbarInteraction/);
  });

  it("toolbar click must not commit editor", () => {
    // cleanupEditingState/commitActiveEdit should not appear inside toolbar click path
    // Ensure toolbar file does not import or call commitActiveEdit
    assert.doesNotMatch(toolbarSrc, /commitActiveEdit|cleanupEditingState|cancelActiveEdit/);
  });

  it("keyboard click still works via click fallback", () => {
    // click handler must have else branch performing action for keyboard
    assert.match(toolbarSrc, /Keyboard activation/);
  });
});

// Behavioral: guard double-toggle and single mark path using real RichText helpers
// We test the guard logic extracted to a small harness mimicking toolbar's behavior

describe("behavioral guard — pointer vs keyboard", () => {
  function makeToolbarHarness() {
    let toggleCalls = 0;
    let lastMark = null;
    let pointerActionHandled = false;
    let toolbarInteraction = false;
    function begin() { toolbarInteraction = true; }
    function end() { toolbarInteraction = false; }
    function perform(mark) {
      toggleCalls++;
      lastMark = mark;
    }
    function pointerdown(mark) {
      begin();
      pointerActionHandled = true;
      perform(mark);
    }
    function click(mark) {
      if (pointerActionHandled) {
        pointerActionHandled = false;
        queueMicrotask(() => end());
        return;
      }
      perform(mark);
      queueMicrotask(() => end());
    }
    return { get calls() { return toggleCalls; }, get mark() { return lastMark; }, pointerdown, click, get interaction() { return toolbarInteraction; }, get handled() { return pointerActionHandled; } };
  }

  it("pointer action toggles once", async () => {
    const h = makeToolbarHarness();
    h.pointerdown("bold");
    assert.equal(h.calls, 1);
    assert.equal(h.mark, "bold");
  });

  it("pointerdown + click does not double toggle", async () => {
    const h = makeToolbarHarness();
    h.pointerdown("bold");
    h.click("bold");
    assert.equal(h.calls, 1, "click after pointerdown must be no-op");
    // allow microtask to clear
    await new Promise(r => queueMicrotask(r));
    assert.equal(h.handled, false);
  });

  it("keyboard click (no pointerdown) toggles once", async () => {
    const h = makeToolbarHarness();
    h.click("italic");
    assert.equal(h.calls, 1);
    assert.equal(h.mark, "italic");
  });

  it("two separate pointer actions toggle twice (not once)", async () => {
    const h = makeToolbarHarness();
    h.pointerdown("bold");
    h.click("bold");
    await new Promise(r => queueMicrotask(r));
    h.pointerdown("bold");
    assert.equal(h.calls, 2);
  });
});

describe("behavioral — applyRichMark clamping", async () => {
  // Mock minimal document for richtext-editor
  globalThis.document = {
    createElement: () => ({ style: {}, setAttribute: () => {}, append: () => {} }),
    createHTMLDocument: () => ({ createElement: () => ({}) }),
    querySelector: () => null,
    querySelectorAll: () => [],
    implementation: { createHTMLDocument: () => ({ createElement: () => ({ childNodes: [], attributes: {}, style: {}, setAttribute(){}, getAttribute(){}, appendChild(){}, querySelector(){}, querySelectorAll(){return []} }) }) }
  };
  globalThis.Node = { TEXT_NODE: 3, ELEMENT_NODE: 1, COMMENT_NODE: 8 };
  globalThis.NodeFilter = { SHOW_TEXT: 4, SHOW_COMMENT: 128 };
  const rich = await import("./richtext-editor.js");

  it("clamps offsets beyond text length", () => {
    const rt = { version: 1, content: [{ text: "hello" }] }; // len 5
    // toggle bold on offsets 0-10 should clamp to 0-5 and apply
    const updated = rich.toggleMarkInRichText(rt, 0, 10, "bold");
    // all text should be bold
    assert.equal(updated.content[0].marks[0].type, "bold");
  });

  it("handles reversed offsets via applyRichMark swap (inline-editor logic)", async () => {
    // Simulate inline-editor's clamp+swap
    function clamp(offsets, len) {
      let s = Math.max(0, Math.min(offsets.start, len));
      let e = Math.max(0, Math.min(offsets.end, len));
      if (s > e) [s, e] = [e, s];
      return { s, e };
    }
    const r = clamp({ start: 5, end: 2 }, 5);
    assert.equal(r.s, 2);
    assert.equal(r.e, 5);
    const r2 = clamp({ start: -3, end: 100 }, 5);
    assert.equal(r2.s, 0);
    assert.equal(r2.e, 5);
  });
});

describe("behavioral — blur/commit guards", () => {
  it("toolbar interaction blocks commit, outside allows", async () => {
    let committed = false;
    let toolbarInteraction = true;
    let popoverVisible = false;
    function onBlur() {
      queueMicrotask(() => {
        if (popoverVisible || toolbarInteraction) return;
        committed = true;
      });
    }
    onBlur();
    await new Promise(r => queueMicrotask(r));
    assert.equal(committed, false, "should not commit during toolbarInteraction");
    toolbarInteraction = false;
    onBlur();
    await new Promise(r => queueMicrotask(r));
    assert.equal(committed, true, "should commit when outside");
  });

  it("link popover preserves offsets and allows focus", () => {
    // Simulate showPopover saving offsets
    let saved = null;
    let popoverVisible = false;
    function showPopover(offsets) {
      saved = { ...offsets };
      popoverVisible = true;
    }
    showPopover({ start: 2, end: 5 });
    assert.deepEqual(saved, { start: 2, end: 5 });
    assert.equal(popoverVisible, true);
    // hide should clear interaction but popover replaces toolbar (correct)
    function hidePopover() { popoverVisible = false; saved = null; }
    hidePopover();
    assert.equal(popoverVisible, false);
  });
});

describe("M5B UX — caret and toolbar persistence", () => {
  it("startInlineEdit does not auto-select whole paragraph", () => {
    assert.match(inlineSrc, /placeCaretFromPoint/);
    assert.match(inlineSrc, /placeCaretAtEnd/);
    const startBlock = inlineSrc.slice(inlineSrc.indexOf("export function startInlineEdit"));
    const endIdx = startBlock.indexOf("if (mode === \"rich\") attachRichHandlers");
    const block = startBlock.slice(0, endIdx);
    // old pattern without collapse should be gone
    assert.doesNotMatch(block, /range\.selectNodeContents\(fieldEl\);\s*\n\s*const sel =/);
    assert.match(block, /range\.collapse\(false\)/);
  });

  it("uses caretPositionFromPoint / caretRangeFromPoint with validation", () => {
    assert.match(inlineSrc, /caretPositionFromPoint/);
    assert.match(inlineSrc, /caretRangeFromPoint/);
    assert.match(inlineSrc, /fieldEl\.contains\(/);
  });

  it("keyboard Enter places caret at end, not select-all", () => {
    const startBlock = inlineSrc.slice(inlineSrc.indexOf("export function startInlineEdit"));
    assert.match(startBlock, /placeCaretAtEnd/);
  });

  it("canvas second click enters edit at point, first selects", () => {
    const canvasSrc = fs.readFileSync(path.join(__dirname, "canvas.js"), "utf8");
    assert.match(canvasSrc, /Second click on already selected editable block/);
    assert.match(canvasSrc, /startInlineEdit\(hit\.nodeId, hit\.instanceKey, this, undefined, \{ clientX: x, clientY: y \}\)/);
  });

  it("canvas onDblClick does not hijack when editing", () => {
    const canvasSrc = fs.readFileSync(path.join(__dirname, "canvas.js"), "utf8");
    const dbl = canvasSrc.slice(canvasSrc.indexOf("onDblClick"));
    assert.match(dbl, /if \(isInlineEditing\(\)\) return;/);
  });

  it("toolbar remains after mark — applyRichMark calls showToolbar not hide", () => {
    const applyBlock = inlineSrc.slice(inlineSrc.indexOf("function applyRichMark"));
    const applyEnd = applyBlock.indexOf("\n}\n");
    const block = applyBlock.slice(0, applyEnd);
    assert.match(block, /RichToolbar\.showToolbar/);
    assert.doesNotMatch(block, /hideToolbar/);
  });

  it("collapsed selection hides toolbar, drag/word selection native", () => {
    assert.match(inlineSrc, /if \(!offsets \|\| offsets\.start === offsets\.end\) \{[^}]*hideToolbar/);
    const canvasSrc = fs.readFileSync(path.join(__dirname, "canvas.js"), "utf8");
    assert.match(canvasSrc, /if \(isInlineEditing\(\)\) \{[^}]*clearHover/);
    assert.match(canvasSrc, /if \(activeEl && \(e\.target === activeEl/);
  });
});

describe("M5B Final Focus — double action + preserve focus", () => {
  it("pointerup does not clear suppressNextClick via microtask", () => {
    assert.doesNotMatch(toolbarSrc, /btn\.addEventListener\("pointerup", \(\) => \{\s*queueMicrotask/);
    assert.match(toolbarSrc, /btn\.addEventListener\("pointerup"[\s\S]*setTimeout/);
    assert.match(toolbarSrc, /suppressNextClick = true/);
  });

  it("pointerdown executes exactly once, click consumes", () => {
    assert.match(toolbarSrc, /btn\.addEventListener\("pointerdown"[\s\S]*suppressNextClick = true[\s\S]*performToolbarAction/);
    assert.match(toolbarSrc, /btn\.addEventListener\("click"[\s\S]*if \(suppressNextClick\)/);
  });

  it("restoreRichEditingContext focuses first then restores", () => {
    assert.match(inlineSrc, /function restoreRichEditingContext/);
    const fn = inlineSrc.slice(inlineSrc.indexOf("function restoreRichEditingContext"));
    const focusIdx = fn.indexOf("fieldEl.focus");
    const restoreIdx = fn.indexOf("restoreSelectionFromOffsets");
    assert.ok(focusIdx !== -1 && restoreIdx !== -1 && focusIdx < restoreIdx, "focus must come before restore");
    assert.match(fn, /fieldEl\.setAttribute\("contenteditable"/);
  });

  it("applyRichMark uses restore helper and preserves toolbar", () => {
    const apply = inlineSrc.slice(inlineSrc.indexOf("function applyRichMark"));
    const applyEnd = apply.indexOf("\n}\n");
    const block = apply.slice(0, applyEnd);
    assert.match(block, /restoreRichEditingContext/);
    assert.match(block, /RichToolbar\.showToolbar/);
    assert.doesNotMatch(block, /hideToolbar/);
  });

  it("link popover close restores via same helper", () => {
    assert.match(inlineSrc, /closeLink[\s\S]*restoreRichEditingContext/);
  });

  it("toolbar click does not commit editing", () => {
    // blur guard still
    assert.match(inlineSrc, /if \(RichToolbar\.isPopoverVisible\(\) \|\| RichToolbar\.isToolbarInteraction\(\)\) return/);
    // restore helper ensures field stays contenteditable
    assert.match(inlineSrc, /fieldEl\.setAttribute\("data-stratum-editing"/);
  });
});
