// state.js — V2 minimal state, no global spaghetti
const bootstrapEl = document.getElementById("editor-v2-bootstrap");
let raw = {};
try {
  raw = bootstrapEl ? JSON.parse(bootstrapEl.textContent || "{}") : {};
} catch (_) {
  raw = {};
}

export const bootstrap = raw;

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
};
