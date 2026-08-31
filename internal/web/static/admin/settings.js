// Settings control panel behaviour:
//  - dirty tracking enables the Save changes button and flips the plain status text
//  - saving and toggles are handled by Datastar (server patches #settings-content);
//    no stacked-layout IntersectionObserver needed — sections are separate routes.
(function () {
  function initForm(form) {
    if (!form || form.dataset.dirtyInit === "1") return;
    form.dataset.dirtyInit = "1";
    const saveButton = form.querySelector('button[type="submit"]');
    const saveState = form.querySelector(".editor-status") || form.querySelector("#settings-save-state") || document.getElementById("settings-save-state");
    let initialStr = JSON.stringify(Array.from(new FormData(form).entries()));
    let isSubmitting = false;

    function isDirty() {
      const cur = new FormData(form);
      return JSON.stringify(Array.from(cur.entries())) !== initialStr;
    }
    function markDirty() {
      if (isSubmitting) return;
      const dirty = isDirty();
      if (saveState) {
        saveState.textContent = dirty ? "Unsaved changes" : "Saved";
        saveState.classList.toggle("is-dirty", dirty);
        saveState.classList.toggle("is-saved", !dirty);
      }
      if (saveButton) saveButton.disabled = !dirty;
    }
    function syncBaseline() {
      initialStr = JSON.stringify(Array.from(new FormData(form).entries()));
      markDirty();
    }
    form.addEventListener("input", markDirty);
    form.addEventListener("change", markDirty);

    // Busy state owns by submit event, not click — ensures POST is dispatched first.
    form.addEventListener("submit", () => {
      if (isSubmitting) return;
      isSubmitting = true;
      if (saveButton) {
        saveButton.dataset.busy = "1";
        saveButton.dataset.origText = saveButton.textContent;
        // Use aria-busy rather than immediately disabling to avoid suppressing native submit.
        saveButton.setAttribute("aria-busy", "true");
        // Apply disabled on next frame after submit has been dispatched.
        requestAnimationFrame(() => {
          if (isSubmitting && saveButton) {
            saveButton.textContent = "Saving…";
            saveButton.disabled = true;
          }
        });
      }
      if (saveState) {
        saveState.textContent = "Saving…";
        saveState.classList.add("is-dirty");
        saveState.classList.remove("is-saved");
      }
      // Fallback timeout only if server never patches (network hang)
      const orig = saveButton ? (saveButton.dataset.origText || saveButton.textContent) : "Save changes";
      const fallback = setTimeout(() => {
        if (!isSubmitting) return;
        isSubmitting = false;
        if (saveButton) {
          saveButton.textContent = orig;
          saveButton.removeAttribute("aria-busy");
          saveButton.dataset.busy = "";
          saveButton.disabled = !isDirty();
        }
        if (saveState) {
          const dirty = isDirty();
          saveState.textContent = dirty ? "Unsaved changes" : "Saved";
          saveState.classList.toggle("is-dirty", dirty);
          saveState.classList.toggle("is-saved", !dirty);
        }
      }, 8000);

      // Observe server patch to restore Saved state authoritatively
      const region = document.getElementById("settings-content");
      if (region) {
        const obs = new MutationObserver(() => {
          const patchedState = region.querySelector(".editor-status");
          const text = patchedState ? patchedState.textContent.trim() : "";
          if (text === "Saved") {
            clearTimeout(fallback);
            isSubmitting = false;
            try { syncBaseline(); } catch (e) {}
            if (saveButton) {
              saveButton.textContent = orig;
              saveButton.removeAttribute("aria-busy");
              saveButton.dataset.busy = "";
              saveButton.disabled = true;
            }
            // Ensure state element reflects server's Saved (already patched)
            obs.disconnect();
          } else if (patchedState && patchedState.textContent.trim() === "Unsaved changes") {
            // validation error patched
            clearTimeout(fallback);
            isSubmitting = false;
            if (saveButton) {
              saveButton.textContent = orig;
              saveButton.removeAttribute("aria-busy");
              saveButton.dataset.busy = "";
              saveButton.disabled = false;
            }
            obs.disconnect();
          }
        });
        obs.observe(region, { childList: true, subtree: true });
        // Also disconnect fallback cleanup after 8s will handle.
        // Clean up observer on fallback timeout via fallback closure re-check
        setTimeout(() => { try { obs.disconnect(); } catch (e) {} }, 8500);
      }
    });

    // Ctrl/Cmd+S invokes same submit pathway via requestSubmit
    form.addEventListener("keydown", (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "s") {
        e.preventDefault();
        if (saveButton && !saveButton.disabled) {
          // Use requestSubmit so submit event fires with correct submitter
          if (typeof form.requestSubmit === "function") {
            form.requestSubmit(saveButton);
          } else {
            saveButton.click();
          }
        } else if (!saveButton) {
          if (typeof form.requestSubmit === "function") form.requestSubmit();
          else form.submit();
        }
      }
    });

    window.addEventListener("beforeunload", (e) => {
      if (isDirty() && !isSubmitting) { e.preventDefault(); e.returnValue = ""; }
    });

    const content = document.getElementById("settings-content");
    if (content) {
      const mo = new MutationObserver(() => {
        const newForm = document.getElementById("settings-form-general") || document.getElementById("settings-form-reading") || document.getElementById("settings-form-seo") || document.getElementById("settings-form-performance");
        if (newForm && newForm !== form) {
          mo.disconnect();
          initForm(newForm);
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
  document.querySelectorAll("form[id^='settings-form-']").forEach(initForm);
})();
