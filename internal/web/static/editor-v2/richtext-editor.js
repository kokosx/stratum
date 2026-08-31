// richtext-editor.js — RichText v1 DOM editing, serialization, marks
// Allowed inline elements: text, <strong>/<b>, <em>/<i>, <s>/<strike>, <code>, <a href>

const ALLOWED_MARKS = new Set(["bold", "italic", "strike", "code", "link"]);
const TAG_TO_MARK = {
  "strong": "bold",
  "b": "bold",
  "em": "italic",
  "i": "italic",
  "s": "strike",
  "strike": "strike",
  "code": "code",
};
const MARK_TO_TAG = {
  "bold": "strong",
  "italic": "em",
  "strike": "s",
  "code": "code",
};

// Canonical mark order: alphabetical (bold, code, italic, link, strike) matching Go's sort
function sortMarks(marks) {
  return [...marks].sort((a, b) => {
    if (a.type === b.type) return (a.href || "").localeCompare(b.href || "");
    return a.type.localeCompare(b.type);
  });
}

function dedupMarks(marks) {
  const map = new Map();
  for (const m of marks || []) {
    if (!m || typeof m.type !== "string") continue;
    if (!ALLOWED_MARKS.has(m.type)) continue;
    const key = m.type + "\x00" + (m.href || "");
    if (m.type === "link" && (!m.href || typeof m.href !== "string")) continue;
    map.set(key, { type: m.type, ...(m.href ? { href: m.href } : {}) });
  }
  return sortMarks(Array.from(map.values()));
}

export function isSafeHref(href) {
  if (!href || typeof href !== "string") return false;
  const t = href.trim();
  if (t === "") return false;
  if (t.startsWith("#") || t.startsWith("/")) return !t.startsWith("//");
  try {
    const u = new URL(t, "http://example.com");
    const scheme = u.protocol.replace(":", "").toLowerCase();
    return ["http", "https", "mailto", "tel"].includes(scheme);
  } catch (_) { return false; }
}

export function normalizeRichText(richText) {
  if (!richText || typeof richText !== "object" || richText.version !== 1 || !Array.isArray(richText.content)) {
    return { version: 1, content: [] };
  }
  const out = [];
  for (const run of richText.content) {
    if (!run || typeof run.text !== "string" || run.text.length === 0) continue;
    let marks = run.marks && Array.isArray(run.marks) ? dedupMarks(run.marks) : undefined;
    if (marks && marks.length === 0) marks = undefined;
    if (out.length > 0) {
      const last = out[out.length - 1];
      const lastMarks = last.marks ? JSON.stringify(last.marks) : "[]";
      const curMarks = marks ? JSON.stringify(marks) : "[]";
      if (lastMarks === curMarks) {
        last.text += run.text;
        continue;
      }
    }
    const newRun = { text: run.text };
    if (marks) newRun.marks = marks;
    out.push(newRun);
  }
  // Enforce limits
  if (out.length > 200) out.splice(200);
  let total = 0;
  for (const r of out) total += r.text.length;
  if (total > 10000) {
    let acc = 0;
    const truncated = [];
    for (const r of out) {
      if (acc + r.text.length <= 10000) { truncated.push(r); acc += r.text.length; }
      else {
        const rem = 10000 - acc;
        if (rem > 0) truncated.push({ ...r, text: r.text.slice(0, rem) });
        break;
      }
    }
    return { version: 1, content: truncated };
  }
  return { version: 1, content: out };
}

export function renderRichTextToDOM(fieldEl, richText) {
  if (!fieldEl) return;
  const doc = fieldEl.ownerDocument;
  fieldEl.replaceChildren();
  if (!richText || !Array.isArray(richText.content) || richText.content.length === 0) return;
  for (const run of richText.content) {
    if (!run || typeof run.text !== "string" || run.text.length === 0) continue;
    let node = doc.createTextNode(run.text);
    const marks = run.marks && Array.isArray(run.marks) ? dedupMarks(run.marks) : [];
    // Sort marks and wrap
    // For deterministic nesting, wrap in sorted order (outermost first is last in array? We need consistent)
    // We'll wrap from last to first so first in sorted order is outermost
    // But Go's Render loops marks in stored order and does content = "<strong>"+content+"</strong>" sequentially, so sorted order outermost is last? Let's just wrap in sorted order sequentially: start with text node, for each mark in sorted order wrap.
    for (const mark of marks) {
      const tag = MARK_TO_TAG[mark.type] || (mark.type === "link" ? "a" : null);
      if (!tag) continue;
      const el = doc.createElement(tag);
      if (mark.type === "link") {
        el.setAttribute("href", mark.href);
      }
      el.appendChild(node);
      node = el;
    }
    fieldEl.appendChild(node);
  }
}

export function domToRichText(fieldEl) {
  const content = [];
  function walk(node, marks) {
    if (node.nodeType === Node.TEXT_NODE) {
      const text = node.textContent;
      // Preserve text as is; placeholder via CSS not in DOM
      if (text.length === 0) return;
      // Split on \n? For M5B, we don't store \n as block, but if text contains \n, keep as is for now
      // For now, push as single run
      content.push({ text, marks: marks.length ? [...marks] : undefined });
      return;
    }
    if (node.nodeType === Node.ELEMENT_NODE) {
      const tag = node.tagName.toLowerCase();
      let newMarks = marks;
      if (tag === "strong" || tag === "b") newMarks = [...marks, { type: "bold" }];
      else if (tag === "em" || tag === "i") newMarks = [...marks, { type: "italic" }];
      else if (tag === "s" || tag === "strike") newMarks = [...marks, { type: "strike" }];
      else if (tag === "code") newMarks = [...marks, { type: "code" }];
      else if (tag === "a" && node.getAttribute("href")) {
        const href = node.getAttribute("href");
        if (isSafeHref(href)) newMarks = [...marks, { type: "link", href }];
        else newMarks = [...marks];
      } else if (tag === "br") {
        // Treat <br> as space to avoid words joining, since RichText has no block
        content.push({ text: " ", marks: marks.length ? [...marks] : undefined });
        return;
      } else {
        // Unsupported tag: unwrap (e.g., span, div, p, ul, li) — insert space for block boundaries
        const isBlock = ["p","div","li","ul","ol","h1","h2","h3","h4","h5","h6","blockquote","pre","section","header","footer","article"].includes(tag);
        if (isBlock && content.length > 0) {
          const last = content[content.length - 1];
          if (last && !last.text.endsWith(" ") && !last.text.endsWith("\n")) {
            // Insert separator space if needed
            // Check next text will be added, but we add space now as separate run
            // Instead, we will ensure that walking children will have space separation via explicit " "
            // For now, push a space run if needed
            const needSpace = true;
            if (needSpace) {
              // We'll add a space before next text; simplest is to add a space run
              // But we don't know next text yet. We'll add a space marker and let normalization merge later
              // For now, add a space with same marks
              // Actually we should just ensure that when we walk children, we insert a space between blocks
              // We'll handle by adding a space text node before walking children if needed
            }
          }
        }
        // For block tags, we want to ensure separation: walk children and after each block, ensure space
        // We handle by walking children with same marks, but inserting space between block children at parent level
        // Simpler: just walk children
        for (const child of node.childNodes) {
          // For block children, insert space before each subsequent block's content if needed
          walk(child, marks);
        }
        // After walking a block tag, if it was a block and next sibling will be another block, we should ensure separation
        // We handle separation at higher level by checking after walk, but easier: if tag is block and next sibling exists, ensure last run ends with space?
        // We'll just add a space after each block element if not already space-terminated
        if (isBlock) {
          // Add a space separator as separate run (will be normalized)
          // But only if content not empty and last text doesn't end with space
          if (content.length > 0) {
            const last = content[content.length - 1];
            if (last.text && !last.text.endsWith(" ")) {
              // Peek next sibling: if next sibling exists and its text doesn't start with space, insert space
              // For simplicity, insert a single space run with same marks as current (or no marks)
              // We'll insert a space that will be merged later if needed
              // Actually we should insert a space with no marks to separate blocks
              content.push({ text: " ", marks: undefined });
            }
          }
        }
        return;
      }
      for (const child of node.childNodes) {
        walk(child, newMarks);
      }
      return;
    }
  }
  for (const child of fieldEl.childNodes) {
    walk(child, []);
  }
  // Filter out runs that are whitespace-only? But we need to preserve spaces between words
  // Remove zero-length runs already filtered via walk, but we may have inserted extra spaces for block separation - keep them
  // Normalize will merge and remove empty
  const rich = normalizeRichText({ version: 1, content });
  // Trim leading/trailing single space runs that were added for block separation but at edges?
  // If content starts or ends with a single space that was just for separation, we should trim?
  // For now, keep as is; but we should trim leading/trailing spaces that are just block separators at start/end
  if (rich.content.length > 0) {
    // Remove leading/trailing space-only runs that are at start/end and were added for block separation
    // But we want to preserve intentional leading/trailing spaces? For M5B, leading/trailing spaces are trimmed on commit via plain handling, but for rich we preserve internal spaces
    // We'll just trim if first/last run is single space and next run exists
    // This is edge case for paste of "<p>Hello</p><p>World</p>" -> we inserted space between, so content is ["Hello", " ", "World"] -> "Hello World" correct
    // If paste is single <p>Hello</p>, we inserted trailing space after block, so content would be ["Hello", " "] -> we should trim trailing space
    if (rich.content[0].text === " " && rich.content.length > 1) rich.content.shift();
    if (rich.content.length > 0 && rich.content[rich.content.length - 1].text === " ") rich.content.pop();
    // After trimming, re-normalize merges
    if (rich.content.length !== content.length) {
      return normalizeRichText(rich);
    }
  }
  return rich;
}

export function selectionToOffsets(fieldEl) {
  const doc = fieldEl.ownerDocument;
  const sel = doc.getSelection ? doc.getSelection() : null;
  if (!sel || sel.rangeCount === 0) return null;
  const range = sel.getRangeAt(0);
  if (!fieldEl.contains(range.commonAncestorContainer) && range.commonAncestorContainer !== fieldEl) return null;
  try {
    const preStart = doc.createRange();
    preStart.selectNodeContents(fieldEl);
    preStart.setEnd(range.startContainer, range.startOffset);
    const start = preStart.toString().length;
    const preEnd = doc.createRange();
    preEnd.selectNodeContents(fieldEl);
    preEnd.setEnd(range.endContainer, range.endOffset);
    const end = preEnd.toString().length;
    return { start, end };
  } catch (_) { return null; }
}

export function offsetsToRange(fieldEl, start, end) {
  const doc = fieldEl.ownerDocument;
  let charIndex = 0;
  let startNode, startOffset, endNode, endOffset;
  const walker = doc.createTreeWalker(fieldEl, NodeFilter.SHOW_TEXT, null);
  let node;
  // Handle empty field
  if (fieldEl.textContent.length === 0 && start === 0 && end === 0) {
    const range = doc.createRange();
    range.selectNodeContents(fieldEl);
    range.collapse(true);
    return range;
  }
  while ((node = walker.nextNode())) {
    const next = charIndex + node.textContent.length;
    if (startNode === undefined && start >= charIndex && start <= next) {
      startNode = node;
      startOffset = start - charIndex;
    }
    if (endNode === undefined && end >= charIndex && end <= next) {
      endNode = node;
      endOffset = end - charIndex;
    }
    charIndex = next;
    if (startNode !== undefined && endNode !== undefined) break;
  }
  if (startNode && endNode) {
    const range = doc.createRange();
    try {
      range.setStart(startNode, startOffset);
      range.setEnd(endNode, endOffset);
      return range;
    } catch (_) { return null; }
  }
  if (startNode && end === start) {
    const range = doc.createRange();
    try { range.setStart(startNode, startOffset); range.collapse(true); return range; } catch (_) {}
  }
  // Fallback: select all
  if (start === 0 && end === fieldEl.textContent.length) {
    const range = doc.createRange();
    range.selectNodeContents(fieldEl);
    return range;
  }
  return null;
}

export function restoreSelectionFromOffsets(fieldEl, start, end) {
  const range = offsetsToRange(fieldEl, start, end);
  if (!range) return false;
  const sel = fieldEl.ownerDocument.getSelection();
  if (!sel) return false;
  sel.removeAllRanges();
  sel.addRange(range);
  return true;
}

// Mark toggling over offsets
export function toggleMarkInRichText(richText, start, end, markType, href) {
  if (start == null || end == null || start === end) return richText;
  // Determine if all selected runs have the mark
  let allHave = true;
  let pos = 0;
  for (const run of richText.content) {
    const runEnd = pos + run.text.length;
    if (runEnd <= start || pos >= end) { pos = runEnd; continue; }
    const has = run.marks && run.marks.some(m => {
      if (m.type !== markType) return false;
      if (markType === "link") return m.href === href;
      return true;
    });
    // For link removal when href==null (remove any link), check any link
    let hasForCheck = has;
    if (markType === "link" && href == null) {
      hasForCheck = run.marks && run.marks.some(m => m.type === "link");
    }
    if (!hasForCheck) allHave = false;
    pos = runEnd;
    if (!allHave) break;
  }
  // Special for link with href==null meaning remove any link
  const isRemoveLink = markType === "link" && (href == null || href === "");

  let newContent = [];
  pos = 0;
  for (const run of richText.content) {
    const runStart = pos;
    const runEnd = pos + run.text.length;
    pos = runEnd;
    if (runEnd <= start || runStart >= end) {
      newContent.push({ text: run.text, ...(run.marks ? { marks: [...run.marks] } : {}) });
      continue;
    }
    const beforeLen = Math.max(0, start - runStart);
    const afterLen = Math.max(0, runEnd - end);
    const selectedLen = run.text.length - beforeLen - afterLen;
    if (beforeLen > 0) {
      newContent.push({ text: run.text.slice(0, beforeLen), ...(run.marks ? { marks: [...run.marks] } : {}) });
    }
    let selectedText = run.text.slice(beforeLen, beforeLen + selectedLen);
    let marks = run.marks ? [...run.marks] : [];
    if (markType === "link") {
      if (isRemoveLink) {
        marks = marks.filter(m => m.type !== "link");
      } else if (allHave) {
        // remove this specific href
        marks = marks.filter(m => !(m.type === "link" && m.href === href));
        // if href was uniform and we are toggling off, this removes it
      } else {
        // add/replace link
        marks = marks.filter(m => m.type !== "link");
        if (href && isSafeHref(href)) marks.push({ type: "link", href });
      }
    } else {
      const has = marks.some(m => m.type === markType);
      if (allHave) {
        // remove
        marks = marks.filter(m => m.type !== markType);
      } else {
        // add if not has
        if (!has) marks.push({ type: markType });
      }
    }
    const uniq = new Map();
    for (const m of marks) {
      const key = m.type + "\x00" + (m.href || "");
      uniq.set(key, m);
    }
    let deduped = Array.from(uniq.values());
    deduped = sortMarks(deduped);
    newContent.push({ text: selectedText, ...(deduped.length ? { marks: deduped } : {}) });
    if (afterLen > 0) {
      newContent.push({ text: run.text.slice(run.text.length - afterLen), ...(run.marks ? { marks: [...run.marks] } : {}) });
    }
  }
  return normalizeRichText({ version: 1, content: newContent });
}

export function applyMarkToField(fieldEl, markType, href) {
  const offsets = selectionToOffsets(fieldEl);
  if (!offsets || offsets.start === offsets.end) return false;
  const current = domToRichText(fieldEl);
  const updated = toggleMarkInRichText(current, offsets.start, offsets.end, markType, href);
  renderRichTextToDOM(fieldEl, updated);
  // Restore selection
  restoreSelectionFromOffsets(fieldEl, offsets.start, offsets.end);
  return true;
}

// Paste handling for rich: html string -> sanitized RichText insertion at selection
export function htmlToRichText(html) {
  const doc = document.implementation.createHTMLDocument("");
  const container = doc.createElement("div");
  container.innerHTML = html;
  // Walk and collect text with marks, similar to domToRichText but from container
  const content = [];
  function walk(node, marks) {
    if (node.nodeType === Node.TEXT_NODE) {
      const text = node.textContent;
      if (text.length === 0) return;
      content.push({ text, marks: marks.length ? [...marks] : undefined });
      return;
    }
    if (node.nodeType === Node.ELEMENT_NODE) {
      const tag = node.tagName.toLowerCase();
      // Remove script, style, iframe, img, svg, etc.
      if (["script","style","iframe","img","svg","video","audio","canvas","noscript","template"].includes(tag)) return;
      let newMarks = marks;
      if (tag === "strong" || tag === "b") newMarks = [...marks, { type: "bold" }];
      else if (tag === "em" || tag === "i") newMarks = [...marks, { type: "italic" }];
      else if (tag === "s" || tag === "strike") newMarks = [...marks, { type: "strike" }];
      else if (tag === "code") newMarks = [...marks, { type: "code" }];
      else if (tag === "a" && node.getAttribute("href")) {
        const href = node.getAttribute("href");
        if (isSafeHref(href)) newMarks = [...marks, { type: "link", href }];
      } else if (tag === "br") {
        content.push({ text: " ", marks: marks.length ? [...marks] : undefined });
        return;
      } else {
        // For block tags, handle separation
        const isBlock = ["p","div","li","ul","ol","h1","h2","h3","h4","h5","h6","blockquote","pre","section","header","footer","article","tr","td","th"].includes(tag);
        if (isBlock && content.length > 0) {
          const last = content[content.length - 1];
          if (last && !last.text.endsWith(" ")) {
            // Will insert space before next text via block handling at end
          }
        }
        for (const child of node.childNodes) walk(child, marks);
        if (isBlock) {
          if (content.length > 0) {
            const last = content[content.length - 1];
            if (last && !last.text.endsWith(" ")) {
              content.push({ text: " ", marks: undefined });
            }
          }
        }
        return;
      }
      for (const child of node.childNodes) walk(child, newMarks);
    }
  }
  for (const child of container.childNodes) walk(child, []);
  return normalizeRichText({ version: 1, content });
}

export function insertRichTextAtSelection(fieldEl, pastedRichText) {
  const offsets = selectionToOffsets(fieldEl);
  if (!offsets) return false;
  const current = domToRichText(fieldEl);
  // Split current at offsets and insert pasted
  let newContent = [];
  let pos = 0;
  let inserted = false;
  for (const run of current.content) {
    const runStart = pos;
    const runEnd = pos + run.text.length;
    pos = runEnd;
    if (!inserted && offsets.start >= runStart && offsets.start <= runEnd) {
      const beforeLen = offsets.start - runStart;
      const afterLen = runEnd - offsets.end;
      if (beforeLen > 0) newContent.push({ text: run.text.slice(0, beforeLen), ...(run.marks ? { marks: [...run.marks] } : {}) });
      // Insert pasted content
      for (const prun of pastedRichText.content) {
        newContent.push({ text: prun.text, ...(prun.marks ? { marks: [...prun.marks] } : {}) });
      }
      inserted = true;
      if (afterLen > 0) newContent.push({ text: run.text.slice(run.text.length - afterLen), ...(run.marks ? { marks: [...run.marks] } : {}) });
    } else if (inserted) {
      // If inserted, skip runs that were inside selection (already handled) else keep?
      // We already handled the run that contained start; for subsequent runs that are inside selection, skip
      if (runEnd <= offsets.start || runStart >= offsets.end) {
        // Actually after insertion, we should skip runs that were inside selection (except the one we already split)
        // But our loop above only handles the run containing start; we need to handle runs fully inside selection
        // For runs fully inside selection, they should be omitted (replaced)
        if (runStart >= offsets.start && runEnd <= offsets.end) {
          // inside selection, skip (already replaced)
          continue;
        }
        newContent.push({ text: run.text, ...(run.marks ? { marks: [...run.marks] } : {}) });
      } else if (runStart >= offsets.start && runStart < offsets.end) {
        // This case already handled? For runs after the start run but inside selection, skip
        continue;
      } else {
        newContent.push({ text: run.text, ...(run.marks ? { marks: [...run.marks] } : {}) });
      }
    } else {
      // Before insertion point, but need to check if run is before start, keep
      if (runEnd <= offsets.start) {
        newContent.push({ text: run.text, ...(run.marks ? { marks: [...run.marks] } : {}) });
      } else if (runStart >= offsets.start && runEnd <= offsets.end) {
        // This run is inside selection but we haven't inserted yet? This would happen if start is at boundary between runs
        // We haven't inserted yet, so we should insert before this run?
        if (!inserted) {
          for (const prun of pastedRichText.content) newContent.push({ text: prun.text, ...(prun.marks ? { marks: [...prun.marks] } : {}) });
          inserted = true;
        }
        // skip this run (inside selection)
        continue;
      } else {
        newContent.push({ text: run.text, ...(run.marks ? { marks: [...run.marks] } : {}) });
      }
    }
  }
  if (!inserted) {
    for (const prun of pastedRichText.content) newContent.push({ text: prun.text, ...(prun.marks ? { marks: [...prun.marks] } : {}) });
  }
  const normalized = normalizeRichText({ version: 1, content: newContent });
  renderRichTextToDOM(fieldEl, normalized);
  const newEnd = offsets.start + pastedRichText.content.reduce((acc, r) => acc + r.text.length, 0);
  restoreSelectionFromOffsets(fieldEl, newEnd, newEnd);
  return true;
}
