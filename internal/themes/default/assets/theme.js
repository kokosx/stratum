// Navigation: mobile drawer + dropdown menus.
(() => {
  const header = document.querySelector('.site-header');
  const menuToggle = document.querySelector('.mobile-menu-toggle');
  const primary = document.querySelector('[data-mobile-navigation], .primary-navigation');
  const breakpoint = Number.parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--st-mobile-breakpoint')) || 800;
  const mobileMedia = window.matchMedia(`(max-width: ${breakpoint}px)`);
  const applyBreakpoint = () => header?.classList.toggle('is-mobile', mobileMedia.matches);
  applyBreakpoint();
  mobileMedia.addEventListener('change', applyBreakpoint);

  let backdrop = null;
  const ensureBackdrop = () => {
    if (backdrop) return backdrop;
    backdrop = document.createElement('button');
    backdrop.type = 'button';
    backdrop.className = 'nav-backdrop';
    backdrop.setAttribute('aria-label', 'Close menu');
    backdrop.hidden = true;
    backdrop.addEventListener('click', closeAll);
    document.body.appendChild(backdrop);
    return backdrop;
  };

  const closeSubmenus = (except) => {
    document.querySelectorAll('.submenu-toggle[aria-expanded="true"]').forEach((button) => {
      if (button !== except) button.setAttribute('aria-expanded', 'false');
    });
  };

  function closeAll() {
    menuToggle?.setAttribute('aria-expanded', 'false');
    primary?.classList.remove('is-open');
    if (backdrop) backdrop.hidden = true;
    document.body.style.overflow = '';
    closeSubmenus(null);
  }

  menuToggle?.addEventListener('click', () => {
    const open = menuToggle.getAttribute('aria-expanded') !== 'true';
    menuToggle.setAttribute('aria-expanded', String(open));
    primary?.classList.toggle('is-open', open);
    if (open && primary?.classList.contains('mobile--drawer')) {
      ensureBackdrop().hidden = false;
      document.body.style.overflow = 'hidden';
    } else if (backdrop) {
      backdrop.hidden = true;
      document.body.style.overflow = '';
    }
  });

  document.addEventListener('click', (event) => {
    const button = event.target.closest('.submenu-toggle');
    if (button) {
      const open = button.getAttribute('aria-expanded') !== 'true';
      closeSubmenus(open ? button : null);
      button.setAttribute('aria-expanded', String(open));
      return;
    }
    // Click outside any open menu closes the dropdowns.
    if (!event.target.closest('.menu-item--has-children')) {
      closeSubmenus(null);
    }
  });

  document.addEventListener('keydown', (event) => {
    if (event.key !== 'Escape') return;
    const focusedToggle = document.activeElement?.closest?.('.menu-item')?.querySelector('.submenu-toggle');
    closeAll();
    (focusedToggle || menuToggle)?.focus();
  });
})();

// Code block copy button.
(() => {
  document.addEventListener('click', (event) => {
    const button = event.target.closest('[data-copy-code]');
    if (!button) return;
    const code = button.closest('.stratum-code')?.querySelector('code');
    if (!code) return;
    navigator.clipboard?.writeText(code.textContent).then(() => {
      const original = button.textContent;
      button.textContent = 'Copied';
      setTimeout(() => { button.textContent = original; }, 1500);
    });
  });
})();

// Tabs: build an accessible tablist from each panel's data-label and toggle
// visibility with full ARIA wiring and arrow-key support.
(() => {
  document.querySelectorAll('[data-tabs]').forEach((root) => {
    const nav = root.querySelector('[data-tabs-nav]');
    if (!nav) return;
    const tabs = Array.from(root.querySelectorAll(':scope > .stratum-tab'));
    if (tabs.length === 0) return;

    const buttons = tabs.map((tab, index) => {
      const id = `stratum-tab-${Math.random().toString(36).slice(2, 9)}`;
      tab.id = tab.id || id;
      tab.setAttribute('role', 'tabpanel');
      tab.setAttribute('tabindex', '0');
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'stratum-tab-btn';
      button.id = `${tab.id}-tab`;
      button.textContent = tab.dataset.label || `Tab ${index + 1}`;
      button.setAttribute('role', 'tab');
      button.setAttribute('aria-controls', tab.id);
      button.setAttribute('aria-selected', 'false');
      button.setAttribute('tabindex', index === 0 ? '0' : '-1');
      button.addEventListener('click', () => select(index));
      nav.appendChild(button);
      tab.setAttribute('aria-labelledby', button.id);
      return button;
    });

    const select = (index) => {
      tabs.forEach((other, otherIndex) => { other.hidden = otherIndex !== index; });
      buttons.forEach((other, otherIndex) => {
        other.setAttribute('aria-selected', String(otherIndex === index));
        other.setAttribute('tabindex', otherIndex === index ? '0' : '-1');
      });
    };

    const onKey = (event) => {
      const current = buttons.indexOf(document.activeElement);
      if (current === -1) return;
      let next = null;
      if (event.key === 'ArrowRight') next = (current + 1) % buttons.length;
      else if (event.key === 'ArrowLeft') next = (current - 1 + buttons.length) % buttons.length;
      else if (event.key === 'Home') next = 0;
      else if (event.key === 'End') next = buttons.length - 1;
      if (next === null) return;
      event.preventDefault();
      buttons[next].focus();
      select(next);
    };
    nav.addEventListener('keydown', onKey);

    select(0);
  });
})();
