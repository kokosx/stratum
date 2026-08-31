// preview.js — V2 preview fetcher, thin wrapper over existing /admin/editor/preview pipeline
import { state, bootstrap } from "./state.js";

let pendingController = null;

function csrfToken() {
  if (state.csrfToken) return state.csrfToken;
  const meta = document.querySelector('meta[name="csrf-token"]');
  if (meta) return meta.getAttribute("content") || "";
  return "";
}

export async function fetchPreview(signal) {
  const csrf = csrfToken();
  const doc = state.document;
  const payload = {
    csrf_token: csrf,
    document_json: JSON.stringify(doc),
    title: state.title || "",
    slug: state.slug || "",
    excerpt: state.excerpt || "",
    entry_id: state.entryId || "",
    content_type_id: state.contentTypeId || "",
  };

  // include optional hidden inputs if present (more accurate slug/title/excerpt)
  try {
    const titleEl = document.getElementById("editor-v2-title");
    if (titleEl) payload.title = titleEl.value || payload.title;
    const slugEl = document.getElementById("editor-v2-slug");
    if (slugEl) payload.slug = slugEl.value || payload.slug;
    const excerptEl = document.getElementById("editor-v2-excerpt");
    if (excerptEl) payload.excerpt = excerptEl.value || payload.excerpt;
    const contentTypeEl = document.getElementById("editor-v2-content-type");
    if (contentTypeEl && contentTypeEl.value) payload.content_type_id = contentTypeEl.value;
    const entryEl = document.getElementById("editor-v2-entry-id");
    if (entryEl && entryEl.value) payload.entry_id = entryEl.value;
  } catch (_) {}

  const params = new URLSearchParams(payload);

  // abort previous
  if (pendingController) {
    try {
      pendingController.abort();
    } catch (_) {}
  }
  pendingController = new AbortController();
  const combinedSignal = signal || pendingController.signal;

  const url = new URL(state.previewUrl, window.location.origin);
  // M2: request editor instrumentation markers (existing backend mode)
  url.searchParams.set("editor_canvas", "1");

  const response = await fetch(url.toString(), {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      "X-CSRF-Token": csrf,
      "X-Requested-With": "StratumEditor",
      "X-Stratum-Editor-Canvas": "1",
    },
    body: params,
    signal: combinedSignal,
  });

  const text = await response.text();
  if (!response.ok) {
    throw new Error(text.trim() || "Preview failed");
  }
  return text;
}
