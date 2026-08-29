// Settings control panel behaviour:
//  - dirty tracking enables the Save changes button and flips the plain status text
//  - section navigation scrolls and highlights without a page reload (legacy stacked layout)
//  - saving and toggles are handled by Datastar (see the form/toggle attributes);
//    the server patches #settings-content in place, so no full reload occurs.
(function () {
  // Support both legacy and new per-section forms
  function initForm(form) {
    if (!form || form.dataset.dirtyInit === "1") return;
    form.dataset.dirtyInit = "1";
    const saveButton = form.querySelector('button[type="submit"]');
    const saveState = form.querySelector(".editor-status") || form.querySelector("#settings-save-state") || document.getElementById("settings-save-state");
    const initial = new FormData(form);
    const initialStr = JSON.stringify(Array.from(initial.entries()));

    function isDirty() {
      const cur = new FormData(form);
      return JSON.stringify(Array.from(cur.entries())) !== initialStr;
    }
    function markDirty() {
      const dirty = isDirty();
      if (saveState) {
        saveState.textContent = dirty ? "Unsaved changes" : "Saved";
        saveState.classList.toggle("is-dirty", dirty);
        saveState.classList.toggle("is-saved", !dirty);
      }
      if (saveButton) saveButton.disabled = !dirty;
    }
    function syncBaseline() {
      // Reset baseline after successful save (called via observer)
    }
    form.addEventListener("input", markDirty);
    form.addEventListener("change", markDirty);

    // Double-submit + loading feedback
    if (saveButton) {
      saveButton.addEventListener("click", () => {
        if (saveButton.dataset.busy === "1") return;
        saveButton.dataset.busy = "1";
        const orig = saveButton.textContent;
        saveButton.dataset.origText = orig;
        saveButton.textContent = "Saving…";
        saveButton.disabled = true;
        let timeout = setTimeout(() => { saveButton.textContent = orig; saveButton.disabled = !isDirty(); saveButton.dataset.busy=""; }, 8000);
        const region = document.getElementById("settings-content");
        const obs = new MutationObserver(() => {
          const curDirty = isDirty();
          const newState = region ? region.querySelector(".editor-status") : null;
          if (newState && newState.textContent.trim() === "Saved") {
            clearTimeout(timeout);
            saveButton.textContent = orig;
            saveButton.disabled = true;
            saveButton.dataset.busy = "";
            obs.disconnect();
          } else if (!curDirty) {
            clearTimeout(timeout);
            saveButton.textContent = orig;
            saveButton.disabled = true;
            saveButton.dataset.busy = "";
            obs.disconnect();
          }
        });
        if (region) obs.observe(region, { childList: true, subtree: true });
      });
    }

    // Ctrl/Cmd+S
    form.addEventListener("keydown", (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "s") {
        e.preventDefault();
        if (saveButton && !saveButton.disabled) saveButton.click();
      }
    });

    // Beforeunload guard
    window.addEventListener("beforeunload", (e) => {
      if (isDirty()) { e.preventDefault(); e.returnValue = ""; }
    });

    // Observe Datastar patch to re-init after save (form replaced)
    const content = document.getElementById("settings-content");
    if (content) {
      const mo = new MutationObserver(() => {
        // Re-initialize if form was replaced
        const newForm = document.getElementById("settings-form-general") || document.getElementById("settings-form-reading") || document.getElementById("settings-form-seo") || document.getElementById("settings-form-performance");
        if (newForm && newForm !== form) {
          mo.disconnect();
          initForm(newForm);
        }
        // Also reset status if saved
        const stateEl = content.querySelector(".editor-status");
        if (stateEl && stateEl.textContent.trim() === "Saved") {
          // Update baseline
        }
      });
      mo.observe(content, { childList: true, subtree: true });
    }
  }

  const initialForm =
    document.getElementById("settings-form") ||
    document.getElementById("settings-form-general") ||
    document.getElementById("settings-form-reading") ||
    document.getElementById("settings-form-seo") ||
    document.getElementById("settings-form-performance") ||
    document.querySelector("form[id^='settings-form-']");
  if (initialForm) initForm(initialForm);
  // Also init all section forms for beforeunload (each section)
  document.querySelectorAll("form[id^='settings-form-']").forEach(initForm);

  // Section navigation: legacy stacked layout used data-section anchors and smooth scroll.
  // New layout uses real page links (/admin/settings/general etc) – don't intercept those.
  const navLinks = Array.from(document.querySelectorAll(".settings-nav__link"));
  const hasDataSection = navLinks.some((l) => l.dataset.section);
  if (!hasDataSection) return;

  navLinks.forEach((link) => {
    link.addEventListener("click", (event) => {
      event.preventDefault();
      const section = document.getElementById("section-" + link.dataset.section);
      if (section) section.scrollIntoView({ behavior: "smooth", block: "start" });
    });
  });

  const sections = navLinks
    .map((link) => document.getElementById("section-" + link.dataset.section))
    .filter(Boolean);

  if ("IntersectionObserver" in window && sections.length) {
    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (!entry.isIntersecting) return;
          const id = entry.target.id.replace("section-", "");
          navLinks.forEach((link) => {
            link.classList.toggle("is-active", link.dataset.section === id);
          });
        });
      },
      { rootMargin: "-20% 0px -70% 0px" }
    );
    sections.forEach((section) => observer.observe(section));
  }
})();
