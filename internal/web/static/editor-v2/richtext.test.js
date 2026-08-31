// richtext.test.js — M5B RichText model tests
import { describe, it } from "node:test";
import assert from "node:assert/strict";

// Mock minimal document for richtext-editor imports (it uses document at runtime, but we test pure functions)
globalThis.document = {
  getElementById: () => null,
  createElement: () => ({ style: {}, setAttribute: () => {}, append: () => {} }),
  querySelector: () => null,
  querySelectorAll: () => [],
  implementation: {
    createHTMLDocument: () => ({
      createElement: (tag) => {
        const el = {
          tagName: tag.toUpperCase(),
          childNodes: [],
          attributes: {},
          style: {},
          setAttribute(k, v) { this.attributes[k] = v; },
          getAttribute(k) { return this.attributes[k] || null; },
          appendChild(c) { this.childNodes.push(c); c.parentNode = this; return c; },
          get innerHTML() { return this._innerHTML || ""; },
          set innerHTML(html) {
            this._innerHTML = html;
            this.childNodes = [];
            // Simple parser for test: handle strong, script, span etc.
            // Create a temporary parsing: split by tags
            const tagRe = /<\/?(\w+)([^>]*)>([^<]*)/g;
            // For this test, we want childNodes: strong(Hello), script(alert(1)), span(world)
            // Use a simple state machine: iterate through html, create nodes
            let lastIndex = 0;
            let match;
            const stack = [this];
            // Very simplified: handle <strong>Hello</strong><script>alert(1)</script><span ...>world</span>
            // We'll use regex to find tags and text
            const re = /<(\/?)(\w+)([^>]*)>|([^<]+)/g;
            let m;
            const tempStack = [this];
            while ((m = re.exec(html)) !== null) {
              if (m[4] !== undefined) {
                // Text
                const text = m[4];
                if (text.trim() !== "" || text.includes(" ")) {
                  const parent = tempStack[tempStack.length - 1];
                  parent.childNodes.push({ nodeType: 3, textContent: text, parentNode: parent });
                }
              } else if (m[1] === "") {
                // Opening tag
                const tag = m[2].toLowerCase();
                const attrs = m[3] || "";
                const el = {
                  nodeType: 1,
                  tagName: tag.toUpperCase(),
                  attributes: {},
                  childNodes: [],
                  getAttribute(k) { return this.attributes[k] || null; },
                  setAttribute(k, v) { this.attributes[k] = v; },
                };
                // Parse href for <a>
                if (tag === "a") {
                  const hrefMatch = attrs.match(/href\s*=\s*["']([^"']+)["']/i);
                  if (hrefMatch) el.attributes["href"] = hrefMatch[1];
                }
                // Parse style for span (but we will strip)
                tempStack[tempStack.length - 1].childNodes.push(el);
                // For self-closing like <br>, don't push to stack
                if (!["br","img","hr","input"].includes(tag)) {
                  tempStack.push(el);
                }
              } else {
                // Closing tag
                if (tempStack.length > 1) tempStack.pop();
              }
            }
          },
        };
        return el;
      },
    }),
  },
};
globalThis.Node = { TEXT_NODE: 3, ELEMENT_NODE: 1 };
globalThis.NodeFilter = { SHOW_TEXT: 4 };
globalThis.window = globalThis;
globalThis.window.location = { origin: "http://localhost" };

const richMod = await import("./richtext-editor.js");
const { normalizeRichText, toggleMarkInRichText, isSafeHref, htmlToRichText } = richMod;

describe("normalizeRichText", () => {
  it("merges adjacent same marks", () => {
    const input = { version: 1, content: [{ text: "foo", marks: [{ type: "bold" }] }, { text: "bar", marks: [{ type: "bold" }] }] };
    const out = normalizeRichText(input);
    assert.equal(out.content.length, 1);
    assert.equal(out.content[0].text, "foobar");
    assert.deepEqual(out.content[0].marks, [{ type: "bold" }]);
  });
  it("removes zero-length runs", () => {
    const input = { version: 1, content: [{ text: "" }, { text: "hi" }, { text: "" }] };
    const out = normalizeRichText(input);
    assert.equal(out.content.length, 1);
    assert.equal(out.content[0].text, "hi");
  });
  it("dedup and sort marks", () => {
    const input = { version: 1, content: [{ text: "hi", marks: [{ type: "italic" }, { type: "bold" }, { type: "bold" }] }] };
    const out = normalizeRichText(input);
    assert.deepEqual(out.content[0].marks, [{ type: "bold" }, { type: "italic" }]);
  });
  it("enforces mark order alphabetical", () => {
    const input = { version: 1, content: [{ text: "x", marks: [{ type: "link", href: "/a" }, { type: "bold" }] }] };
    const out = normalizeRichText(input);
    assert.equal(out.content[0].marks[0].type, "bold");
    assert.equal(out.content[0].marks[1].type, "link");
  });
});

describe("toggleMarkInRichText", () => {
  it("partial bold split", () => {
    const input = { version: 1, content: [{ text: "abcdef" }] };
    const out = toggleMarkInRichText(input, 2, 4, "bold");
    assert.equal(out.content.length, 3);
    assert.equal(out.content[0].text, "ab");
    assert.equal(out.content[0].marks, undefined);
    assert.equal(out.content[1].text, "cd");
    assert.deepEqual(out.content[1].marks, [{ type: "bold" }]);
    assert.equal(out.content[2].text, "ef");
  });
  it("overlapping bold+italic", () => {
    const input = { version: 1, content: [{ text: "abcdef", marks: [{ type: "bold" }] }] };
    const out = toggleMarkInRichText(input, 2, 4, "italic");
    assert.equal(out.content.length, 3);
    assert.equal(out.content[0].text, "ab");
    assert.deepEqual(out.content[0].marks, [{ type: "bold" }]);
    assert.equal(out.content[1].text, "cd");
    assert.deepEqual(out.content[1].marks, [{ type: "bold" }, { type: "italic" }]);
    assert.equal(out.content[2].text, "ef");
    assert.deepEqual(out.content[2].marks, [{ type: "bold" }]);
  });
  it("remove bold", () => {
    const input = { version: 1, content: [{ text: "abcdef", marks: [{ type: "bold" }] }] };
    const out = toggleMarkInRichText(input, 2, 4, "bold");
    assert.equal(out.content.length, 3);
    assert.equal(out.content[0].text, "ab");
    assert.deepEqual(out.content[0].marks, [{ type: "bold" }]);
    assert.equal(out.content[1].text, "cd");
    assert.equal(out.content[1].marks, undefined);
    assert.equal(out.content[2].text, "ef");
  });
  it("link with other marks preserved", () => {
    const input = { version: 1, content: [{ text: "Read more", marks: [{ type: "bold" }] }] };
    const out = toggleMarkInRichText(input, 0, 9, "link", "/guide");
    assert.equal(out.content.length, 1);
    assert.deepEqual(out.content[0].marks, [{ type: "bold" }, { type: "link", href: "/guide" }]);
  });
  it("remove link keeps bold", () => {
    const input = { version: 1, content: [{ text: "click", marks: [{ type: "bold" }, { type: "link", href: "/path" }] }] };
    const out = toggleMarkInRichText(input, 0, 5, "link", null);
    assert.equal(out.content[0].marks.length, 1);
    assert.equal(out.content[0].marks[0].type, "bold");
  });
  it("mixed selection applies bold to all", () => {
    const input = { version: 1, content: [{ text: "ab" }, { text: "cd", marks: [{ type: "bold" }] }, { text: "ef" }] };
    const out = toggleMarkInRichText(input, 0, 6, "bold");
    // All should be bold after
    for (const run of out.content) {
      assert.ok(run.marks && run.marks.some(m => m.type === "bold"));
    }
  });
});

describe("isSafeHref", () => {
  it("allows safe hrefs", () => {
    assert.equal(isSafeHref("/path"), true);
    assert.equal(isSafeHref("#anchor"), true);
    assert.equal(isSafeHref("https://example.com"), true);
    assert.equal(isSafeHref("mailto:test@example.com"), true);
    assert.equal(isSafeHref("tel:+123"), true);
  });
  it("rejects unsafe", () => {
    assert.equal(isSafeHref("javascript:alert(1)"), false);
    assert.equal(isSafeHref("//evil.com"), false);
    assert.equal(isSafeHref("data:text/html,hi"), false);
    assert.equal(isSafeHref(""), false);
  });
});

describe("htmlToRichText paste", () => {
  it("preserves strong, strips script", () => {
    const html = '<strong>Hello</strong><script>alert(1)</script><span style="color:red">world</span>';
    const rich = htmlToRichText(html);
    // Should have Hello bold, world plain, script removed
    const text = rich.content.map(r => r.text).join("");
    assert.ok(text.includes("Hello"));
    assert.ok(text.includes("world"));
    assert.equal(rich.content.some(r => r.text.includes("alert")), false);
    // Hello should be bold
    const helloRun = rich.content.find(r => r.text.includes("Hello"));
    assert.ok(helloRun.marks && helloRun.marks.some(m => m.type === "bold"));
    // world should be plain (span stripped)
    const worldRun = rich.content.find(r => r.text.includes("world"));
    assert.equal(worldRun.marks, undefined);
  });
});
