(() => {
  const root = document.querySelector('#theme-customizer');
  if (!root) return;
  const bootstrap = JSON.parse(document.querySelector('#appearance-bootstrap').textContent);
  const customization = bootstrap.customization;
  const schema = customization.schema;
  const defaults = Object.fromEntries(Object.entries(schema.settings).map(([key, setting]) => [key, setting.default]));
  const saved = { settings: structuredClone(customization.settings), customCSS: customization.customCSS || '' };
  const state = { settings: structuredClone(saved.settings), customCSS: saved.customCSS, dirty: false, activeGroup: schema.groups[0]?.id, viewport: 'desktop' };
  const groups = document.querySelector('#appearance-groups');
  const fields = document.querySelector('#appearance-fields');
  const title = document.querySelector('#appearance-group-title');
  const description = document.querySelector('#appearance-group-description');
  const save = document.querySelector('#appearance-save');
  const status = document.querySelector('#appearance-status');
  const frame = document.querySelector('#appearance-frame');
  const frameWrap = document.querySelector('#appearance-frame-wrap');
  let previewTimer;
  let previewController;

  const equal = (a, b) => JSON.stringify(a) === JSON.stringify(b);
  const setDirty = () => {
    state.dirty = !equal(state.settings, saved.settings) || state.customCSS !== saved.customCSS;
    save.disabled = !state.dirty;
    status.textContent = state.dirty ? 'Unsaved changes' : 'Saved';
  };
  const changed = (key) => !equal(state.settings[key], defaults[key]);

  function renderGroups() {
    groups.replaceChildren(...schema.groups.map((group) => {
      const button = document.createElement('button');
      button.type = 'button';
      button.className = state.activeGroup === group.id ? 'active' : '';
      button.textContent = group.label;
      button.addEventListener('click', () => { state.activeGroup = group.id; renderGroups(); renderFields(); });
      return button;
    }));
  }

  function renderFields() {
    const group = schema.groups.find((item) => item.id === state.activeGroup);
    title.textContent = group?.label || '';
    description.textContent = group?.description || '';
    const entries = Object.entries(schema.settings).filter(([, setting]) => setting.ui.group === state.activeGroup);
    const controls = entries.map(([key, setting]) => settingControl(key, setting));
    if (state.activeGroup === 'advanced') controls.push(customCSSControl());
    if (state.activeGroup === 'colors' && schema.presets?.length) controls.unshift(presetsControl());
    fields.replaceChildren(...controls);
  }

  function settingControl(key, setting) {
    const wrapper = document.createElement('div');
    wrapper.className = `theme-field${changed(key) ? ' is-modified' : ''}`;
    wrapper.dataset.setting = key;
    const heading = document.createElement('div'); heading.className = 'theme-field__heading';
    const label = document.createElement('label'); label.textContent = setting.ui.label; label.htmlFor = `setting-${key}`;
    const reset = document.createElement('button'); reset.type = 'button'; reset.className = 'field-reset'; reset.textContent = 'Reset'; reset.disabled = !changed(key);
    reset.addEventListener('click', () => update(key, structuredClone(defaults[key])));
    heading.append(label, reset); wrapper.append(heading);
    if (setting.ui.description) { const help = document.createElement('p'); help.textContent = setting.ui.description; wrapper.append(help); }
    wrapper.append(buildInput(key, setting));
    return wrapper;
  }

  function buildInput(key, setting) {
    const control = setting.ui.control;
    if ((control === 'segmented' || control === 'radio') && setting.enum) {
      const box = document.createElement('div'); box.className = `choice-control choice-control--${control}`; box.id = `setting-${key}`;
      setting.enum.forEach((value) => {
        const label = document.createElement('label'); const input = document.createElement('input');
        input.type = 'radio'; input.name = key; input.value = value; input.checked = state.settings[key] === value;
        input.addEventListener('change', () => update(key, value));
        const text = document.createElement('span'); text.textContent = humanize(value); label.append(input, text); box.append(label);
      });
      return box;
    }
    if ((control === 'select' || control === 'font') && setting.enum) {
      const select = document.createElement('select'); select.id = `setting-${key}`;
      setting.enum.forEach((value) => { const option = document.createElement('option'); option.value = value; option.textContent = humanize(value); option.selected = state.settings[key] === value; select.append(option); });
      select.addEventListener('change', () => update(key, select.value)); return select;
    }
    const box = document.createElement('div'); box.className = 'input-control';
    const input = document.createElement('input'); input.id = `setting-${key}`;
    if (control === 'checkbox') { input.type = 'checkbox'; input.checked = Boolean(state.settings[key]); input.addEventListener('change', () => update(key, input.checked)); box.append(input); return box; }
    if (control === 'color') { input.type = 'color'; input.value = state.settings[key]; const output = document.createElement('output'); output.textContent = state.settings[key].toUpperCase(); input.addEventListener('input', () => { output.textContent = input.value.toUpperCase(); update(key, input.value); }); box.append(input, output); return box; }
    input.type = control === 'range' ? 'range' : setting.type === 'number' ? 'number' : 'text'; input.value = state.settings[key];
    if (setting.minimum !== undefined) input.min = setting.minimum; if (setting.maximum !== undefined) input.max = setting.maximum;
    if (setting.type === 'number') input.step = Number.isInteger(setting.default) ? '1' : '0.05';
    const output = document.createElement('output'); if (control === 'range') { output.textContent = input.value; box.append(input, output); } else box.append(input);
    input.addEventListener('input', () => { const value = setting.type === 'number' ? Number(input.value) : input.value; if (control === 'range') output.textContent = input.value; update(key, value, false); });
    return box;
  }

  function presetsControl() {
    const wrapper = document.createElement('div'); wrapper.className = 'theme-presets';
    const heading = document.createElement('h3'); heading.textContent = 'Appearance presets'; wrapper.append(heading);
    schema.presets.forEach((preset) => { const button = document.createElement('button'); button.type = 'button'; button.textContent = preset.label; button.addEventListener('click', () => { Object.assign(state.settings, structuredClone(preset.values)); setDirty(); renderFields(); schedulePreview(); }); wrapper.append(button); });
    return wrapper;
  }

  function customCSSControl() {
    const wrapper = document.createElement('div'); wrapper.className = 'theme-field custom-css-field';
    const label = document.createElement('label'); label.htmlFor = 'theme-custom-css'; label.textContent = 'Custom CSS';
    const help = document.createElement('p'); help.textContent = 'Trusted administrator CSS appended after the theme stylesheet. JavaScript is not supported.';
    const textarea = document.createElement('textarea'); textarea.id = 'theme-custom-css'; textarea.rows = 14; textarea.maxLength = 204800; textarea.value = state.customCSS;
    textarea.addEventListener('input', () => { state.customCSS = textarea.value; setDirty(); schedulePreview(); });
    wrapper.append(label, help, textarea); return wrapper;
  }

  function update(key, value, rerender = true) { state.settings[key] = value; setDirty(); if (rerender) renderFields(); schedulePreview(); }
  function schedulePreview() { clearTimeout(previewTimer); previewTimer = setTimeout(refreshPreview, 350); }
  async function refreshPreview() {
    previewController?.abort(); previewController = new AbortController(); status.textContent = 'Updating preview…';
    try {
      const response = await fetch('/admin/appearance/preview', { method:'POST', headers:{'Content-Type':'application/json','X-CSRF-Token':bootstrap.csrfToken}, body:JSON.stringify({settings:state.settings, customCSS:state.customCSS, previewPath:bootstrap.previewPath}), signal:previewController.signal });
      if (!response.ok) throw new Error((await response.json()).error || 'Preview failed');
      frame.srcdoc = await response.text(); status.textContent = state.dirty ? 'Unsaved changes' : 'Saved';
    } catch (error) { if (error.name !== 'AbortError') status.textContent = error.message; }
  }

  save.addEventListener('click', async () => {
    save.disabled = true; status.textContent = 'Saving…';
    try {
      const response = await fetch('/admin/appearance', {method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':bootstrap.csrfToken},body:JSON.stringify({settings:state.settings,customCSS:state.customCSS})});
      const result = await response.json(); if (!response.ok) throw new Error(result.error || 'Save failed');
      saved.settings = structuredClone(result.customization.settings); saved.customCSS = result.customization.customCSS || ''; state.settings = structuredClone(saved.settings); state.customCSS = saved.customCSS; setDirty(); renderFields();
    } catch (error) { status.textContent = error.message; save.disabled = false; }
  });
  document.querySelector('#appearance-reset-group').addEventListener('click', () => { Object.entries(schema.settings).forEach(([key, setting]) => { if (setting.ui.group === state.activeGroup) state.settings[key] = structuredClone(defaults[key]); }); setDirty(); renderFields(); schedulePreview(); });
  document.querySelector('#appearance-reset-all').addEventListener('click', () => { state.settings = structuredClone(defaults); state.customCSS = ''; setDirty(); renderFields(); schedulePreview(); });
  const viewportButtons = document.querySelectorAll('.appearance-viewports [data-viewport]');
  viewportButtons.forEach((button) => button.addEventListener('click', () => { state.viewport = button.dataset.viewport; frameWrap.dataset.viewport = state.viewport; viewportButtons.forEach((item) => item.classList.toggle('active', item === button)); }));
  window.addEventListener('beforeunload', (event) => { if (state.dirty) event.preventDefault(); });
  function humanize(value) { return value.replace(/([a-z])([A-Z])/g, '$1 $2').replace(/^./, (letter) => letter.toUpperCase()); }
  renderGroups(); renderFields(); refreshPreview();
})();
