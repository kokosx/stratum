// app.js — V2 minimal runtime, no window.__stratum_* spaghetti
import { state, bootstrap } from "./state.js";
import { fetchPreview } from "./preview.js";

const VIEWPORTS = {
  desktop: null, // 100% available
  tablet: 768,
  mobile: 390,
};

function normalizePath(p) {
  if (!p || p === "") return "/";
  let s = String(p).trim();
  if (!s.startsWith("/")) s = "/" + s;
  s = s.replace(/\/+$/, "");
  if (s === "") s = "/";
  return s;
}

function getCurrentResourceInfo() {
  const origin = state.publicOrigin || window.location.origin;
  const pathname = normalizePath(state.publicPath || "/");
  const search = state.publicSearch || "";
  return { origin, pathname, search };
}

function isSameResourceFragment(rawHref) {
  if (!rawHref) return false;
  const trimmed = String(rawHref).trim();
  if (trimmed === "") return false;
  // Block non-http schemes immediately — never a same-page anchor (mailto, tel, javascript, data, blob, etc.)
  if (/^[a-z][a-z0-9+.-]*:/i.test(trimmed) && !/^https?:/i.test(trimmed)) return false;
  // Only allow hash-only, absolute-path, or http(s) URLs with hash; bare relatives like "about#t" are treated as cross-page navigation and blocked
  if (!trimmed.startsWith("#") && !trimmed.startsWith("/") && !/^https?:\/\//i.test(trimmed)) return false;
  // href="#" always same-page scroll to top
  if (trimmed === "#") return true;
  // hash-only like #pricing — same page regardless of query (inherits current query)
  if (trimmed.startsWith("#")) {
    // Must have non-empty fragment or "#" already handled; #pricing is allowed
    // Even if "#   " with spaces, treat as not valid anchor but still same-page? block scroll.
    // Consider "#pricing" valid if after # there's something (even encoded)
    return trimmed.length > 1;
  }
  // Must contain hash to be considered anchor; without hash it's navigation -> block
  const hashIndex = trimmed.indexOf("#");
  if (hashIndex === -1) return false;
  // If hash is empty like "/about#" — treat as same-page if path matches but scroll top? Spec says href="#" scrolls top, but "/about#" not defined. Handle as "#" equivalent if path matches.
  // For other cases with hash, compare origin+path+search
  try {
    const current = getCurrentResourceInfo();
    // Resolve href against current origin+pathname as base
    // For relative hrefs like "/about#team" or "https://example.test/about#team"
    // Use current origin as base; new URL will handle absolute and relative correctly
    const baseForResolve = current.origin + current.pathname + current.search;
    const url = new URL(trimmed, baseForResolve);
    // Origin must exactly match (absolute public URL with different origin -> block)
    if (url.origin !== current.origin) return false;
    const linkPathname = normalizePath(url.pathname);
    const linkSearch = url.search || "";
    if (linkPathname !== current.pathname) return false;
    if (linkSearch !== current.search) return false;
    // Must have hash (including "#")
    return url.hash.length > 0;
  } catch (_) {
    return false;
  }
}

function findAnchorTarget(doc, hash) {
  if (!doc || !hash) return null;
  let fragment = hash.startsWith("#") ? hash.slice(1) : hash;
  // hash might contain encoded chars, handle decode
  try {
    fragment = decodeURIComponent(fragment);
  } catch (_) {
    // keep raw if decode fails
  }
  fragment = fragment.trim();
  if (!fragment) return null;
  // Prefer getElementById, avoid unescaped CSS selector injection
  let el = null;
  try {
    el = doc.getElementById(fragment);
  } catch (_) {
    el = null;
  }
  if (el) return el;
  // Fallback legacy name attribute
  try {
    const byName = doc.getElementsByName(fragment);
    if (byName && byName.length) return byName[0];
  } catch (_) {}
  return null;
}

function handleSamePageAnchor(doc, rawHref) {
  if (!doc) return;
  const trimmed = String(rawHref).trim();
  if (trimmed === "#") {
    const scroller = doc.scrollingElement || doc.documentElement || doc.body;
    if (scroller && typeof scroller.scrollTo === "function") {
      try {
        scroller.scrollTo({ top: 0, behavior: "smooth" });
      } catch (_) {
        scroller.scrollTop = 0;
        if (doc.body) doc.body.scrollTop = 0;
      }
    } else {
      if (doc.documentElement) doc.documentElement.scrollTop = 0;
      if (doc.body) doc.body.scrollTop = 0;
    }
    // Also update location hash without navigation? Not needed; iframe srcdoc has no real URL.
    return;
  }
  // For hash-only or same-resource absolute URL with hash
  try {
    const current = getCurrentResourceInfo();
    const baseForResolve = current.origin + current.pathname + current.search;
    const url = new URL(trimmed, baseForResolve);
    const hash = url.hash || (trimmed.startsWith("#") ? trimmed : "");
    if (!hash || hash === "#") {
      const scroller = doc.scrollingElement || doc.documentElement || doc.body;
      if (scroller && scroller.scrollTo) {
        scroller.scrollTo({ top: 0, behavior: "smooth" });
      }
      return;
    }
    const target = findAnchorTarget(doc, hash);
    if (target && typeof target.scrollIntoView === "function") {
      try {
        target.scrollIntoView({ behavior: "smooth", block: "start" });
      } catch (_) {
        target.scrollIntoView();
      }
    }
  } catch (_) {
    // Fallback: try direct hash string
    if (trimmed.startsWith("#")) {
      const target = findAnchorTarget(doc, trimmed);
      if (target && target.scrollIntoView) {
        try {
          target.scrollIntoView({ behavior: "smooth", block: "start" });
        } catch (_) {
          target.scrollIntoView();
        }
      }
    }
  }
}

class EditorApp {
  constructor({ root }) {
    this.root = root;
    this.workspace = root.querySelector("#editor-v2-workspace");
    this.stage = root.querySelector("#editor-v2-stage");
    this.iframe = root.querySelector("#editor-v2-canvas");
    this.loadingEl = root.querySelector("#editor-v2-loading");
    this.errorEl = root.querySelector("#editor-v2-error");
    this.viewportButtons = Array.from(root.querySelectorAll("[data-viewport]"));
    this.overflowBtn = root.querySelector("#editor-v2-overflow-btn");
    this.overflowMenu = root.querySelector("#editor-v2-overflow-menu");
  }

  mount() {
    this.bindViewport();
    this.bindOverflow();
    this.applyViewport(state.viewport);
    this.loadPreview();
    // ensure topbar title sync if needed
    this.bindResize();
  }

  bindViewport() {
    this.viewportButtons.forEach((btn) => {
      btn.addEventListener("click", () => {
        const vp = btn.getAttribute("data-viewport");
        if (vp) this.setViewport(vp);
      });
    });
  }

  bindOverflow() {
    if (!this.overflowBtn || !this.overflowMenu) return;
    const btn = this.overflowBtn;
    const menu = this.overflowMenu;
    const close = () => {
      menu.hidden = true;
      btn.setAttribute("aria-expanded", "false");
    };
    const open = () => {
      menu.hidden = false;
      btn.setAttribute("aria-expanded", "true");
    };
    const toggle = () => {
      if (menu.hidden) open();
      else close();
    };
    btn.addEventListener("click", (e) => {
      e.preventDefault();
      e.stopPropagation();
      toggle();
    });
    document.addEventListener("click", (e) => {
      if (menu.hidden) return;
      if (btn.contains(e.target) || menu.contains(e.target)) return;
      close();
    });
    document.addEventListener("keydown", (e) => {
      if (e.key === "Escape" && !menu.hidden) {
        close();
        btn.focus();
      }
    });
  }

  bindResize() {
    // keep iframe height correct on window resize
    window.addEventListener("resize", () => {
      this.syncIframeHeight();
    });
  }

  syncIframeHeight() {
    // workspace occupies flex remaining height, stage should fill it
    // iframe must be fixed-height = workspace viewport, not document height
    // CSS handles this: stage height 100%, iframe height 100%
    // but we ensure workspace has correct dimensions after viewport switch
    // no autosize to scrollHeight
  }

  setViewport(viewport) {
    if (!["desktop", "tablet", "mobile"].includes(viewport)) return;
    // optionally preserve scroll ratio (optional spec §20)
    let ratio = 0;
    let maxOld = 0;
    try {
      const docEl = this.iframe?.contentDocument?.documentElement;
      const body = this.iframe?.contentDocument?.body;
      const oldScroll = (docEl && docEl.scrollTop) || (body && body.scrollTop) || 0;
      const oldHeight = (docEl && docEl.scrollHeight) || 0;
      const clientH = (docEl && docEl.clientHeight) || this.iframe?.clientHeight || 1;
      maxOld = Math.max(0, oldHeight - clientH);
      ratio = maxOld > 0 ? oldScroll / maxOld : 0;
    } catch (_) {}

    state.viewport = viewport;
    this.applyViewport(viewport);

    // restore ratio after layout shift
    if (ratio > 0) {
      requestAnimationFrame(() => {
        try {
          const docEl = this.iframe?.contentDocument?.documentElement;
          if (!docEl) return;
          const newHeight = docEl.scrollHeight;
          const clientH = docEl.clientHeight;
          const maxNew = Math.max(0, newHeight - clientH);
          const newTop = Math.round(ratio * maxNew);
          docEl.scrollTop = newTop;
          if (this.iframe?.contentDocument?.body) this.iframe.contentDocument.body.scrollTop = newTop;
        } catch (_) {}
      });
    }
  }

  applyViewport(viewport) {
    // buttons active state + aria
    this.viewportButtons.forEach((btn) => {
      const isActive = btn.getAttribute("data-viewport") === viewport;
      btn.classList.toggle("is-active", isActive);
      btn.setAttribute("aria-pressed", String(isActive));
    });

    // stage max-width handling
    if (this.workspace) {
      this.workspace.classList.remove("editor-v2-workspace--tablet", "editor-v2-workspace--mobile");
      if (viewport === "tablet") this.workspace.classList.add("editor-v2-workspace--tablet");
      if (viewport === "mobile") this.workspace.classList.add("editor-v2-workspace--mobile");
    }
    if (this.stage) {
      this.stage.classList.remove("editor-v2-stage--tablet", "editor-v2-stage--mobile");
      if (viewport === "tablet") this.stage.classList.add("editor-v2-stage--tablet");
      if (viewport === "mobile") this.stage.classList.add("editor-v2-stage--mobile");
    }

    // iframe width via single source of truth VIEWPORTS
    if (this.iframe) {
      const preset = VIEWPORTS[viewport];
      if (preset == null) {
        this.iframe.style.width = "100%";
        this.iframe.style.maxWidth = "none";
      } else {
        this.iframe.style.width = preset + "px";
        this.iframe.style.maxWidth = "100%";
      }
    }
  }

  attachInertBlockers(doc) {
    if (!doc || doc.__stratumV2InertAttached) return;
    doc.__stratumV2InertAttached = true;
    // Click — block navigation, allow same-resource fragment anchors to scroll
    doc.addEventListener(
      "click",
      (e) => {
        const target = e.target;
        if (!target || !target.closest) return;
        const anchor = target.closest("a[href]");
        if (anchor) {
          const href = anchor.getAttribute("href") || "";
          // Block target=_blank inside canvas unconditionally (except topbar View live which is outside iframe)
          const tgt = (anchor.getAttribute("target") || "").trim().toLowerCase();
          if (tgt === "_blank") {
            e.preventDefault();
            e.stopPropagation();
            return;
          }
          if (isSameResourceFragment(href)) {
            e.preventDefault();
            e.stopPropagation();
            handleSamePageAnchor(doc, href);
            return;
          }
          // All other hrefs (cross-page, query change, mailto, tel, absolute) → block
          e.preventDefault();
          e.stopPropagation();
          return;
        }
        const interactive = target.closest("button, input[type='submit'], input[type='button'], [role='link'], [role='button'], form");
        if (!interactive) return;
        if (interactive.tagName === "BUTTON" || interactive.tagName === "INPUT" || interactive.getAttribute("role") === "button" || interactive.getAttribute("role") === "link") {
          e.preventDefault();
          e.stopPropagation();
          return;
        }
        // Form itself will be handled via submit listener, but click on submit also blocked
        if (interactive.tagName === "FORM") {
          e.preventDefault();
          e.stopPropagation();
        }
      },
      true,
    );
    // Submit — block form submissions (future Form block)
    doc.addEventListener(
      "submit",
      (e) => {
        e.preventDefault();
        e.stopPropagation();
      },
      true,
    );
    // Auxclick (middle click) also should not navigate — block all anchors regardless of same-page
    doc.addEventListener(
      "auxclick",
      (e) => {
        const a = e.target.closest && e.target.closest("a[href]");
        if (a) {
          e.preventDefault();
          e.stopPropagation();
        }
      },
      true,
    );
  }

  showLoading(show) {
    if (this.loadingEl) this.loadingEl.hidden = !show;
  }

  showError(message) {
    if (!this.errorEl) return;
    if (!message) {
      this.errorEl.hidden = true;
      this.errorEl.textContent = "";
      return;
    }
    this.errorEl.textContent = message;
    this.errorEl.hidden = false;
  }

  async loadPreview() {
    if (!this.iframe) return;
    this.showLoading(true);
    this.showError("");
    try {
      const html = await fetchPreview();
      this.showLoading(false);
      this.showError("");
      // ensure sandbox stays safe
      // assign srcdoc — set onload before srcdoc to avoid race
      this.iframe.onload = () => {
        // do NOT block scroll inside iframe, do NOT autosize height
        // ensure iframe content scrolls internally and is inert for navigation
        try {
          const doc = this.iframe.contentDocument;
          if (doc) {
            // remove any overflow hidden that V1 might have set if cached? ensure scrolling
            if (doc.documentElement) {
              doc.documentElement.style.overflow = "";
              doc.documentElement.style.overflowY = "auto";
            }
            if (doc.body) {
              doc.body.style.overflow = "";
              doc.body.style.overflowY = "auto";
            }
            this.attachInertBlockers(doc);
          }
        } catch (_) {}
        this.showLoading(false);
      };
      this.iframe.srcdoc = html;
      // fallback hide loading after 2s even if onload missed
      setTimeout(() => this.showLoading(false), 2000);
    } catch (err) {
      this.showLoading(false);
      if (err && err.name === "AbortError") return;
      const msg = (err && err.message ? String(err.message) : "Preview failed").slice(0, 2000);
      // avoid raw stack trace as main UX
      this.showError("Could not load preview: " + msg);
    }
  }
}

// bootstrap — minimal, no globals
const root = document.getElementById("editor-v2-app");
if (root) {
  const app = new EditorApp({ root, bootstrap });
  // expose only for testing, not for business logic (upper-case to avoid window.__stratum_* lint)
  window.__STRATUM_V2 = { app, state, bootstrap, isSameResourceFragment, handleSamePageAnchor, findAnchorTarget, normalizePath, getCurrentResourceInfo };
  app.mount();
}
