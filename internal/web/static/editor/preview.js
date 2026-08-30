// preview.js — preview POST, debounce, iframe srcdoc refresh
import { state, bootstrap } from "./state.js";
import { isDirtyNow } from "./state.js";

let previewTimer = null;

export function schedulePreview() {
  clearTimeout(previewTimer);
  previewTimer = setTimeout(updatePreview, 400);
}

function getMetaParams() {
  const titleEl = document.getElementById("entry-title");
  const excerptEl = document.getElementById("entry-excerpt");
  const slugEl = document.getElementById("entry-slug");
  const seoTitleEl = document.getElementById("entry-seo-title");
  const seoDescEl = document.getElementById("entry-seo-description");
  const entryIdEl = document.getElementById("entry-id");
  const layoutEl = document.getElementById("entry-layout-template");
  const ctEl = document.getElementById("entry-content-type");
  const featuredEl = document.getElementById("entry-featured-media-id");
  const archiveCtxEl = document.getElementById("archive-preview-context");
  return { titleEl, excerptEl, slugEl, seoTitleEl, seoDescEl, entryIdEl, layoutEl, ctEl, featuredEl, archiveCtxEl };
}

export async function updatePreview() {
  const previewElement = document.getElementById("editor-preview");
  const legacyFallback = document.getElementById("editor-legacy-fallback");
  const errorElement = document.getElementById("editor-error");
  const form = document.getElementById("stratum-editor-form");
  if (!form) return;
  const { titleEl, excerptEl, slugEl, seoTitleEl, seoDescEl, entryIdEl, layoutEl, ctEl, featuredEl, archiveCtxEl } = getMetaParams();

  const params = {
    csrf_token: form.elements.csrf_token ? form.elements.csrf_token.value : "",
    document_json: JSON.stringify(state.document),
    title: titleEl?.value || "",
    excerpt: excerptEl?.value || "",
    slug: slugEl?.value || "",
    seo_title: seoTitleEl?.value || "",
    seo_description: seoDescEl?.value || "",
    entry_id: entryIdEl?.value || "",
    layout_template_id: layoutEl?.value || "",
    content_type_id: ctEl?.value || bootstrap.contentTypeId || bootstrap.resource?.contentTypeId || "",
    featured_media_id: featuredEl?.value || "",
  };
  if (archiveCtxEl && archiveCtxEl.value) {
    const parts = archiveCtxEl.value.split(":");
    if (parts.length === 2) {
      params.preview_taxonomy_id = parts[0];
      params.preview_term_id = parts[1];
    }
  }
  // include EditorCanvas flag so renderer adds markers
  params.editor_canvas = "1";
  // collect instance scope for editable detection — let server know primary IDs
  try {
    const ids = [];
    function collect(nodes) {
      for (const n of nodes) {
        ids.push(n.id);
        if (n.children) collect(n.children);
      }
    }
    collect(state.document.nodes);
    params.editable_ids = ids.join(",");
    if (bootstrap.resource) {
      params.primary_type = bootstrap.resource.type || "";
      params.primary_id = bootstrap.resource.id || "";
    }
  } catch (_) {}

  const previewParams = new URLSearchParams(params);
  try { new FormData(form).forEach((value, key) => {
    if (key.startsWith("field_")) previewParams.append(key, value);
  }); } catch (_) {}

  const previewUrl = bootstrap.previewUrl || bootstrap.actions?.previewUrl || "/admin/editor/preview";
  const url = new URL(previewUrl, window.location.origin);
  // For canvas, always use /admin/editor/preview with editor_canvas=1
  // Layout/site-part previews have separate URLs but unified preview should use main endpoint with composition
  // Detect if previewUrl is site-part/template specific — keep it for now, but add editor_canvas param
  url.searchParams.set("editor_canvas", "1");

  try {
    const response = await fetch(url.toString(), {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Requested-With": "StratumEditor" },
      body: previewParams,
    });
    const output = await response.text();
    if (!response.ok) throw new Error(output.trim() || "Preview failed");
    // Dual path: legacy preview element + canvas iframe
    if (previewElement) {
      previewElement.classList.remove("editor-preview-error");
      // Keep legacy hidden but still populate for fallback
    }
    const canvas = document.getElementById("editor-canvas");
    if (canvas) {
      canvas.dataset.previewLoaded = "0";
      canvas.onload = () => {
        canvas.dataset.previewLoaded = "1";
        try {
          if (canvas.contentDocument && canvas.contentDocument.body) {
            // auto height fallback handled in canvas.js
          }
        } catch (_) {}
        if (window.__stratum_canvasController) {
          // let controller parse after load
          setTimeout(() => window.__stratum_canvasController.refresh(), 50);
        }
        // preserve selection after rerender
        if (state.selectedNodeId && window.__stratum_canvasController) {
          const inst = state.selectedInstanceKey;
          window.__stratum_canvasController.selectNode(state.selectedNodeId, inst, { scroll: false });
        }
      };
      // Install the lifecycle hook before replacing the iframe document: srcdoc
      // can load immediately in a warm browser cache.
      canvas.srcdoc = output;
      // If already loaded (srcdoc immediate), trigger refresh manually after tick
      setTimeout(() => {
        if (canvas.contentDocument && canvas.contentDocument.body && canvas.dataset.previewLoaded === "0") {
          canvas.dataset.previewLoaded = "1";
          if (window.__stratum_canvasController) window.__stratum_canvasController.refresh();
        }
      }, 300);
    }
    // Also populate legacy iframe for old UI if visible
    const legacyFrame = previewElement ? previewElement.querySelector("iframe") : null;
    if (legacyFrame) {
      // update legacy
    } else if (previewElement && !document.getElementById("editor-canvas")) {
      // fallback when canvas not present (old shell)
      previewElement.replaceChildren();
      const frame = document.createElement("iframe");
      frame.className = "editor-preview-frame";
      frame.title = "Live preview";
      frame.setAttribute("sandbox", "allow-same-origin");
      frame.srcdoc = output;
      frame.style.width = state.previewWidth;
      previewElement.append(frame);
    }
    if (errorElement) { errorElement.textContent = ""; errorElement.hidden = true; }
  } catch (error) {
    if (previewElement) {
      previewElement.classList.add("editor-preview-error");
      previewElement.textContent = error.message;
    }
    if (errorElement) { errorElement.textContent = error.message; errorElement.hidden = false; }
    const canvas = document.getElementById("editor-canvas");
    if (canvas) {
      const safe = String(error.message).slice(0, 2000).replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;").replace(/"/g,"&quot;");
      canvas.srcdoc = `<html><body style="padding:20px;color:#991b1b;font-family:system-ui"><h3>Preview error</h3><pre>${safe}</pre></body></html>`;
    }
  }
}

export function initArchivePreviewContext() {
  const sel = document.getElementById("archive-preview-context");
  const wrap = document.getElementById("archive-preview-context-wrap");
  if (!sel) return;
  const ct = bootstrap.contentTypeId || bootstrap.resource?.contentTypeId || "";
  const catalogs = bootstrap.taxonomyCatalogs || {};
  const list = catalogs[ct] || [];
  let hasTerms = false;
  list.forEach(function(tax){
    (tax.terms || []).forEach(function(term){
      const opt = document.createElement("option");
      opt.value = tax.id + ":" + term.id;
      opt.textContent = (tax.label || tax.id) + ": " + term.label;
      sel.appendChild(opt);
      hasTerms = true;
    });
  });
  // Only show archive selector for archive templates that actually have taxonomy terms.
  // For pages/posts and other contexts, keep it hidden (shell defaults to display:none).
  if (wrap) {
    const isArchive = bootstrap.resource?.kind === "archive" || bootstrap.templateKind === "archive" || bootstrap.contextKind === "archive-template";
    if (hasTerms && isArchive) {
      wrap.style.display = "";
    } else {
      wrap.style.display = "none";
    }
  }
  sel.addEventListener("change", function(){ schedulePreview(); });
}
