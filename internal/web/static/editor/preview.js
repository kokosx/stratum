// preview.js — preview POST, debounce, iframe srcdoc refresh
import { state, bootstrap } from "./state.js";
import { isDirtyNow } from "./state.js";

let previewTimer = null;
let lastGoodSrcdoc = null;
let pendingController = null;

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
  url.searchParams.set("editor_canvas", "1");

  // Abort previous pending preview
  if (pendingController) {
    try { pendingController.abort(); } catch (_) {}
  }
  pendingController = new AbortController();

  try {
    const response = await fetch(url.toString(), {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Requested-With": "StratumEditor" },
      body: previewParams,
      signal: pendingController.signal,
    });
    const output = await response.text();
    if (!response.ok) throw new Error(output.trim() || "Preview failed");
    // Success — remember last good
    lastGoodSrcdoc = output;
    if (previewElement) {
      previewElement.classList.remove("editor-preview-error");
    }
    const canvas = document.getElementById("editor-canvas");
    if (canvas) {
      canvas.dataset.previewLoaded = "0";
      canvas.onload = () => {
        canvas.dataset.previewLoaded = "1";
        try {
          if (canvas.contentDocument && canvas.contentDocument.body) {}
        } catch (_) {}
        if (window.__stratum_canvasController) {
          setTimeout(() => window.__stratum_canvasController.refresh(), 50);
        }
        if (state.selectedNodeId && window.__stratum_canvasController) {
          const inst = state.selectedInstanceKey;
          window.__stratum_canvasController.selectNode(state.selectedNodeId, inst, { scroll: false });
        }
      };
      canvas.srcdoc = output;
      setTimeout(() => {
        if (canvas.contentDocument && canvas.contentDocument.body && canvas.dataset.previewLoaded === "0") {
          canvas.dataset.previewLoaded = "1";
          if (window.__stratum_canvasController) window.__stratum_canvasController.refresh();
        }
      }, 300);
    }
    const legacyFrame = previewElement ? previewElement.querySelector("iframe") : null;
    if (legacyFrame) {
    } else if (previewElement && !document.getElementById("editor-canvas")) {
      previewElement.replaceChildren();
      const frame = document.createElement("iframe");
      frame.className = "editor-preview-frame";
      frame.title = "Live preview";
      frame.setAttribute("sandbox", "allow-same-origin");
      frame.srcdoc = output;
      frame.style.width = state.previewWidth;
      previewElement.append(frame);
    }
    if (errorElement) {
      errorElement.textContent = "";
      errorElement.hidden = true;
      errorElement.style.display = "none";
      // also hide banner if present
      const banner = document.getElementById("preview-error-banner");
      if (banner) banner.hidden = true;
    }
  } catch (error) {
    if (error.name === "AbortError") return;
    const msg = String(error.message || "Preview failed").slice(0, 2000);
    const friendly = `Could not render current draft: ${msg}`;
    if (previewElement) {
      previewElement.classList.add("editor-preview-error");
      // keep previewElement minimal; main error is banner
    }
    // Show banner above canvas instead of replacing iframe
    let banner = document.getElementById("preview-error-banner");
    const canvasWrap = document.getElementById("editor-canvas-wrap");
    if (!banner && canvasWrap) {
      banner = document.createElement("div");
      banner.id = "preview-error-banner";
      banner.className = "preview-error-banner";
      banner.style.cssText = "background:#fef2f2;border:1px solid #fecaca;color:#991b1b;padding:8px 12px;margin-bottom:8px;border-radius:6px;font-size:12px;display:flex;gap:8px;align-items:center;justify-content:space-between";
      const textSpan = document.createElement("span");
      textSpan.id = "preview-error-text";
      textSpan.style.flex = "1";
      const undoBtn = document.createElement("button");
      undoBtn.type = "button";
      undoBtn.textContent = "Undo";
      undoBtn.className = "button button-small";
      undoBtn.style.cssText = "padding:4px 8px;font-size:11px;border:1px solid #fecaca;background:white;color:#991b1b;border-radius:4px;cursor:pointer";
      undoBtn.addEventListener("click", () => {
        if (window.__stratum_undo) window.__stratum_undo();
      });
      banner.append(textSpan, undoBtn);
      canvasWrap.parentNode.insertBefore(banner, canvasWrap);
    }
    if (banner) {
      const txt = banner.querySelector("#preview-error-text");
      if (txt) txt.textContent = friendly;
      banner.hidden = false;
      banner.style.display = "flex";
    }
    if (errorElement) {
      errorElement.textContent = friendly;
      errorElement.hidden = false;
      errorElement.style.display = "block";
    }
    // Do NOT replace canvas srcdoc if we have a last good version
    const canvas = document.getElementById("editor-canvas");
    if (canvas && !lastGoodSrcdoc) {
      const safe = msg.replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;").replace(/"/g,"&quot;");
      canvas.srcdoc = `<html><body style="padding:20px;color:#991b1b;font-family:system-ui"><h3>Preview error</h3><pre>${safe}</pre></body></html>`;
    }
    // If we have lastGood, keep it — canvas already shows last good
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
