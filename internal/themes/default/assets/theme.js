(() => {
  const header = document.querySelector('.site-header');
  const menuToggle = document.querySelector('.mobile-menu-toggle');
  const primary = document.querySelector('.primary-navigation');
  const breakpoint = Number.parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--st-mobile-breakpoint')) || 800;
  const mobileMedia = window.matchMedia(`(max-width: ${breakpoint}px)`);
  const applyBreakpoint = () => header?.classList.toggle('is-mobile', mobileMedia.matches);
  applyBreakpoint();
  mobileMedia.addEventListener('change', applyBreakpoint);

  const closeAll = () => {
    if (menuToggle && primary) {
      menuToggle.setAttribute('aria-expanded', 'false');
      primary.classList.remove('is-open');
    }
    document.querySelectorAll('.submenu-toggle[aria-expanded="true"]').forEach((button) => {
      button.setAttribute('aria-expanded', 'false');
    });
  };

  menuToggle?.addEventListener('click', () => {
    const open = menuToggle.getAttribute('aria-expanded') !== 'true';
    menuToggle.setAttribute('aria-expanded', String(open));
    primary.classList.toggle('is-open', open);
  });

  document.addEventListener('click', (event) => {
    const button = event.target.closest('.submenu-toggle');
    if (!button) return;
    const open = button.getAttribute('aria-expanded') !== 'true';
    button.setAttribute('aria-expanded', String(open));
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

// Tabs: build the navigation from each panel's data-label and toggle visibility.
(() => {
  document.querySelectorAll('[data-tabs]').forEach((root) => {
    const nav = root.querySelector('[data-tabs-nav]');
    if (!nav) return;
    const tabs = Array.from(root.querySelectorAll(':scope > .stratum-tab'));
    if (tabs.length === 0) return;
    const buttons = tabs.map((tab, index) => {
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'stratum-tab-btn';
      button.textContent = tab.dataset.label || `Tab ${index + 1}`;
      button.setAttribute('role', 'tab');
      button.addEventListener('click', () => {
        tabs.forEach((other, otherIndex) => { other.hidden = otherIndex !== index; });
        buttons.forEach((other, otherIndex) => other.setAttribute('aria-selected', String(otherIndex === index)));
      });
      nav.appendChild(button);
      return button;
    });
    tabs.forEach((tab, index) => { tab.hidden = index !== 0; });
    buttons[0]?.setAttribute('aria-selected', 'true');
  });
})();
