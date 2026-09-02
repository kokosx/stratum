// preview-morph.js — DOM morph for editor preview (V2)
// Small, focused morph that preserves stable nodes via keys.
// No framework, only native DOMParser + keyed diff.

export function parsePreviewDocument(html) {
  try {
    const parser = new DOMParser();
    const doc = parser.parseFromString(html, "text/html");
    if (!doc || !doc.documentElement) return null;
    return doc;
  } catch (_) {
    return null;
  }
}

function isStratumStartComment(node) {
  return node.nodeType === 8 && String(node.data || "").trim().startsWith("stratum-node-start:");
}
function isStratumEndComment(node) {
  return node.nodeType === 8 && String(node.data || "").trim().startsWith("stratum-node-end:");
}

function getStratumKeyFromStart(commentData) {
  const raw = String(commentData || "").trim();
  if (!raw.startsWith("stratum-node-start:")) return null;
  const payload = raw.slice("stratum-node-start:".length);
  const parts = payload.split(":");
  // InstanceKey may contain unescaped colon in test data, so find editable flag ("true"/"false") and treat everything between nodeId and it as instanceKey
  let editableIdx = -1;
  for (let i = 0; i < parts.length; i++) {
    if (parts[i] === "true" || parts[i] === "false") { editableIdx = i; break; }
  }
  if (editableIdx === -1) {
    if (parts.length < 2) return null;
    try {
      const nodeId = decodeURIComponent(parts[0]);
      const inst = decodeURIComponent(parts[1]);
      return `${nodeId}::${inst}`;
    } catch (_) {
      return `${parts[0]}::${parts[1]}`;
    }
  }
  const nodeId = parts[0];
  const instParts = parts.slice(1, editableIdx);
  const inst = instParts.join(":");
  try {
    return `${decodeURIComponent(nodeId)}::${decodeURIComponent(inst)}`;
  } catch (_) {
    return `${nodeId}::${inst}`;
  }
}

// Annotate block wrapper elements with data-stratum-key derived from surrounding markers.
// This gives stable identity for keyed morph without leaking extra metadata to public render
// (annotation is done in detached JS only, not in Go).
export function annotatePreviewDocument(doc) {
  if (!doc || !doc.body) return;
  // Walk comments
  const walker = doc.createTreeWalker(doc.body, NodeFilter.SHOW_COMMENT, null);
  const starts = [];
  let n;
  while ((n = walker.nextNode())) {
    const data = String(n.data || "").trim();
    if (data.startsWith("stratum-node-start:")) starts.push(n);
  }
  for (const start of starts) {
    const key = getStratumKeyFromStart(start.data);
    if (!key) continue;
    // Find consecutive element siblings after start until matching end
    let cur = start.nextSibling;
    // We need to match end for this start: find end with same nodeId/instanceKey
    // For simplicity, just annotate the first element sibling(s) before end.
    // Collect element siblings until we hit the matching end comment.
    // To find matching end, we can scan forward to find end with same key.
    // But we can just annotate the immediate next element(s) before hitting any stratum end/start at same level.
    // Approach: iterate siblings after start, stop when we encounter end with same key.
    const toAnnotate = [];
    let foundEnd = null;
    let probe = cur;
    while (probe) {
      if (probe.nodeType === 8) {
        const d = String(probe.data || "").trim();
        if (d.startsWith("stratum-node-end:")) {
          // Check if this end corresponds to this start (same nodeId/instanceKey) — join remainder to handle ":" in instanceKey
          const payload = d.slice("stratum-node-end:".length);
          const parts = payload.split(":");
          if (parts.length >= 2) {
            try {
              const eNode = decodeURIComponent(parts[0]);
              const eInst = decodeURIComponent(parts.slice(1).join(":"));
              const eKey = `${eNode}::${eInst}`;
              if (eKey === key) {
                foundEnd = probe;
                break;
              }
            } catch (_) {}
          }
          // If not matching, it's an inner block's end, continue (since nested)
          // But nested markers are inside wrapper, not siblings, so we won't encounter them here as siblings.
          // For parent, siblings after start are wrapper, then end, so we will find parent's end directly.
        } else if (d.startsWith("stratum-node-start:")) {
          // Another sibling block start at same level before end -> block with no wrapper? Means previous block had no wrapper element, skip.
          break;
        }
      }
      if (probe.nodeType === 1) {
        toAnnotate.push(probe);
        // If block wrapper is single element, we break after first element? But if block renders multiple top-level elements, we should annotate all until end.
        // Continue to collect all element siblings before end, so we annotate each.
      }
      probe = probe.nextSibling;
      // Safety: limit to avoid infinite loop; if we passed foundEnd, break.
      if (foundEnd) break;
    }
    for (const el of toAnnotate) {
      if (!el.hasAttribute("data-stratum-key")) {
        try { el.setAttribute("data-stratum-key", key); } catch (_) {}
      }
    }
  }
}

export function getNodeKey(node) {
  if (!node) return null;
  if (node.nodeType === 8) {
    const data = String(node.data || "").trim();
    if (data.startsWith("stratum-node-start:") || data.startsWith("stratum-node-end:")) {
      return `comment:${data}`;
    }
    // Generic comment key (preserve order but not crucial)
    // For generic comments, use data as key if non-empty, else null
    if (data) return `comment:generic:${data.slice(0, 80)}`;
    return null;
  }
  if (node.nodeType === 3) {
    return null;
  }
  if (node.nodeType === 1) {
    const el = node;
    // Editor block key
    if (el.hasAttribute("data-stratum-key")) {
      return `stratum:${el.getAttribute("data-stratum-key")}`;
    }
    if (el.hasAttribute("data-node-id")) {
      return `node:${el.getAttribute("data-node-id")}`;
    }
    if (el.id) {
      // For head elements, id is strong key, for body also
      return `id:${el.tagName.toLowerCase()}#${el.id}`;
    }
    // For head specific, generate deterministic key
    const tag = el.tagName ? el.tagName.toLowerCase() : "";
    if (el.ownerDocument && el.ownerDocument.head && el.ownerDocument.head.contains(el)) {
      const headKey = getHeadKey(el);
      if (headKey) return headKey;
    }
    // No stable key
    return null;
  }
  return null;
}

function getHeadKey(el) {
  const tag = el.tagName.toLowerCase();
  if (tag === "meta") {
    if (el.hasAttribute("charset")) return "meta:charset";
    if (el.hasAttribute("name")) return `meta:name=${el.getAttribute("name")}`;
    if (el.hasAttribute("property")) return `meta:property=${el.getAttribute("property")}`;
    if (el.hasAttribute("http-equiv")) return `meta:http-equiv=${el.getAttribute("http-equiv")}`;
    // fallback
    const name = el.getAttribute("name") || el.getAttribute("property") || "";
    if (name) return `meta:${name}`;
    return null;
  }
  if (tag === "link") {
    const rel = (el.getAttribute("rel") || "").toLowerCase();
    const href = el.getAttribute("href") || "";
    const as = el.getAttribute("as") || "";
    const type = el.getAttribute("type") || "";
    // For stylesheet, href is key
    if (href) return `link:${rel}:${href}:${as}:${type}`;
    if (el.id) return `link#${el.id}`;
    return `link:${rel}:${href}`;
  }
  if (tag === "style") {
    if (el.id) return `style#${el.id}`;
    if (el.hasAttribute("data-stratum-custom-code")) return `style:custom:${el.getAttribute("data-stratum-custom-code")}`;
    // For preview theme style with id stratum-preview-theme
    // If no id, use first 100 chars of content as part of key? But we want to preserve same style node even if content changes.
    // So return generic style key only if no id — but then we would treat all generic styles as same key, which is wrong.
    // Better to return null for generic style without id, so they are treated as unkeyed and matched positionally.
    return null;
  }
  if (tag === "title") return "title";
  if (tag === "script") {
    const src = el.getAttribute("src") || "";
    if (src) return `script:src=${src}`;
    if (el.id) return `script#${el.id}`;
    // Inline scripts without src/id are not keyed
    return null;
  }
  if (tag === "base") {
    const href = el.getAttribute("href") || "";
    return `base:${href}`;
  }
  if (el.id) return `${tag}#${el.id}`;
  return null;
}

function isSameType(a, b) {
  if (a.nodeType !== b.nodeType) return false;
  if (a.nodeType === 1) return a.tagName === b.tagName;
  if (a.nodeType === 3 || a.nodeType === 8) return true;
  return false;
}

function syncAttributes(oldEl, newEl) {
  if (oldEl.tagName !== newEl.tagName) return;
  // Remove old attrs not in new
  const oldAttrs = Array.from(oldEl.attributes);
  for (const attr of oldAttrs) {
    if (!newEl.hasAttribute(attr.name)) {
      try { oldEl.removeAttribute(attr.name); } catch (_) {}
    }
  }
  // Add/update new attrs
  const newAttrs = Array.from(newEl.attributes);
  for (const attr of newAttrs) {
    const oldVal = oldEl.getAttribute(attr.name);
    if (oldVal !== attr.value) {
      try { oldEl.setAttribute(attr.name, attr.value); } catch (_) {}
    }
  }
  // Special: for value/checked/prop sync delegated to syncFormState
}

function syncFormState(oldEl, newEl) {
  const tag = oldEl.tagName ? oldEl.tagName.toLowerCase() : "";
  if (tag === "input") {
    const type = (oldEl.getAttribute("type") || "").toLowerCase();
    if (type !== "file") {
      const newVal = newEl.getAttribute("value");
      const oldVal = oldEl.getAttribute("value");
      // Sync property if attribute value differs
      if (newVal !== oldVal) {
        // Preserve user entry if old had user-modified value that differs from new's default?
        // Server is source of truth for preview, so we sync.
        try {
          if (oldEl.value !== (newEl.value || newVal || "")) {
            oldEl.value = newEl.value || newVal || "";
          }
        } catch (_) {}
      }
      // checked
      const newChecked = newEl.hasAttribute("checked");
      if (oldEl.checked !== newChecked) {
        try { oldEl.checked = newChecked; } catch (_) {}
      }
      const newDisabled = newEl.hasAttribute("disabled");
      if (oldEl.disabled !== newDisabled) {
        try { oldEl.disabled = newDisabled; } catch (_) {}
      }
    }
  } else if (tag === "textarea") {
    const newVal = newEl.value !== undefined ? newEl.value : newEl.textContent;
    try {
      if (oldEl.value !== newVal) oldEl.value = newVal;
    } catch (_) {}
    // Also sync textContent child
    if (oldEl.textContent !== newEl.textContent) {
      try { oldEl.textContent = newEl.textContent; } catch (_) {}
    }
  } else if (tag === "select") {
    const newVal = newEl.value;
    try {
      if (oldEl.value !== newVal) oldEl.value = newVal;
    } catch (_) {}
    // Sync selected for options
    const oldOptions = oldEl.querySelectorAll ? oldEl.querySelectorAll("option") : [];
    const newOptions = newEl.querySelectorAll ? newEl.querySelectorAll("option") : [];
    for (let i = 0; i < Math.min(oldOptions.length, newOptions.length); i++) {
      const oOld = oldOptions[i];
      const oNew = newOptions[i];
      const sel = oNew.hasAttribute("selected");
      if (oOld.selected !== sel) {
        try { oOld.selected = sel; } catch (_) {}
        if (sel) oOld.setAttribute("selected", "");
        else oOld.removeAttribute("selected");
      }
    }
  } else if (tag === "option") {
    const sel = newEl.hasAttribute("selected");
    if (oldEl.selected !== sel) {
      try { oldEl.selected = sel; } catch (_) {}
    }
  }
}

function cloneNodeForImport(newNode, targetDoc) {
  try {
    return targetDoc.importNode(newNode, true);
  } catch (_) {
    // Fallback: create via outerHTML? For elements, use importNode, for text/comment create manually
    if (newNode.nodeType === 3) return targetDoc.createTextNode(newNode.nodeValue || "");
    if (newNode.nodeType === 8) return targetDoc.createComment(newNode.data || "");
    if (newNode.nodeType === 1) {
      const el = targetDoc.createElement(newNode.tagName);
      for (const attr of Array.from(newNode.attributes || [])) {
        try { el.setAttribute(attr.name, attr.value); } catch (_) {}
      }
      // Clone children shallow
      for (const child of Array.from(newNode.childNodes)) {
        try { el.appendChild(cloneNodeForImport(child, targetDoc)); } catch (_) {}
      }
      return el;
    }
    return targetDoc.importNode(newNode, true);
  }
}

function morphNode(oldNode, newNode) {
  if (oldNode.nodeType !== newNode.nodeType) {
    const replacement = cloneNodeForImport(newNode, oldNode.ownerDocument);
    try { oldNode.replaceWith(replacement); } catch (_) {
      try { oldNode.parentNode.replaceChild(replacement, oldNode); } catch (_) {}
    }
    return replacement;
  }
  if (oldNode.nodeType === 8) {
    if (oldNode.data !== newNode.data) {
      try { oldNode.data = newNode.data; } catch (_) {}
    }
    return oldNode;
  }
  if (oldNode.nodeType === 3) {
    if (oldNode.nodeValue !== newNode.nodeValue) {
      try { oldNode.nodeValue = newNode.nodeValue; } catch (_) {}
    }
    return oldNode;
  }
  if (oldNode.nodeType === 1) {
    if (oldNode.tagName !== newNode.tagName) {
      const replacement = cloneNodeForImport(newNode, oldNode.ownerDocument);
      try { oldNode.replaceWith(replacement); } catch (_) {
        try { oldNode.parentNode.replaceChild(replacement, oldNode); } catch (_) {}
      }
      return replacement;
    }
    syncAttributes(oldNode, newNode);
    // For style/script, also sync text content via children morph
    morphChildren(oldNode, newNode);
    syncFormState(oldNode, newNode);
    return oldNode;
  }
  // Other node types (document, etc.)
  return oldNode;
}

export function morphChildren(oldParent, newParent) {
  if (!oldParent || !newParent) return;
  // Build old keyed map
  const oldChildren = Array.from(oldParent.childNodes);
  const oldKeyMap = new Map();
  const oldKeyList = [];
  for (const child of oldChildren) {
    const key = getNodeKey(child);
    if (key != null) {
      oldKeyMap.set(key, child);
      oldKeyList.push(key);
    }
  }

  const newChildren = Array.from(newParent.childNodes);
  // For quick lookup of new keys set
  const newKeySet = new Set();
  for (const nc of newChildren) {
    const k = getNodeKey(nc);
    if (k != null) newKeySet.add(k);
  }

  // Iterate new children in order, ensuring oldParent has correct node at each position
  for (let i = 0; i < newChildren.length; i++) {
    const newChild = newChildren[i];
    const newKey = getNodeKey(newChild);
    let currentOldAtPos = oldParent.childNodes[i] || null;

    // If current old at position matches new key/type, morph it
    if (currentOldAtPos) {
      const curKey = getNodeKey(currentOldAtPos);
      if (newKey != null && curKey === newKey) {
        morphNode(currentOldAtPos, newChild);
        if (newKey != null) oldKeyMap.delete(newKey);
        continue;
      }
      if (newKey == null && curKey == null && isSameType(currentOldAtPos, newChild)) {
        morphNode(currentOldAtPos, newChild);
        continue;
      }
    }

    // Need to find matching old node elsewhere
    let matchedOld = null;
    if (newKey != null) {
      matchedOld = oldKeyMap.get(newKey) || null;
      if (matchedOld && matchedOld.parentNode !== oldParent) matchedOld = null; // already moved/removed
    } else {
      // For unkeyed, search forward for matching type unkeyed node
      // Look ahead in oldParent starting from i
      let candidate = currentOldAtPos;
      // If currentOldAtPos is keyed, skip it — we need unkeyed match
      // Search forward for unkeyed same type
      let search = currentOldAtPos;
      while (search) {
        const sKey = getNodeKey(search);
        if (sKey == null && isSameType(search, newChild)) {
          matchedOld = search;
          break;
        }
        search = search.nextSibling;
      }
      // If not found forward, search globally for any unkeyed matching that hasn't been placed yet
      // For simplicity, if not found forward, we will create new node
    }

    if (matchedOld) {
      // Move matched node to position i
      try {
        oldParent.insertBefore(matchedOld, currentOldAtPos);
      } catch (_) {}
      morphNode(matchedOld, newChild);
      if (newKey != null) oldKeyMap.delete(newKey);
    } else {
      // Create new node
      const imported = cloneNodeForImport(newChild, oldParent.ownerDocument);
      try {
        oldParent.insertBefore(imported, currentOldAtPos);
      } catch (_) {
        try { oldParent.appendChild(imported); } catch (_) {}
      }
      // If imported is element with children that could have been keyed elsewhere, our later iterations will handle moving those children?
      // But since we cloned deep, those children are fresh. For keyed children that existed elsewhere in oldParent's subtree (not just direct children), they are not at this level.
      // For top-level body children, cloning deep is fine for new blocks.
    }
  }

  // Remove leftover old nodes that are beyond newChildren length or keyed not in new
  // First remove keyed old nodes that were not matched (still in map and still in oldParent)
  for (const [key, node] of oldKeyMap) {
    if (node.parentNode === oldParent) {
      try { node.remove(); } catch (_) {
        try { oldParent.removeChild(node); } catch (_) {}
      }
    }
  }
  // Remove excess unkeyed old nodes beyond newChildren length
  // After positioning, oldParent should have newChildren.length + (preserved keyed not in new? already removed) + maybe overlay etc.
  // We need to remove any remaining old nodes at positions >= newChildren.length that are not preserved
  while (oldParent.childNodes.length > newChildren.length) {
    // The excess is at the end, but we must ensure we don't remove preserved overlay nodes that we detached earlier?
    // At this point, overlay has been detached, so remaining nodes are all part of server html.
    // Remove the last child if it is not needed. However our loop inserted new nodes at correct positions, so extra old nodes will be at the end or scattered.
    // Safer to iterate from end and remove nodes that are not at correct position and not matched.
    // For simplicity, if length > newChildren.length, remove the node at position newChildren.length (the first excess)
    const excess = oldParent.childNodes[newChildren.length];
    if (!excess) break;
    // Check if excess is keyed and should be preserved? But keyed excess already handled via oldKeyMap removal, so remaining excess must be unkeyed and truly extra.
    // However if newChildren has keyed nodes that caused moves, the old unkeyed nodes that were skipped may still be in middle, not at end.
    // Our earlier loop for unkeyed matching may have left some unkeyed old nodes that were not matched and are now beyond or in middle gaps.
    // We need a more robust cleanup: collect old children that are still in parent but their key is not in newKeySet and they are not at a position that matches new.
    // For now, just remove excess at end, and then do a second pass to remove any old unkeyed nodes that are not matched and still present but not in correct order.
    // To avoid complexity, we can do a final sweep: for any old child still in parent that is not at a position where new has same key/type, remove if it has no matching new.
    // Simpler: Iterate oldParent childNodes and if its key is not in newKeySet and it's unkeyed and we have more old than new, remove it if it doesn't match new at same index.
    // But this may be overkill for tests.

    // For now, remove the first node beyond newChildren.length that is not preserved.
    // Check if this excess node's key is in newKeySet — if yes, it should have been matched and kept, so this shouldn't happen.
    const k = getNodeKey(excess);
    if (k != null && newKeySet.has(k)) {
      // This shouldn't be excess; maybe our earlier move logic failed to place it correctly. Break to avoid infinite loop.
      break;
    }
    try { excess.remove(); } catch (_) {
      try { oldParent.removeChild(excess); } catch (_) { break; }
    }
    if (oldParent.childNodes.length <= newChildren.length) break;
    // To avoid infinite loop, limit iterations
    if (oldParent.childNodes.length > newChildren.length + 10) {
      // Fallback: just trim to length
      while (oldParent.childNodes.length > newChildren.length) {
        try { oldParent.lastChild.remove(); } catch (_) { break; }
      }
      break;
    }
  }

  // Final cleanup: remove any remaining old nodes (unkeyed) that are not at expected positions
  // For unkeyed nodes that were skipped, they remain in oldParent but should be removed if newChildren at that index is keyed different.
  // Our while loop already handled excess at end, but middle gaps remain.
  // Do a final pass: iterate oldParent children and compare to newChildren at same index; if old child's key not in new and type mismatch, and we have more old than new, we need to remove.
  // Simpler: If oldParent still has more children than newChildren after above, keep trimming.
  // For now, handle middle gaps by scanning.
  let idx = 0;
  // We need to ensure oldParent length equals newChildren length; if not, remove mismatched nodes from old
  // Use a bounded loop
  for (let safety = 0; safety < 100 && oldParent.childNodes.length > newChildren.length; safety++) {
    // Find first index where old and new mismatch and old is removable
    let mismatchIdx = -1;
    for (let j = 0; j < Math.min(oldParent.childNodes.length, newChildren.length); j++) {
      const oc = oldParent.childNodes[j];
      const nc = newChildren[j];
      const ok = getNodeKey(oc);
      const nk = getNodeKey(nc);
      if (ok !== nk) {
        // If nk is keyed and ok is unkeyed, or vice versa, this position is mismatched
        // Check if old node at j is unkeyed and could be removed
        if (ok == null && nk != null) {
          mismatchIdx = j;
          break;
        }
        if (ok != null && nk == null) {
          mismatchIdx = j;
          break;
        }
        if (ok == null && nk == null && !isSameType(oc, nc)) {
          mismatchIdx = j;
          break;
        }
      }
    }
    if (mismatchIdx === -1) {
      // No mismatch in overlapping range, so excess must be at end
      if (oldParent.childNodes.length > newChildren.length) {
        const toRemove = oldParent.childNodes[newChildren.length];
        if (toRemove) {
          try { toRemove.remove(); } catch (_) { break; }
        } else break;
      } else break;
    } else {
      const toRemove = oldParent.childNodes[mismatchIdx];
      // Only remove if this old node's key is not in new set (or unkeyed extra)
      const rk = getNodeKey(toRemove);
      if (rk == null || !newKeySet.has(rk)) {
        try { toRemove.remove(); } catch (_) { break; }
      } else {
        // This old keyed node is needed elsewhere, so we should have moved it earlier; break to avoid deleting needed node
        break;
      }
    }
  }
}

function syncHead(currentDoc, nextDoc) {
  if (!currentDoc.head || !nextDoc.head) return;
  // Ensure both heads are annotated (though head elements have ids, we use getHeadKey)
  morphChildren(currentDoc.head, nextDoc.head);
}

function syncDocumentAttributes(oldEl, newEl) {
  if (!oldEl || !newEl) return;
  syncAttributes(oldEl, newEl);
}

export function patchPreviewDocument(currentDoc, nextDoc) {
  if (!currentDoc || !nextDoc) return false;
  try {
    // Annotate both docs so block wrappers have stable keys
    annotatePreviewDocument(currentDoc);
    annotatePreviewDocument(nextDoc);

    // Preserve overlay and other editor injected nodes that are not in server HTML
    const oldBody = currentDoc.body;
    const preserved = [];
    if (oldBody) {
      // Collect nodes to preserve (editor overlay, etc.)
      const toPreserveSelectors = ["stratum-editor-overlay-root", "[data-stratum-overlay]", "[data-stratum-editor-ui]"];
      for (const sel of toPreserveSelectors) {
        try {
          const nodes = oldBody.querySelectorAll(sel);
          for (const n of nodes) {
            if (n && n.parentNode === oldBody) {
              preserved.push({ node: n, nextSibling: n.nextSibling });
              try { n.remove(); } catch (_) {}
            }
          }
        } catch (_) {}
      }
      // Also check direct children that are overlay hosts by tag name
      const direct = Array.from(oldBody.childNodes);
      for (const ch of direct) {
        if (ch.nodeType === 1 && ch.tagName && ch.tagName.toLowerCase() === "stratum-editor-overlay-root") {
          if (!preserved.find(p => p.node === ch)) {
            preserved.push({ node: ch, nextSibling: ch.nextSibling });
            try { ch.remove(); } catch (_) {}
          }
        }
      }
    }

    // Sync html element
    try { syncDocumentAttributes(currentDoc.documentElement, nextDoc.documentElement); } catch (_) {}

    // Sync head
    try { syncHead(currentDoc, nextDoc); } catch (_) {}

    // Sync body attributes
    try { syncAttributes(currentDoc.body, nextDoc.body); } catch (_) {}

    // Morph body children
    try { morphChildren(currentDoc.body, nextDoc.body); } catch (e) {
      // Fallback to full body replace if morph fails
      console.warn("[preview-morph] body morph failed, falling back", e);
      // Restore preserved before fallback?
      for (const p of preserved) {
        try { oldBody.appendChild(p.node); } catch (_) {}
      }
      return false;
    }

    // Restore preserved nodes (append at end, overlay expects to be at end)
    for (const p of preserved) {
      try { oldBody.appendChild(p.node); } catch (_) {}
    }

    // Dispatch patched event
    try {
      currentDoc.dispatchEvent(new CustomEvent("stratum:preview-patched", { bubbles: false }));
      if (currentDoc.defaultView) {
        currentDoc.defaultView.dispatchEvent(new CustomEvent("stratum:preview-patched", { bubbles: false }));
      }
    } catch (_) {}

    return true;
  } catch (e) {
    console.warn("[preview-morph] patch failed", e);
    return false;
  }
}

export function isPreviewInitialized(iframe) {
  try {
    if (!iframe) return false;
    if (iframe.dataset && iframe.dataset.previewInitialized === "1") return true;
    const doc = iframe.contentDocument;
    if (!doc || !doc.documentElement) return false;
    // Check if doc has content beyond loading placeholder
    if (doc.body && doc.body.childNodes.length > 0) {
      // If body contains at least one stratum marker, consider initialized
      const html = doc.documentElement.outerHTML || "";
      if (html.includes("stratum-node-start")) return true;
      // Fallback: if body has meaningful content
      if (doc.body.textContent && doc.body.textContent.trim().length > 0) return true;
    }
    return false;
  } catch (_) { return false; }
}

export function markPreviewInitialized(iframe) {
  try { if (iframe && iframe.dataset) iframe.dataset.previewInitialized = "1"; } catch (_) {}
}

export function fallbackReplacePreview(iframe, html) {
  try {
    // For V2, this is the recovery path: full srcdoc replace
    iframe.srcdoc = html;
    // Reset initialized flag; onload will re-mark
    try { delete iframe.dataset.previewInitialized; } catch (_) {}
    return true;
  } catch (e) {
    console.error("[preview-morph] fallback failed", e);
    return false;
  }
}
