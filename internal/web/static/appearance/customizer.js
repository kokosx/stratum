(() => {
  const root = document.querySelector('#theme-customizer');
  if (!root) return;
  const bootstrap = JSON.parse(document.querySelector('#appearance-bootstrap').textContent);
  const customization = bootstrap.customization;
  const schema = customization.schema;
  const defaults = Object.fromEntries(Object.entries(schema.settings).map(([k, s]) => [k, s.default]));
  const saved = { settings: structuredClone(customization.settings), customCSS: customization.customCSS || '' };
  const state = {
    settings: structuredClone(saved.settings),
    customCSS: saved.customCSS,
    dirty: false,
    expandedGroup: null,
    previewPath: bootstrap.previewPath || '/',
    viewport: 'desktop',
    search: '',
  };

  const equal = (a, b) => JSON.stringify(a) === JSON.stringify(b);
  const changed = (key) => !equal(state.settings[key], defaults[key]);

  function setDirty() {
    state.dirty = !equal(state.settings, saved.settings) || state.customCSS !== saved.customCSS;
    saveBtn.disabled = !state.dirty;
    statusEl.textContent = state.dirty ? 'Unsaved changes' : 'Saved';
  }

  const GROUP_ICONS = {
    colors: '&#9670;', typography: '&#9783;', spacing: '&#8693;', shape: '&#9723;',
    layout: '&#9638;', header: '&#9776;', navigation: '&#8644;', topbar: '&#9650;',
    footer: '&#9660;', buttons: '&#9654;', links: '&#8734;', advanced: '&#9881;',
  };

  const groupsEl = document.querySelector('#cz-groups');
  const presetsEl = document.querySelector('#cz-presets');
  const searchInput = document.querySelector('#cz-search');
  const saveBtn = document.querySelector('#appearance-save');
  const statusEl = document.querySelector('#appearance-status');
  const frame = document.querySelector('#appearance-frame');
  const frameWrap = document.querySelector('.cz-preview__frame');
  const loadingEl = document.querySelector('#cz-loading');
  const previewPageSelect = document.querySelector('#cz-preview-page');
  let lastScrollY = 0;

  let previewTimer;
  let previewController;
  let lastGoodSrcdoc = '';

  function renderPresets() {
    if (!schema.presets?.length) { presetsEl.replaceChildren(); return; }
    presetsEl.innerHTML = '<p class="cz-presets__title">Presets</p><div class="cz-presets__grid" id="cz-presets-grid"></div>';
    const grid = presetsEl.querySelector('#cz-presets-grid');
    schema.presets.forEach((preset) => {
      const card = document.createElement('button');
      card.type = 'button';
      card.className = 'cz-preset-card';
      const colors = extractPresetColors(preset);
      const swatch = document.createElement('div');
      swatch.className = 'cz-preset-card__swatch';
      colors.forEach((c) => { const s = document.createElement('span'); s.style.background = c; swatch.append(s); });
      const label = document.createElement('span');
      label.className = 'cz-preset-card__label';
      label.textContent = preset.label;
      card.append(swatch, label);
      card.addEventListener('click', () => {
        Object.assign(state.settings, structuredClone(preset.values));
        setDirty(); renderGroups(); schedulePreview();
      });
      grid.append(card);
    });
  }

  function extractPresetColors(preset) {
    const colors = [];
    for (const [key, val] of Object.entries(preset.values)) {
      if (typeof val === 'string' && /^#[0-9a-fA-F]{6}$/.test(val)) {
        colors.push(val);
        if (colors.length >= 3) break;
      }
    }
    while (colors.length < 3) colors.push('#e1e4ea');
    return colors;
  }

  function renderGroups() {
    const filter = state.search.toLowerCase().trim();
    groupsEl.replaceChildren();
    schema.groups.forEach((group) => {
      const groupSettings = Object.entries(schema.settings).filter(([, s]) => s.ui.group === group.id);
      const visibleSettings = filter
        ? groupSettings.filter(([key, s]) => s.ui.label.toLowerCase().includes(filter) || key.toLowerCase().includes(filter) || (s.ui.description || '').toLowerCase().includes(filter))
        : groupSettings;
      if (filter && visibleSettings.length === 0) return;

       const modifiedCount = groupSettings.filter(([key]) => changed(key)).length;
      const badgeCount = filter ? visibleSettings.length : modifiedCount;
      const isOpen = filter ? true : state.expandedGroup === group.id;

      const wrapper = document.createElement('div');
      wrapper.className = 'cz-group' + (isOpen ? ' open' : '');

      const header = document.createElement('button');
      header.type = 'button';
      header.className = 'cz-group__header' + (isOpen ? ' active' : '');
      header.setAttribute('aria-expanded', String(isOpen));

      const icon = document.createElement('span');
      icon.className = 'cz-group__icon';
      icon.innerHTML = GROUP_ICONS[group.id] || '&#9679;';

      const name = document.createElement('span');
      name.textContent = group.label;

      header.append(icon, name);

      if (badgeCount > 0) {
        const badge = document.createElement('span');
        badge.className = 'cz-group__badge';
        badge.textContent = badgeCount;
        header.append(badge);
      }

      const chevron = document.createElement('span');
      chevron.className = 'cz-group__chevron';
      chevron.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="m9 18 6-6-6-6"/></svg>';
      header.append(chevron);

      header.addEventListener('click', () => {
        state.expandedGroup = state.expandedGroup === group.id ? null : group.id;
        renderGroups();
      });

      const body = document.createElement('div');
      body.className = 'cz-group__body';

      if (group.description && !filter) {
        const desc = document.createElement('p');
        desc.className = 'cz-field__desc';
        desc.style.padding = '0 14px 4px';
        desc.textContent = group.description;
        body.append(desc);
      }

      visibleSettings.forEach(([key, setting]) => {
        body.append(renderField(key, setting));
      });

      if (group.id === 'advanced') {
        body.append(renderCustomCSS());
      }

      wrapper.append(header, body);
      groupsEl.append(wrapper);
    });
  }

  function renderField(key, setting) {
    const field = document.createElement('div');
    field.className = 'cz-field' + (changed(key) ? ' is-modified' : '');
    field.dataset.setting = key;

    const row = document.createElement('div');
    row.className = 'cz-field__row';

    const label = document.createElement('label');
    label.className = 'cz-field__label';
    label.textContent = setting.ui.label;
    label.htmlFor = 'setting-' + key;

    const reset = document.createElement('button');
    reset.type = 'button';
    reset.className = 'cz-field__reset';
    reset.textContent = 'Reset';
    reset.disabled = !changed(key);
    reset.addEventListener('click', () => update(key, structuredClone(defaults[key])));

    row.append(label, reset);
    field.append(row);

    if (setting.ui.description) {
      const desc = document.createElement('p');
      desc.className = 'cz-field__desc';
      desc.textContent = setting.ui.description;
      field.append(desc);
    }

    field.append(buildControl(key, setting));
    return field;
  }

  function buildControl(key, setting) {
    const ctrl = setting.ui.control;

    if ((ctrl === 'segmented' || ctrl === 'radio') && setting.enum) {
      const box = document.createElement('div');
      box.className = 'cz-ctrl cz-ctrl--segmented';
      box.id = 'setting-' + key;
      setting.enum.forEach((value) => {
        const lbl = document.createElement('label');
        const input = document.createElement('input');
        input.type = 'radio'; input.name = key; input.value = value;
        input.checked = state.settings[key] === value;
        input.addEventListener('change', () => update(key, value));
        const span = document.createElement('span');
        span.textContent = humanize(value);
        lbl.append(input, span);
        box.append(lbl);
      });
      return box;
    }

    if ((ctrl === 'select' || ctrl === 'font') && setting.enum) {
      const box = document.createElement('div');
      box.className = 'cz-ctrl';
      const select = document.createElement('select');
      select.id = 'setting-' + key;
      setting.enum.forEach((value) => {
        const opt = document.createElement('option');
        opt.value = value; opt.textContent = humanize(value);
        opt.selected = state.settings[key] === value;
        select.append(opt);
      });
      select.addEventListener('change', () => update(key, select.value));
      box.append(select);
      return box;
    }

    if (ctrl === 'checkbox') {
      const box = document.createElement('div');
      box.className = 'cz-ctrl cz-ctrl--check';
      const input = document.createElement('input');
      input.type = 'checkbox'; input.id = 'setting-' + key;
      input.checked = Boolean(state.settings[key]);
      input.addEventListener('change', () => update(key, input.checked));
      const lbl = document.createElement('label');
      lbl.htmlFor = 'setting-' + key;
      lbl.textContent = state.settings[key] ? 'Enabled' : 'Disabled';
      input.addEventListener('change', () => { lbl.textContent = input.checked ? 'Enabled' : 'Disabled'; });
      box.append(input, lbl);
      return box;
    }

    if (ctrl === 'color') {
      const box = document.createElement('div');
      box.className = 'cz-ctrl cz-ctrl--color';
      const input = document.createElement('input');
      input.type = 'color'; input.id = 'setting-' + key;
      input.value = state.settings[key];
      const output = document.createElement('output');
      output.textContent = state.settings[key].toUpperCase();
      input.addEventListener('input', () => {
        output.textContent = input.value.toUpperCase();
        update(key, input.value, false);
      });
      box.append(input, output);
      return box;
    }

    if (ctrl === 'range') {
      const box = document.createElement('div');
      box.className = 'cz-ctrl cz-ctrl--range';
      const input = document.createElement('input');
      input.type = 'range'; input.id = 'setting-' + key;
      input.value = state.settings[key];
      if (setting.minimum !== undefined) input.min = setting.minimum;
      if (setting.maximum !== undefined) input.max = setting.maximum;
      input.step = Number.isInteger(setting.default) ? '1' : '0.05';
      const output = document.createElement('output');
      output.textContent = formatRangeValue(key, input.value);
      input.addEventListener('input', () => {
        output.textContent = formatRangeValue(key, input.value);
        const val = setting.type === 'number' ? Number(input.value) : input.value;
        update(key, val, false);
      });
      box.append(input, output);
      return box;
    }

    const box = document.createElement('div');
    box.className = 'cz-ctrl cz-ctrl--' + (setting.type === 'number' ? 'number' : 'text');
    const input = document.createElement('input');
    input.id = 'setting-' + key;
    input.type = setting.type === 'number' ? 'number' : 'text';
    input.value = state.settings[key];
    if (setting.minimum !== undefined) input.min = setting.minimum;
    if (setting.maximum !== undefined) input.max = setting.maximum;
    if (setting.type === 'number') input.step = Number.isInteger(setting.default) ? '1' : '0.05';
    input.addEventListener('input', () => {
      const val = setting.type === 'number' ? Number(input.value) : input.value;
      update(key, val, false);
    });
    box.append(input);
    return box;
  }

  function formatRangeValue(key, value) {
    if (key.endsWith('.size') || key.endsWith('Size') || key.endsWith('Width') || key.endsWith('Height') ||
        key.endsWith('Spacing') || key.endsWith('Gap') || key.endsWith('Padding') || key.endsWith('paddingX') || key.endsWith('paddingY') ||
        key.includes('radius') || key.includes('Radius') || key === 'border.width' || key.includes('Weight')) {
      return value;
    }
    return value;
  }

  function renderCustomCSS() {
    const wrapper = document.createElement('div');
    wrapper.className = 'cz-field';
    const row = document.createElement('div');
    row.className = 'cz-field__row';
    const label = document.createElement('label');
    label.className = 'cz-field__label';
    label.textContent = 'Custom CSS';
    label.htmlFor = 'theme-custom-css';
    row.append(label);
    wrapper.append(row);
    const desc = document.createElement('p');
    desc.className = 'cz-field__desc';
    desc.textContent = 'Trusted CSS appended after the theme stylesheet.';
    wrapper.append(desc);
    const box = document.createElement('div');
    box.className = 'cz-ctrl';
    const textarea = document.createElement('textarea');
    textarea.id = 'theme-custom-css';
    textarea.rows = 8;
    textarea.maxLength = 204800;
    textarea.value = state.customCSS;
    textarea.addEventListener('input', () => {
      state.customCSS = textarea.value;
      setDirty();
      schedulePreview();
    });
    box.append(textarea);
    wrapper.append(box);
    return wrapper;
  }

  function update(key, value, rerender = true) {
    state.settings[key] = value;
    setDirty();
    if (rerender) renderGroups();
    schedulePreview();
  }

  function schedulePreview() {
    clearTimeout(previewTimer);
    previewTimer = setTimeout(refreshPreview, 300);
  }

  async function refreshPreview() {
    previewController?.abort();
    previewController = new AbortController();
    loadingEl.classList.add('visible');
    statusEl.textContent = 'Updating preview\u2026';
    try {
      const response = await fetch('/admin/appearance/preview', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': bootstrap.csrfToken },
        body: JSON.stringify({ settings: state.settings, customCSS: state.customCSS, previewPath: state.previewPath }),
        signal: previewController.signal,
      });
      if (!response.ok) {
        const body = await response.json().catch(() => ({}));
        throw new Error(body.error || 'Preview failed');
      }
      const html = await response.text();
      lastGoodSrcdoc = html;
      try {
        const doc = frame.contentDocument;
        if (doc) lastScrollY = doc.documentElement.scrollTop || doc.body.scrollTop || 0;
      } catch (_) { /* cross-origin guard */ }
      frame.onload = () => {
        loadingEl.classList.remove('visible');
        try { frame.contentWindow.scrollTo(0, lastScrollY); } catch (_) { /* ignore */ }
        statusEl.textContent = state.dirty ? 'Unsaved changes' : 'Saved';
      };
      frame.srcdoc = html;
    } catch (error) {
      if (error.name === 'AbortError') return;
      loadingEl.classList.remove('visible');
      if (lastGoodSrcdoc) frame.srcdoc = lastGoodSrcdoc;
      statusEl.textContent = state.dirty ? 'Unsaved changes' : 'Saved';
      window.stratumToast('error', error.message);
    }
  }

  saveBtn.addEventListener('click', async () => {
    saveBtn.disabled = true;
    statusEl.textContent = 'Saving\u2026';
    try {
      const response = await fetch('/admin/appearance', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': bootstrap.csrfToken },
        body: JSON.stringify({ settings: state.settings, customCSS: state.customCSS }),
      });
      const result = await response.json();
      if (!response.ok) throw new Error(result.error || 'Save failed');
      saved.settings = structuredClone(result.customization.settings);
      saved.customCSS = result.customization.customCSS || '';
      state.settings = structuredClone(saved.settings);
      state.customCSS = saved.customCSS;
      setDirty();
      renderGroups();
      window.stratumToast('success', 'Theme settings saved.');
    } catch (error) {
      statusEl.textContent = error.message;
      window.stratumToast('error', error.message);
      saveBtn.disabled = false;
    }
  });

  document.querySelector('#appearance-reset-all').addEventListener('click', () => {
    state.settings = structuredClone(defaults);
    state.customCSS = '';
    setDirty(); renderGroups(); schedulePreview();
  });

  const viewportButtons = document.querySelectorAll('.cz-viewports [data-viewport]');
  viewportButtons.forEach((btn) => btn.addEventListener('click', () => {
    state.viewport = btn.dataset.viewport;
    frameWrap.dataset.viewport = state.viewport;
    viewportButtons.forEach((b) => b.classList.toggle('active', b === btn));
  }));

  if (previewPageSelect) {
    previewPageSelect.addEventListener('change', () => {
      state.previewPath = previewPageSelect.value;
      schedulePreview();
    });
  }

  searchInput.addEventListener('input', () => {
    state.search = searchInput.value;
    renderGroups();
  });

  window.addEventListener('beforeunload', (e) => { if (state.dirty) e.preventDefault(); });

  function humanize(value) {
    return value.replace(/([a-z])([A-Z])/g, '$1 $2').replace(/^./, (l) => l.toUpperCase());
  }

  renderPresets();
  renderGroups();
  refreshPreview();
})();
