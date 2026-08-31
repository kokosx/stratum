// state.js — V2 minimal state, no global spaghetti
const bootstrapEl = document.getElementById("editor-v2-bootstrap");
let raw = {};
try {
  raw = bootstrapEl ? JSON.parse(bootstrapEl.textContent || "{}") : {};
} catch (_) {
  raw = {};
}

export const bootstrap = raw;

function derivePublicInfo() {
  const candidates = [
    raw.actions?.publicPreviewUrl,
    raw.actions?.publicPreviewURL,
    raw.actions?.PublicPreviewURL,
    raw.publicUrl,
    raw.PublicURL,
    bootstrap.publicUrl,
    raw.publicPreviewUrl,
  ];
  let rawUrl = "";
  for (const c of candidates) {
    if (typeof c === "string" && c.trim() !== "") {
      rawUrl = c.trim();
      break;
    }
  }
  if (!rawUrl && raw.resource?.location) rawUrl = String(raw.resource.location);
  if (!rawUrl && bootstrap.resource?.location) rawUrl = String(bootstrap.resource.location);

  let publicUrl = rawUrl;
  try {
    if (rawUrl) {
      const u = new URL(rawUrl, window.location.origin);
      publicUrl = u.origin + u.pathname + u.search;
    } else {
      const slug = raw.slug || bootstrap.slug || raw.resource?.slug || "";
      if (slug) {
        let p = String(slug).trim();
        if (!p.startsWith("/")) p = "/" + p;
        p = p.replace(/\/+$/, "") || "/";
        publicUrl = window.location.origin + p;
      } else {
        publicUrl = window.location.origin + "/";
      }
    }
  } catch (_) {
    publicUrl = rawUrl || window.location.origin + "/";
  }
  return { publicUrl };
}

const derived = derivePublicInfo();

// selection is pure runtime, does not mutate SDT here
// { nodeId, instanceKey, editable, ownerType, ownerId, ownerLabel } or null
function createSelectionState() {
  return { current: null, hoveredKey: null };
}

const sel = createSelectionState();

export const state = {
  // generic resource descriptor, M1 focuses on entry/page but keeps shape generic
  resource: bootstrap.resource || {},
  // viewport: desktop | tablet | mobile
  viewport: "desktop",
  // preview endpoint
  previewUrl: bootstrap.previewUrl || bootstrap.actions?.previewUrl || "/admin/editor/preview",
  // document tree (SDT) from bootstrap
  document: bootstrap.document || { version: 1, nodes: [] },
  // metadata fallback from bootstrap top-level
  title: bootstrap.title || bootstrap.resource?.label || "",
  slug: bootstrap.slug || "",
  excerpt: bootstrap.excerpt || "",
  contentTypeId: bootstrap.contentTypeId || bootstrap.resource?.contentTypeId || "",
  entryId: bootstrap.entryId || bootstrap.resource?.id || "",
  csrfToken: bootstrap.csrfToken || "",
  // public URL for View Live / bootstrap
  publicUrl: derived.publicUrl,
  // M2 selection
  get selection() {
    return sel.current;
  },
  set selection(v) {
    sel.current = v;
  },
  get hoveredKey() {
    return sel.hoveredKey;
  },
  set hoveredKey(v) {
    sel.hoveredKey = v;
  },
};

// Helpers for displayName lookup (block id -> displayName)
// Uses bootstrap catalog + definitions if present, falls back to minimal map.
// NEVER returns technical IDs — always friendly.
const displayNameCache = new Map();

function humanizeBlockName(block) {
  if (!block || typeof block !== "string") return "";
  let name = block.includes("/") ? block.split("/").pop() : block;
  // handle kebab/sneak
  name = name.replace(/[_-]+/g, " ");
  // insert space before capitals for camelCase (rare)
  name = name.replace(/([a-z])([A-Z])/g, "$1 $2");
  return name
    .split(" ")
    .filter(Boolean)
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase())
    .join(" ");
}

function buildDisplayNameMap() {
  if (displayNameCache.size) return displayNameCache;
  const candidates = [];
  if (Array.isArray(bootstrap.catalog)) candidates.push(...bootstrap.catalog);
  if (Array.isArray(bootstrap.definitions)) candidates.push(...bootstrap.definitions);
  // catalog entries are {block, version, displayName}
  for (const item of candidates) {
    if (!item || !item.block) continue;
    // keep latest by block name (if multiple versions, larger version wins)
    const existing = displayNameCache.get(item.block);
    const disp = item.displayName && String(item.displayName).trim() ? String(item.displayName).trim() : "";
    const friendly = disp || humanizeBlockName(item.block);
    if (!existing || (item.version && existing.version < item.version)) {
      displayNameCache.set(item.block, { displayName: friendly, version: item.version });
    }
  }
  // Semantic fallback for known core blocks if no catalog provided or missing displayName
  const fallback = {
    "core/section": "Section",
    "core/stack": "Stack",
    "core/grid": "Grid",
    "core/heading": "Heading",
    "core/text": "Text",
    "core/button": "Button",
    "core/image": "Image",
    "core/collection": "Collection",
    "core/accordion": "Accordion",
    "core/navigation": "Navigation",
    "core/site-part": "Site Part",
    "core/entry-field": "Entry Field",
    "core/entry-title": "Entry Title",
    "core/entry-excerpt": "Entry Excerpt",
    "core/entry-content": "Entry Content",
    "core/entry-media": "Entry Media",
    "core/featured-image": "Featured Image",
    "core/content-slot": "Page Content",
    "core/archive-title": "Archive Title",
    "core/archive-description": "Archive Description",
    "core/site-name": "Site Name",
    "core/site-logo": "Site Logo",
    "core/form": "Form",
    "core/card": "Card",
  };
  for (const [k, v] of Object.entries(fallback)) {
    if (!displayNameCache.has(k)) displayNameCache.set(k, { displayName: v, version: 0 });
  }
  return displayNameCache;
}

export function displayNameForBlock(blockName) {
  if (!blockName || typeof blockName !== "string") return "Block";
  // Never treat technical IDs (blk_*, uuid, AaGwQ...) as block names — they lack "/"
  if (!blockName.includes("/")) {
    // Might be a technical nodeId passed by mistake — never show it
    return "Block";
  }
  const map = buildDisplayNameMap();
  const entry = map.get(blockName);
  if (entry && entry.displayName) return entry.displayName;
  // humanized fallback
  const hum = humanizeBlockName(blockName);
  return hum || "Block";
}

export function friendlyLabelForUnknown() {
  return "Template element";
}

export function clearSelection() {
  sel.current = null;
}
export function setSelection(selObj) {
  sel.current = selObj;
}
