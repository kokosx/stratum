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
