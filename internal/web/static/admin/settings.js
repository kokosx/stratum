// Settings control panel behaviour:
//  - dirty tracking enables the Save Changes button and flips the status pill
//  - section navigation scrolls and highlights without a page reload (legacy stacked layout)
//  - saving and toggles are handled by Datastar (see the form/toggle attributes);
//    the server patches #settings-content in place, so no full reload occurs.
(function () {
  // New template renders one form per section (general/reading/seo/performance).
  // Old template used a single #settings-form. Support both.
  const form =
    document.getElementById("settings-form") ||
    document.getElementById("settings-form-general") ||
    document.getElementById("settings-form-reading") ||
    document.getElementById("settings-form-seo") ||
    document.getElementById("settings-form-performance") ||
    document.querySelector("form[id^='settings-form-']");

  if (form) {
    const saveButton = form.querySelector('button[type="submit"]') || document.getElementById("settings-save");
    const saveState = form.querySelector("#settings-save-state") || document.getElementById("settings-save-state");

    function markDirty() {
      if (saveState) {
        saveState.textContent = "Unsaved";
        saveState.classList.remove("is-saved");
        saveState.classList.add("is-dirty");
      }
      if (saveButton) saveButton.disabled = false;
    }

    form.addEventListener("input", markDirty);
    form.addEventListener("change", markDirty);
  }

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
