// Reusable Media Picker. Opened from the block editor, the Site Icon field, and
// the Media Library upload button. One component, reused everywhere an image is
// chosen, so uploads and selection stay consistent across the CMS.
(function () {
  let modal = null;
  let active = null;
  let currentSearch = "";
  let currentPage = 1;
  let hasMore = false;
  let loading = false;
  let debounceTimer = null;

  function getCSRFToken() {
    const meta = document.querySelector('meta[name="csrf-token"]');
    if (meta && meta.content) return meta.content;
    const c = document.cookie.split("; ").find((x) => x.startsWith("stratum_csrf="));
    return c ? decodeURIComponent(c.split("=")[1]) : "";
  }

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  }

  async function api(path, opts) {
    const res = await fetch(path, Object.assign({ headers: { "X-CSRF-Token": getCSRFToken() } }, opts));
    if (!res.ok) {
      let msg = "Request failed";
      try { const j = await res.json(); if (j.error) msg = j.error; } catch (_) {}
      throw new Error(msg);
    }
    return res.json();
  }

  function ensureRoot() {
    let root = document.getElementById("media-picker-root");
    if (!root) {
      root = document.createElement("div");
      root.id = "media-picker-root";
      document.body.appendChild(root);
    }
    return root;
  }

  function open(opts) {
    active = opts || {};
    if (!modal) buildModal(ensureRoot());
    modal.hidden = false;
    currentSearch = "";
    currentPage = 1;
    const searchInput = modal.querySelector(".media-picker__search");
    if (searchInput) searchInput.value = "";
    loadLibrary("", 1, false);
    document.addEventListener("keydown", onKey);
  }

  function close() {
    if (modal) modal.hidden = true;
    document.removeEventListener("keydown", onKey);
    active = null;
  }

  function onKey(e) { if (e.key === "Escape") close(); }

  function buildModal(root) {
    modal = document.createElement("div");
    modal.className = "media-picker";
    modal.hidden = true;
    modal.innerHTML = `
      <div class="media-picker__backdrop" data-close></div>
      <div class="media-picker__dialog" role="dialog" aria-modal="true" aria-label="Media picker">
        <header class="media-picker__tabs">
          <button type="button" class="is-active" data-tab="library">Media Library</button>
          <button type="button" data-tab="upload">Upload</button>
          <button type="button" class="media-picker__close" data-close aria-label="Close">×</button>
        </header>
        <div class="media-picker__body">
          <div class="media-picker__panel" data-panel="library">
            <input type="search" class="media-picker__search" placeholder="Search filename, title, alt, caption…">
            <div class="media-picker__grid" id="picker-grid"></div>
            <div class="media-picker__pagination"><button type="button" class="button" id="picker-load-more" hidden>Load more</button></div>
          </div>
          <div class="media-picker__panel" data-panel="upload" hidden>
            <div class="media-dropzone" id="picker-dropzone">
              <p>Drag &amp; drop an image here, or</p>
              <input type="file" accept="image/*" id="picker-file" hidden>
              <button type="button" class="button" id="picker-choose">Choose file</button>
            </div>
          </div>
        </div>
      </div>`;
    root.appendChild(modal);
    modal.addEventListener("click", (e) => { if (e.target.dataset.close !== undefined) close(); });
    modal.querySelector('[data-tab="library"]').addEventListener("click", () => switchTab("library"));
    modal.querySelector('[data-tab="upload"]').addEventListener("click", () => switchTab("upload"));
    modal.querySelector("#picker-choose").addEventListener("click", () => modal.querySelector("#picker-file").click());
    modal.querySelector("#picker-file").addEventListener("change", (e) => { if (e.target.files[0]) uploadFile(e.target.files[0]); });
    const dz = modal.querySelector("#picker-dropzone");
    dz.addEventListener("dragover", (e) => { e.preventDefault(); dz.classList.add("is-drag"); });
    dz.addEventListener("dragleave", () => dz.classList.remove("is-drag"));
    dz.addEventListener("drop", (e) => {
      e.preventDefault(); dz.classList.remove("is-drag");
      if (e.dataTransfer.files[0]) uploadFile(e.dataTransfer.files[0]);
    });
    const searchEl = modal.querySelector(".media-picker__search");
    searchEl.addEventListener("input", (e) => {
      const val = e.target.value;
      clearTimeout(debounceTimer);
      debounceTimer = setTimeout(() => {
        currentSearch = val;
        currentPage = 1;
        loadLibrary(val, 1, false);
      }, 300);
    });
    const loadMore = modal.querySelector("#picker-load-more");
    loadMore.addEventListener("click", () => {
      if (hasMore && !loading) {
        loadLibrary(currentSearch, currentPage + 1, true);
      }
    });
  }

  function switchTab(tab) {
    modal.querySelectorAll("[data-tab]").forEach((b) => b.classList.toggle("is-active", b.dataset.tab === tab));
    modal.querySelectorAll("[data-panel]").forEach((p) => (p.hidden = p.dataset.panel !== tab));
  }

  async function loadLibrary(query, page, append) {
    const grid = modal.querySelector("#picker-grid");
    const loadMore = modal.querySelector("#picker-load-more");
    if (loading) return;
    loading = true;
    if (!append) {
      grid.innerHTML = '<p class="editor-empty">Loading…</p>';
      loadMore.hidden = true;
    } else {
      loadMore.textContent = "Loading…";
      loadMore.disabled = true;
    }
    try {
      const params = new URLSearchParams();
      if (query) params.set("search", query);
      params.set("page", String(page));
      params.set("limit", "40");
      const data = await api("/admin/media.json?" + params.toString());
      const items = data.items || [];
      const total = data.total || items.length;
      const perPage = data.perPage || 40;
      hasMore = (page * perPage) < total;
      currentPage = page;
      if (!append) grid.innerHTML = "";
      if (!items.length && !append) { grid.innerHTML = '<p class="editor-empty">No media found.</p>'; loadMore.hidden = true; return; }
      items.forEach((it) => {
        const btn = document.createElement("button");
        btn.type = "button";
        btn.className = "media-card";
        const altHint = it.alt ? `<small class="media-card__alt-hint">${escapeHtml(it.alt.slice(0,60))}</small>` : `<small class="muted">Alt text not set</small>`;
        btn.innerHTML =
          `<span class="media-thumb"><img src="${escapeHtml(it.url)}" alt="" loading="lazy" onerror="this.onerror=null;this.src='${escapeHtml(it.original)}'"></span>` +
          `<span class="media-card__name">${escapeHtml(it.filename)}</span>` +
          altHint;
        btn.addEventListener("click", () => selectAsset(it));
        grid.appendChild(btn);
      });
      loadMore.hidden = !hasMore;
      loadMore.textContent = "Load more";
      loadMore.disabled = false;
    } catch (err) {
      if (!append) grid.innerHTML = `<p class="form-error">${escapeHtml(err.message)}</p>`;
      else {
        const errP = document.createElement("p");
        errP.className = "form-error";
        errP.textContent = err.message;
        grid.appendChild(errP);
      }
    } finally {
      loading = false;
    }
  }

  function selectAsset(asset) {
    if (active && active.onSelect) active.onSelect(asset);
    // For gallery incremental add, keep picker open? Specification says preserve order; we close per selection for simplicity.
    // If active expects multi-select without closing, it could set active.keepOpen.
    if (!active || !active.keepOpen) close();
  }

  async function uploadFile(file) {
    const fd = new FormData();
    fd.append("file", file);
    try {
      const data = await api("/admin/media/upload", { method: "POST", body: fd });
      if (!data.ok) throw new Error("Upload failed");
      const asset = data.asset || (data.assets && data.assets[0]);
      if (!asset) throw new Error("Upload failed");
      if (active && active.onUploaded) active.onUploaded(asset);
      if (active && active.mode === "upload") { close(); return; }
      selectAsset(asset);
    } catch (err) {
      window.stratumToast("error", err.message);
    }
  }

  window.openMediaPicker = open;
  window.closeMediaPicker = close;
})();
