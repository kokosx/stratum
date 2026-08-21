// Reusable Media Picker. Opened from the block editor, the Site Icon field, and
// the Media Library upload button. One component, reused everywhere an image is
// chosen, so uploads and selection stay consistent across the CMS.
(function () {
  let modal = null;
  let active = null;

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
    loadLibrary();
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
            <input type="search" class="media-picker__search" placeholder="Search…">
            <div class="media-picker__grid" id="picker-grid"></div>
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
    modal.querySelector(".media-picker__search").addEventListener("input", (e) => loadLibrary(e.target.value));
  }

  function switchTab(tab) {
    modal.querySelectorAll("[data-tab]").forEach((b) => b.classList.toggle("is-active", b.dataset.tab === tab));
    modal.querySelectorAll("[data-panel]").forEach((p) => (p.hidden = p.dataset.panel !== tab));
  }

  async function loadLibrary(query) {
    const grid = modal.querySelector("#picker-grid");
    grid.innerHTML = '<p class="editor-empty">Loading…</p>';
    try {
      const data = await api("/admin/media.json");
      const items = (data.items || []).filter((it) => !query || it.filename.toLowerCase().includes(query.toLowerCase()));
      if (!items.length) { grid.innerHTML = '<p class="editor-empty">No media found.</p>'; return; }
      grid.innerHTML = "";
      items.forEach((it) => {
        const btn = document.createElement("button");
        btn.type = "button";
        btn.className = "media-card";
        btn.innerHTML =
          `<span class="media-thumb"><img src="${it.url}" alt="" loading="lazy" onerror="this.onerror=null;this.src='${it.original}'"></span>` +
          `<span class="media-card__name">${escapeHtml(it.filename)}</span>`;
        btn.addEventListener("click", () => selectAsset(it));
        grid.appendChild(btn);
      });
    } catch (err) {
      grid.innerHTML = `<p class="form-error">${escapeHtml(err.message)}</p>`;
    }
  }

  function selectAsset(asset) {
    if (active && active.onSelect) active.onSelect(asset);
    close();
  }

  async function uploadFile(file) {
    const fd = new FormData();
    fd.append("file", file);
    try {
      const data = await api("/admin/media/upload", { method: "POST", body: fd });
      if (!data.ok) throw new Error("Upload failed");
      if (active && active.onUploaded) active.onUploaded(data.asset);
      if (active && active.mode === "upload") { close(); return; }
      selectAsset(data.asset);
    } catch (err) {
      window.stratumToast("error", err.message);
    }
  }

  window.openMediaPicker = open;
  window.closeMediaPicker = close;
})();
