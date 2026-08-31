// state.js — V2 minimal state, no global spaghetti
const bootstrapEl = document.getElementById("editor-v2-bootstrap");
let raw = {};
try {
  raw = bootstrapEl ? JSON.parse(bootstrapEl.textContent || "{}") : {};
} catch (_) {
  raw = {};
}

export const bootstrap = raw;

function normalizePathname(pathname) {
  if (!pathname || pathname === "") return "/";
  let p = pathname.trim();
  if (!p.startsWith("/")) p = "/" + p;
  // Collapse trailing slashes except root
  p = p.replace(/\/+$/, "");
  if (p === "") p = "/";
  return p;
}

function derivePublicInfo() {
  // Prefer bootstrap.actions.publicPreviewUrl (absolute public URL from entryEditorStatus)
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
  // Also check resource.location if present
  if (!rawUrl && raw.resource?.location) rawUrl = String(raw.resource.location);
  if (!rawUrl && bootstrap.resource?.location) rawUrl = String(bootstrap.resource.location);

  let origin = "";
  let pathname = "/";
  let search = "";
  let publicUrl = rawUrl;
  try {
    if (rawUrl) {
      const u = new URL(rawUrl, window.location.origin);
      origin = u.origin;
      pathname = normalizePathname(u.pathname);
      search = u.search || "";
      publicUrl = u.origin + pathname + search;
      // Keep hash out of publicUrl base
    } else {
      // Fallback to slug
      const slug = raw.slug || bootstrap.slug || raw.resource?.slug || "";
      if (slug) {
        pathname = normalizePathname("/" + String(slug).replace(/^\/+/, ""));
      }
      origin = window.location.origin;
      publicUrl = origin + pathname;
    }
  } catch (_) {
    const slug = raw.slug || bootstrap.slug || "";
    if (slug) pathname = normalizePathname("/" + String(slug).replace(/^\/+/, ""));
    origin = window.location.origin;
    publicUrl = origin + pathname;
  }
  // If slug fallback produced just "/", but we have entry still, keep "/"
  return { publicUrl, publicPath: pathname, publicSearch: search, publicOrigin: origin };
}

const derived = derivePublicInfo();

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
  // public URL info for same-page anchor resolution (source of truth, not iframe location)
  publicUrl: derived.publicUrl,
  publicPath: derived.publicPath,
  publicSearch: derived.publicSearch,
  publicOrigin: derived.publicOrigin,
};
