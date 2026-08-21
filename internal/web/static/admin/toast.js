// Shared admin toast helper. Used by client-driven flows (e.g. the Appearance
// customizer) that talk to the backend over JSON; Datastar-driven flows append
// toasts directly via SSE. Both write into the same #admin-toast-region so the
// UX is consistent.
window.stratumToast = function stratumToast(kind, message) {
  const region = document.getElementById("admin-toast-region");
  if (!region) return;
  const toast = document.createElement("div");
  toast.className = "toast toast-" + kind;
  toast.setAttribute("role", "status");

  const text = document.createElement("span");
  text.className = "toast-message";
  text.textContent = message;
  toast.append(text);

  const close = document.createElement("button");
  close.type = "button";
  close.className = "toast-close";
  close.setAttribute("aria-label", "Dismiss");
  close.textContent = "×";
  close.addEventListener("click", () => toast.remove());
  toast.append(close);

  region.append(toast);
  if (kind !== "error") {
    setTimeout(() => toast.remove(), 4500);
  }
};
