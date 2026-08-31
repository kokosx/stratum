// markers.js — parser + index for renderer instrumentation
// Renderer format (Go): <!-- stratum-node-start:nodeID:instanceKey:editable:block:version[:ownerType:ownerId[:ownerLabel]] -->
// nodeID, instanceKey and block are url.PathEscape'd; instanceKey contains "/" and ":" so it is split via editable token.
// block is always present (e.g. core%2Fbutton), version is integer. Old markers without block are still tolerated.

function decodeSafe(s) {
  try {
    return decodeURIComponent(s);
  } catch (_) {
    return s;
  }
}

// Parse a single start comment data string (already trimmed, without surrounding "<!--" "-->")
// data is like "stratum-node-start:sec1:root%2Fnode%3Asec1:true:core%2Fsection:2" or with owners
export function parseStartComment(data) {
  if (!data || typeof data !== "string") return null;
  const prefix = "stratum-node-start:";
  if (!data.startsWith(prefix)) return null;
  const payload = data.slice(prefix.length);
  const parts = payload.split(":");
  if (parts.length < 3) return null;
  // Find editable token (first "true"/"false")
  let editableIdx = -1;
  for (let i = 1; i < parts.length; i++) {
    if (parts[i] === "true" || parts[i] === "false") {
      editableIdx = i;
      break;
    }
  }
  if (editableIdx === -1) return null;
  let nodeId = "";
  try {
    nodeId = decodeSafe(parts[0]);
  } catch (_) {
    nodeId = parts[0];
  }
  const keyParts = parts.slice(1, editableIdx);
  let instanceKey = keyParts.join(":");
  instanceKey = decodeSafe(instanceKey);
  const editable = parts[editableIdx] === "true";
  let block = "";
  let version = 0;
  let ownerType = "";
  let ownerId = "";
  let ownerLabel = "";
  const remaining = parts.length - editableIdx - 1;
  // Detect new format with block:version after editable (block must match blockNamePattern)
  const blockPattern = /^[a-z0-9][a-z0-9_-]*\/[a-z0-9][a-z0-9_-]*$/;
  if (remaining >= 2) {
    const candBlock = decodeSafe(parts[editableIdx + 1]);
    const candVersionStr = parts[editableIdx + 2];
    const isBlock = blockPattern.test(candBlock) && /^[1-9][0-9]*$/.test(candVersionStr);
    if (isBlock) {
      block = candBlock;
      version = parseInt(candVersionStr, 10) || 0;
      if (remaining >= 4) {
        ownerType = decodeSafe(parts[editableIdx + 3]);
        ownerId = decodeSafe(parts[editableIdx + 4]);
      }
      if (remaining >= 5) {
        ownerLabel = decodeSafe(parts[editableIdx + 5]);
      }
    } else {
      // Legacy: no block/version, remaining are owner fields
      ownerType = decodeSafe(parts[editableIdx + 1]);
      if (remaining >= 2) ownerId = decodeSafe(parts[editableIdx + 2]);
      if (remaining >= 3) ownerLabel = decodeSafe(parts[editableIdx + 3]);
    }
  }
  return { nodeId, instanceKey, editable, block, version, ownerType, ownerId, ownerLabel };
}

export function parseEndComment(data) {
  if (!data || typeof data !== "string") return null;
  const prefix = "stratum-node-end:";
  if (!data.startsWith(prefix)) return null;
  const payload = data.slice(prefix.length);
  const parts = payload.split(":");
  if (parts.length < 2) return null;
  // nodeId is first part, remainder is instanceKey (may contain colons)
  let nodeId = decodeSafe(parts[0]);
  let instanceKey = decodeSafe(parts.slice(1).join(":"));
  return { nodeId, instanceKey };
}

// Build index from iframe document.
// Returns { index: Map<instanceKey, RenderedNodeInstance>, elementToNode: WeakMap<Element, RenderedNodeInstance> }
// RenderedNodeInstance = { nodeId, instanceKey, block, version, editable, ownerType, ownerId, ownerLabel, rootElements: Element[], visualElement: Element|null }
export function buildMarkerIndex(doc) {
  const index = new Map();
  const elementToNode = new WeakMap();
  const nodeToKeys = new Map(); // nodeId -> [instanceKey]

  if (!doc || !doc.body) {
    return { index, elementToNode, nodeToKeys, instances: [] };
  }

  // First pass: walk comments to collect stack and instance metadata
  // We use TreeWalker over body with SHOW_COMMENT | SHOW_ELEMENT, but need to maintain stack order.
  // Simplest: walk all nodes via TreeWalker(SHOW_ALL) without filter, handling types.
  const stack = [];
  // Use NodeFilter.SHOW_ALL = 0xFFFFFFFF, fallback for cross-window
  const NF_SHOW_ALL = (typeof NodeFilter !== "undefined" && NodeFilter.SHOW_ALL) || 0xffffffff;
  const NODE_COMMENT = (typeof Node !== "undefined" && Node.COMMENT_NODE) || 8;
  const NODE_ELEMENT = (typeof Node !== "undefined" && Node.ELEMENT_NODE) || 1;
  const walker = doc.createTreeWalker(doc.body, NF_SHOW_ALL, null);

  // We need to handle nodes in document order. The walker set to body will visit body itself first.
  // We'll iterate with nextNode().
  // To correctly map ELEMENT -> deepest stack entry, we push on start comment, assign on element, pop on end.
  let node;

  while ((node = walker.nextNode())) {
    if (node.nodeType === NODE_COMMENT) {
      const data = (node.data || "").trim();
      if (data.startsWith("stratum-node-start:")) {
        const parsed = parseStartComment(data);
        if (!parsed) continue;
        const info = {
          nodeId: parsed.nodeId,
          instanceKey: parsed.instanceKey,
          editable: parsed.editable,
          block: parsed.block || "",
          version: parsed.version || 0,
          ownerType: parsed.ownerType,
          ownerId: parsed.ownerId,
          ownerLabel: parsed.ownerLabel,
          startComment: node,
          depth: stack.length,
          rootElements: [],
          visualElement: null,
        };
        stack.push(info);
        // Keep reference for later end
        // pending map removed — stack is source of truth
      } else if (data.startsWith("stratum-node-end:")) {
        const parsed = parseEndComment(data);
        if (!parsed) continue;
        // Find matching start in stack (search from top)
        let idx = stack.length - 1;
        while (idx >= 0) {
          if (stack[idx].nodeId === parsed.nodeId && stack[idx].instanceKey === parsed.instanceKey) break;
          idx--;
        }
        if (idx < 0) continue;
        const startInfo = stack[idx];
        // Build RenderedNodeInstance
        // rootElements already collected
        const instance = {
          nodeId: startInfo.nodeId,
          instanceKey: startInfo.instanceKey,
          editable: startInfo.editable,
          block: startInfo.block || "",
          version: startInfo.version || 0,
          ownerType: startInfo.ownerType,
          ownerId: startInfo.ownerId,
          ownerLabel: startInfo.ownerLabel,
          rootElements: startInfo.rootElements.slice(),
          visualElement: startInfo.visualElement || null,
        };
        index.set(startInfo.instanceKey, instance);
        if (!nodeToKeys.has(startInfo.nodeId)) nodeToKeys.set(startInfo.nodeId, []);
        nodeToKeys.get(startInfo.nodeId).push(startInfo.instanceKey);
        // Remove from stack
        stack.splice(idx, 1);
      }
    } else if (node.nodeType === NODE_ELEMENT) {
      if (stack.length === 0) continue;
      // Skip overlay host itself if present (we inject later, but during rebuild it may exist)
      if (node.tagName && node.tagName.toLowerCase() === "stratum-editor-overlay-root") continue;
      const deepest = stack[stack.length - 1];
      // Map element to deepest node
      elementToNode.set(node, deepest);
      // Determine rootElements: if parent not mapped to same instance, it's root
      let isRoot = true;
      const parent = node.parentElement;
      if (parent && elementToNode.has(parent)) {
        const parentInfo = elementToNode.get(parent);
        if (parentInfo && parentInfo.instanceKey === deepest.instanceKey) {
          isRoot = false;
        }
      }
      if (isRoot) {
        deepest.rootElements.push(node);
      }
    }
  }

  // Fallback: any stack remaining without end marker (malformed) — treat as instance with current roots
  for (const leftover of stack) {
    if (!index.has(leftover.instanceKey)) {
      const instance = {
        nodeId: leftover.nodeId,
        instanceKey: leftover.instanceKey,
        editable: leftover.editable,
        block: leftover.block || "",
        version: leftover.version || 0,
        ownerType: leftover.ownerType,
        ownerId: leftover.ownerId,
        ownerLabel: leftover.ownerLabel,
        rootElements: leftover.rootElements.slice(),
        visualElement: leftover.visualElement || null,
      };
      index.set(leftover.instanceKey, instance);
      if (!nodeToKeys.has(leftover.nodeId)) nodeToKeys.set(leftover.nodeId, []);
      nodeToKeys.get(leftover.nodeId).push(leftover.instanceKey);
    }
  }

  return { index, elementToNode, nodeToKeys, instances: Array.from(index.values()) };
}

// Find visual element inside same instance. Selector is block-defined visualRoot.
// Must not escape into nested child instance: candidate's deepest owner must be same instanceKey.
export function findVisualElementForInstance(instance, selector, elementToNode) {
  if (!instance || !selector || typeof selector !== "string") return null;
  const sel = selector.trim();
  if (sel === "") return null;
  if (!instance.rootElements || instance.rootElements.length === 0) return null;
  for (const root of instance.rootElements) {
    if (!root) continue;
    try {
      if (root.matches && typeof root.matches === "function" && root.matches(sel)) {
        const owner = elementToNode ? elementToNode.get(root) : null;
        if (!owner || owner.instanceKey === instance.instanceKey) return root;
      }
    } catch (_) {
      return null; // invalid selector -> fallback
    }
    try {
      const el = root.querySelector(sel);
      if (el) {
        const owner = elementToNode ? elementToNode.get(el) : null;
        if (!owner || owner.instanceKey === instance.instanceKey) return el;
        // If first match belongs to nested instance, try next distinct match via querySelectorAll fallback
        const list = root.querySelectorAll(sel);
        for (const cand of list) {
          const o = elementToNode ? elementToNode.get(cand) : null;
          if (!o || o.instanceKey === instance.instanceKey) return cand;
        }
      }
    } catch (_) {
      return null; // SyntaxError -> invalid selector, fallback
    }
  }
  return null;
}

// Resolve visualRoot for all instances using a lookup function.
// getVisualRoot(block, version) => selector string | "".
// Lazily cached per instance; call once after buildMarkerIndex. Invalid/non-matching falls back to natural bounds.
export function resolveVisualElements(index, elementToNode, getVisualRoot) {
  if (!index || typeof getVisualRoot !== "function") return;
  for (const inst of index.values()) {
    if (inst.visualElement) continue; // already resolved
    let selector = "";
    try {
      selector = getVisualRoot(inst.block, inst.version) || "";
    } catch (_) {
      selector = "";
    }
    if (!selector) continue;
    const el = findVisualElementForInstance(inst, selector, elementToNode);
    if (el) inst.visualElement = el;
  }
}

// Visual bounds helpers

export function unionRects(rects) {
  if (!rects || rects.length === 0) return null;
  // Filter out empty synthetic rects unless all are empty
  const clean = rects.filter((r) => r && (r.width > 0 || r.height > 0));
  const src = clean.length ? clean : rects;
  if (!src.length) return null;
  let l = src[0].left;
  let t = src[0].top;
  let r = src[0].right !== undefined ? src[0].right : src[0].left + src[0].width;
  let b = src[0].bottom !== undefined ? src[0].bottom : src[0].top + src[0].height;
  for (let i = 1; i < src.length; i++) {
    const rr = src[i];
    const rl = rr.left;
    const rt = rr.top;
    const rr2 = rr.right !== undefined ? rr.right : rr.left + rr.width;
    const rb = rr.bottom !== undefined ? rr.bottom : rr.top + rr.height;
    if (rl < l) l = rl;
    if (rt < t) t = rt;
    if (rr2 > r) r = rr2;
    if (rb > b) b = rb;
  }
  return { left: l, top: t, right: r, bottom: b, width: r - l, height: b - t };
}

export function visualRectForInstance(instance, doc) {
  if (!instance) return null;
  // Prefer editor visual root if present — actual widget bounds, not technical wrapper.
  if (instance.visualElement && typeof instance.visualElement.getBoundingClientRect === "function") {
    if (!doc || doc.contains(instance.visualElement)) {
      try {
        const r = instance.visualElement.getBoundingClientRect();
        if (r && (r.width > 0 || r.height > 0)) return r;
      } catch (_) {}
    }
  }
  if (!instance.rootElements || instance.rootElements.length === 0) {
    // No rendered output — M2: do not show synthetic placeholder
    return null;
  }
  // Filter out elements not in DOM or display:none
  const rects = [];
  for (const el of instance.rootElements) {
    if (!el || typeof el.getBoundingClientRect !== "function") continue;
    // Skip if detached
    if (!doc || !doc.contains(el)) continue;
    try {
      const r = el.getBoundingClientRect();
      if (r && (r.width > 0 || r.height > 0)) {
        rects.push(r);
      } else if (r) {
        // Still consider zero-height but maybe hidden — ignore
      }
    } catch (_) {}
  }
  if (rects.length === 0) {
    // Edge: text-only block may have no element rect but has Range?
    // Attempt Range over first root's parent? If no rects, try Range fallback for single text node?
    // For M2, we keep Range as fallback only for multi-root/text-only where needed.
    // Try to create range spanning all rootElements' contents
    try {
      if (instance.rootElements.length === 1) {
        const el = instance.rootElements[0];
        const range = doc.createRange();
        range.selectNodeContents(el);
        const r = range.getBoundingClientRect();
        if (r && (r.width > 0 || r.height > 0)) return r;
        const rects2 = Array.from(range.getClientRects());
        if (rects2.length) return unionRects(rects2);
      }
    } catch (_) {}
    return null;
  }
  if (rects.length === 1) return rects[0];
  return unionRects(rects);
}
