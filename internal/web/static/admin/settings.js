// Settings control panel behaviour:
//  - dirty tracking enables the Save Changes button and flips the status pill
//  - section navigation scrolls and highlights without a page reload
//  - saving and toggles are handled by Datastar (see the form/toggle attributes);
//    the server patches #settings-content in place, so no full reload occurs.
(function () {
  const form = document.getElementById("settings-form");
  if (!form) return;

  const saveButton = document.getElementById("settings-save");
  const saveState = document.getElementById("settings-save-state");

  function markDirty() {
    if (saveState) {
      saveState.textContent = "Unsaved";
      saveState.classList.remove("is-saved");
      saveState.classList.add("is-dirty");
    }
    if (saveButton) saveButton.disabled = false;
  }

  // Delegated listeners survive Datastar morphs that replace #settings-content,
  // because the form element itself is never replaced.
  form.addEventListener("input", markDirty);
  form.addEventListener("change", markDirty);

  // Section navigation (smooth scroll + active highlight). Sections are stacked,
  // so this is purely a navigation aid and never affects saved state.
  const navLinks = Array.from(document.querySelectorAll(".settings-nav__link"));
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
