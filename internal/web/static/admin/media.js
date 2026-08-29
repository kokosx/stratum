// Media Library page behaviour: multi-upload, detail with metadata, usage, replace, regenerate, safe delete.
(function () {
  const grid = document.getElementById("media-grid");
  const detail = document.getElementById("media-detail");
  const uploadBtn = document.getElementById("media-upload");
  const uploadInput = document.getElementById("media-upload-input");
  if (!grid) return;

  function getCSRFToken() {
    const meta = document.querySelector('meta[name="csrf-token"]');
    if (meta && meta.content) return meta.content;
    const c = document.cookie.split("; ").find((x) => x.startsWith("stratum_csrf="));
    return c ? decodeURIComponent(c.split("=")[1]) : "";
  }
  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  }
  function formatBytes(n) {
    if (!n) return "0 B";
    const u = ["B", "KB", "MB", "GB"];
    const i = Math.floor(Math.log(n) / Math.log(1024));
    return (n / Math.pow(1024, i)).toFixed(i ? 1 : 0) + " " + u[i];
  }

  // Multi-upload via hidden input
  if (uploadBtn && uploadInput) {
    uploadBtn.addEventListener("click", () => uploadInput.click());
    uploadInput.addEventListener("change", async () => {
      const files = uploadInput.files;
      if (!files || files.length === 0) return;
      const fd = new FormData();
      for (let i = 0; i < files.length; i++) {
        fd.append("file", files[i]);
      }
      // Also support multiple via "file" repeated; server iterates over all
      uploadBtn.disabled = true;
      uploadBtn.textContent = "Uploading…";
      try {
        const res = await fetch("/admin/media/upload", { method: "POST", headers: { "X-CSRF-Token": getCSRFToken() }, body: fd });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || "Upload failed");
        const assets = data.assets || (data.asset ? [data.asset] : []);
        let okCount = assets.length;
        let failCount = (data.failed || 0);
        assets.forEach((asset) => addCard(asset));
        if (okCount > 0 && failCount > 0) {
          window.stratumToast("success", okCount + " images uploaded. " + failCount + " failed.");
          if (data.errors) {
            const errMsg = data.errors.map(e => e.filename + ": " + e.error).join("; ");
            window.stratumToast("error", errMsg);
          }
        } else if (okCount > 0) {
          window.stratumToast("success", okCount + " image(s) uploaded.");
        } else if (failCount > 0) {
          window.stratumToast("error", "Upload failed: " + (data.errors ? data.errors[0].error : "unknown"));
        }
      } catch (e) {
        window.stratumToast("error", e.message);
      } finally {
        uploadBtn.disabled = false;
        uploadBtn.textContent = "Upload";
        uploadInput.value = "";
      }
    });
  } else if (uploadBtn) {
    // Fallback to picker
    uploadBtn.addEventListener("click", () => {
      window.openMediaPicker({ mode: "upload", onUploaded: (asset) => addCard(asset) });
    });
  }

  grid.addEventListener("click", (e) => {
    const card = e.target.closest("[data-media-card]");
    if (card) selectCard(card);
  });

  function selectCard(card) {
    grid.querySelectorAll(".media-card.is-selected").forEach((c) => {
      c.classList.remove("is-selected");
      c.setAttribute("aria-pressed", "false");
    });
    card.classList.add("is-selected");
    card.setAttribute("aria-pressed", "true");
    showDetail(card.dataset.mediaId);
  }

  function addCard(asset) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "media-card";
    btn.setAttribute("aria-pressed", "false");
    btn.dataset.mediaCard = asset.id;
    btn.dataset.mediaId = asset.id;
    btn.innerHTML =
      `<span class="media-thumb"><img src="${escapeHtml(asset.url)}" alt="" loading="lazy" onerror="this.onerror=null;this.src='${escapeHtml(asset.original)}'"></span>` +
      `<span class="media-card__name" title="${escapeHtml(asset.filename)}">${escapeHtml(asset.filename)}</span>` +
      `<span class="media-card__meta">${asset.width} × ${asset.height}</span>`;
    // Insert at beginning
    if (grid.firstChild) grid.insertBefore(btn, grid.firstChild);
    else grid.appendChild(btn);
    selectCard(btn);
  }

  async function showDetail(id) {
    detail.innerHTML = '<p class="editor-empty">Loading…</p>';
    try {
      const data = await fetch("/admin/media/" + encodeURIComponent(id) + "/json").then((r) => r.json());
      renderDetail(data);
    } catch (e) {
      detail.innerHTML = '<p class="form-error">Failed to load.</p>';
    }
  }

  function renderDetail(data) {
    const a = data.asset;
    const variants = (data.variants || [])
      .map((v) => `<li><code>${escapeHtml(v.kind)}</code> — ${v.width}×${v.height} (${formatBytes(v.size)}) <a href="${escapeHtml(v.url)}" target="_blank" rel="noopener">view</a></li>`)
      .join("");
    const usageRefs = data.usageRefs || [];
    let usageHTML = "";
    if (usageRefs.length > 0) {
      usageHTML = `<div class="media-usage"><h3>Usage — ${usageRefs.length} place(s)</h3><ul class="media-usage-list">` +
        usageRefs.map(r => `<li><a href="${escapeHtml(r.editUrl)}">${escapeHtml(r.sourceLabel)}</a> — ${escapeHtml(r.context)} ${r.public ? "(published)" : "(draft)"} <a href="${escapeHtml(r.editUrl)}">Edit</a></li>`).join("") +
        `</ul></div>`;
    } else {
      usageHTML = `<div class="media-usage"><h3>Usage</h3><p class="muted">Not used. Safe to delete.</p></div>`;
    }
    const altHelper = a.alt ? `<small>Alt: ${escapeHtml(a.alt)}</small>` : `<small class="muted">Alt text not set</small>`;
    const canDelete = usageRefs.length === 0;
    const deleteSection = canDelete
      ? `<button type="button" class="button button-danger" id="media-delete">Delete image</button>`
      : `<div id="media-delete-warning" role="alert"><p class="form-warning">This image cannot be deleted because it is used in ${usageRefs.length} place(s).</p>` + usageHTML + `</div>`;

    detail.innerHTML = `
      <div class="media-detail__preview" id="media-detail-preview"><img src="${escapeHtml(a.original)}" alt=""></div>
      <dl class="media-detail__meta">
        <dt>Filename</dt><dd>${escapeHtml(a.filename)}</dd>
        <dt>Dimensions</dt><dd>${a.width ? a.width + "×" + a.height : "—"}</dd>
        <dt>File size</dt><dd>${formatBytes(a.size)}</dd>
        <dt>Uploaded</dt><dd>${escapeHtml(a.mime)}</dd>
        <dt>Type</dt><dd>${escapeHtml(a.mime)}</dd>
      </dl>
      <form id="media-meta-form" class="media-detail__form" data-id="${escapeHtml(a.id)}">
        <div class="form-field"><label>Alt text<input type="text" name="alt_text" value="${escapeHtml(a.alt)}"></label><small>Describe the image when it conveys meaningful content. Leave empty when the specific usage is decorative.</small><div id="media-detail-alt">${escapeHtml(a.alt)}</div>${altHelper}</div>
        <div class="form-field"><label>Title<input type="text" name="title" value="${escapeHtml(a.title)}"></label></div>
        <div class="form-field"><label>Caption<input type="text" name="caption" value="${escapeHtml(a.caption)}"></label></div>
        <div class="form-field"><label>Description<textarea name="description" rows="3">${escapeHtml(a.description)}</textarea></label></div>
        <button type="button" class="button button-primary" id="media-save">Save</button>
      </form>
      <h3>Variants</h3>
      <ul class="media-variants">${variants || "<li>None</li>"}</ul>
      ${usageHTML}
      <section class="media-replace">
        <h3>Replace image</h3>
        <p>Choose a new image file. Existing pages and blocks will keep using this Media item. The Media ID and references will be preserved.</p>
        <form id="media-replace-form" enctype="multipart/form-data">
          <input type="file" accept="image/*" id="media-replace-input" name="file">
          <button type="submit" class="button" id="media-replace-btn">Replace image</button>
        </form>
      </section>
      <section class="media-regenerate">
        <h3>Regenerate variants</h3>
        <p>Rebuild responsive and optimized versions from the original image.</p>
        <button type="button" class="button" id="media-regenerate">Regenerate variants</button>
      </section>
      <section class="media-delete">
        ${deleteSection}
      </section>`;

    const saveBtn = detail.querySelector("#media-save");
    if (saveBtn) saveBtn.addEventListener("click", saveMeta);
    const delBtn = detail.querySelector("#media-delete");
    if (delBtn) delBtn.addEventListener("click", deleteAsset);
    const replaceForm = detail.querySelector("#media-replace-form");
    if (replaceForm) replaceForm.addEventListener("submit", replaceAsset);
    const regenBtn = detail.querySelector("#media-regenerate");
    if (regenBtn) regenBtn.addEventListener("click", regenerateAsset);
  }

  async function saveMeta() {
    const form = detail.querySelector("#media-meta-form");
    const id = form.dataset.id;
    const fd = new FormData(form);
    const btn = detail.querySelector("#media-save");
    if (btn) { btn.disabled = true; btn.textContent = "Saving…"; }
    try {
      const res = await fetch("/admin/media/" + encodeURIComponent(id), { method: "POST", headers: { "X-CSRF-Token": getCSRFToken() }, body: fd });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || "Save failed");
      window.stratumToast("success", "Media updated");
      // Refresh detail to show new alt etc
      showDetail(id);
      // Update card alt hint if present
      const card = grid.querySelector('[data-media-card="' + id + '"]');
      if (card) {
        // Could update alt indicator but not critical
      }
    } catch (e) {
      window.stratumToast("error", e.message);
    } finally {
      if (btn) { btn.disabled = false; btn.textContent = "Save"; }
    }
  }

  async function deleteAsset() {
    const form = detail.querySelector("#media-meta-form");
    if (!form) return;
    const id = form.dataset.id;
    if (!confirm("Delete this image?")) return;
    const fd = new FormData();
    try {
      const res = await fetch("/admin/media/" + encodeURIComponent(id) + "/delete", { method: "POST", headers: { "X-CSRF-Token": getCSRFToken() }, body: fd });
      const data = await res.json().catch(() => ({}));
      if (res.status === 409) {
        const msg = data.error || "Media is still in use";
        window.stratumToast("error", msg);
        // Re-render detail with usage info
        showDetail(id);
        return;
      }
      if (!res.ok) throw new Error(data.error || "Delete failed");
      const card = grid.querySelector('[data-media-card="' + id + '"]');
      if (card) card.remove();
      detail.innerHTML = '<p class="editor-empty">Select an asset to view details.</p>';
      window.stratumToast("success", "Media deleted");
    } catch (e) {
      window.stratumToast("error", e.message);
    }
  }

  async function replaceAsset(e) {
    e.preventDefault();
    const form = detail.querySelector("#media-meta-form");
    const id = form.dataset.id;
    const input = detail.querySelector("#media-replace-input");
    if (!input.files || input.files.length === 0) {
      window.stratumToast("error", "Choose a file to replace");
      return;
    }
    const fd = new FormData();
    fd.append("file", input.files[0]);
    const btn = detail.querySelector("#media-replace-btn");
    if (btn) { btn.disabled = true; btn.textContent = "Replacing…"; }
    try {
      const res = await fetch("/admin/media/" + encodeURIComponent(id) + "/replace", { method: "POST", headers: { "X-CSRF-Token": getCSRFToken() }, body: fd });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || "Could not replace image. The existing image was not changed.");
      window.stratumToast("success", "Image replaced.");
      showDetail(id);
      // Update grid thumbnail src to force reload (bust cache via new URL)
      const card = grid.querySelector('[data-media-card="' + id + '"] img');
      if (card && data.asset) {
        card.src = data.asset.url + "?t=" + Date.now();
      }
    } catch (e) {
      window.stratumToast("error", e.message);
    } finally {
      if (btn) { btn.disabled = false; btn.textContent = "Replace image"; }
      input.value = "";
    }
  }

  async function regenerateAsset() {
    const form = detail.querySelector("#media-meta-form");
    const id = form.dataset.id;
    const btn = detail.querySelector("#media-regenerate");
    if (btn) { btn.disabled = true; btn.textContent = "Regenerating…"; }
    try {
      const res = await fetch("/admin/media/" + encodeURIComponent(id) + "/regenerate", { method: "POST", headers: { "X-CSRF-Token": getCSRFToken() } });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || "Could not regenerate");
      window.stratumToast("success", "Variants regenerated.");
      showDetail(id);
    } catch (e) {
      window.stratumToast("error", e.message);
    } finally {
      if (btn) { btn.disabled = false; btn.textContent = "Regenerate variants"; }
    }
  }
})();
