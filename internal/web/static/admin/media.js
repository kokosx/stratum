// Media Library page behaviour: upload (via the shared picker), open the detail
// panel, edit metadata and delete with a usage guard.
(function () {
  const grid = document.getElementById("media-grid");
  const detail = document.getElementById("media-detail");
  const uploadBtn = document.getElementById("media-upload");
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

  if (uploadBtn) {
    uploadBtn.addEventListener("click", () => {
      window.openMediaPicker({ mode: "upload", onUploaded: (asset) => addCard(asset) });
    });
  }

  grid.addEventListener("click", (e) => {
    const card = e.target.closest("[data-media-card]");
    if (card) selectCard(card);
  });

  // P2.61: visible, keyboard-reachable selected state.
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
      `<span class="media-thumb"><img src="${asset.url}" alt="" loading="lazy" onerror="this.onerror=null;this.src='${asset.original}'"></span>` +
      `<span class="media-card__name">${escapeHtml(asset.filename)}</span>`;
    grid.prepend(btn);
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
      .map((v) => `<li><code>${escapeHtml(v.kind)}</code> — ${v.width}×${v.height} (${formatBytes(v.size)}) <a href="${v.url}" target="_blank" rel="noopener">view</a></li>`)
      .join("");
    const usageWarn = data.usage > 0
      ? `<p class="form-warning" id="media-delete-warning" role="alert">Used by ${data.usage} piece(s) of content.</p>`
      : "";
    detail.innerHTML = `
      <div class="media-detail__preview"><img src="${a.original}" alt=""></div>
      <dl class="media-detail__meta">
        <dt>Filename</dt><dd>${escapeHtml(a.filename)}</dd>
        <dt>URL</dt><dd><code>${a.original}</code></dd>
        <dt>Type</dt><dd>${escapeHtml(a.mime)}</dd>
        <dt>File size</dt><dd>${formatBytes(a.size)}</dd>
        <dt>Dimensions</dt><dd>${a.width ? a.width + "×" + a.height : "—"}</dd>
      </dl>
      <form id="media-meta-form" class="media-detail__form" data-id="${a.id}">
        <label>Alt text<input type="text" name="alt_text" value="${escapeHtml(a.alt)}"></label>
        <label>Title<input type="text" name="title" value="${escapeHtml(a.title)}"></label>
        <label>Caption<input type="text" name="caption" value="${escapeHtml(a.caption)}"></label>
        <label>Description<textarea name="description" rows="3">${escapeHtml(a.description)}</textarea></label>
        <button type="button" class="button button-primary" id="media-save">Save</button>
      </form>
      <h3>Variants</h3>
      <ul class="media-variants">${variants || "<li>None</li>"}</ul>
      ${usageWarn}
      <button type="button" class="button button-danger" id="media-delete">Delete</button>`;
    detail.querySelector("#media-save").addEventListener("click", saveMeta);
    detail.querySelector("#media-delete").addEventListener("click", deleteAsset);
  }

  async function saveMeta() {
    const form = detail.querySelector("#media-meta-form");
    const id = form.dataset.id;
    const fd = new FormData(form);
    try {
      const res = await fetch("/admin/media/" + id, { method: "POST", headers: { "X-CSRF-Token": getCSRFToken() }, body: fd });
      if (!res.ok) throw new Error("Save failed");
      window.stratumToast("success", "Media updated");
    } catch (e) {
      window.stratumToast("error", e.message);
    }
  }

  async function deleteAsset() {
    const id = detail.querySelector("#media-meta-form").dataset.id;
    if (!confirm("Delete this media?")) return;
    const fd = new FormData();
    try {
      const res = await fetch("/admin/media/" + id + "/delete", { method: "POST", headers: { "X-CSRF-Token": getCSRFToken() }, body: fd });
      if (res.status === 409) { window.stratumToast("error", "Media is still in use"); return; }
      if (!res.ok) throw new Error("Delete failed");
      const card = grid.querySelector('[data-media-card="' + id + '"]');
      if (card) card.remove();
      detail.innerHTML = '<p class="editor-empty">Select an asset to view details.</p>';
      window.stratumToast("success", "Media deleted");
    } catch (e) {
      window.stratumToast("error", e.message);
    }
  }
})();
