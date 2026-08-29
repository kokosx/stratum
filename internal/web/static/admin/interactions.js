// Stratum Admin interactions — lightweight vanilla JS for coherent behaviour
// Handles: double-submit protection + loading feedback, focus management helpers,
// and progressive enhancement fallbacks. No framework, server remains source of truth.
(function(){
  // Double-submit protection + subtle loading feedback
  document.addEventListener('click', function(e){
    const btn = e.target.closest('button[type="submit"], input[type="submit"]');
    if(!btn) return;
    // Don't block if already busy
    if(btn.dataset.busy === '1') { e.preventDefault(); e.stopPropagation(); return; }
    const form = btn.closest('form');
    if(!form) return;
    // Only guard important actions
    const isDestructive = btn.classList.contains('button-danger') || btn.textContent.toLowerCase().includes('delete') || btn.textContent.toLowerCase().includes('trash') || btn.textContent.toLowerCase().includes('remove');
    const isSave = btn.textContent.toLowerCase().includes('save') || btn.textContent.toLowerCase().includes('create') || btn.textContent.toLowerCase().includes('publish') || btn.textContent.toLowerCase().includes('update') || btn.textContent.toLowerCase().includes('add');
    // Light guard only for admin forms (avoid global overload)
    if(!isDestructive && !isSave) return;
    // For confirm dialogs, don't disable if user cancelled
    // The confirm handler runs via onsubmit/onClick before this? Click fires before submit, but confirm return false prevents default via inline handler -> we check defaultPrevented
    // Defer disable to next tick to allow confirm to cancel
    setTimeout(function(){
      if(e.defaultPrevented) return;
      // Check if form validation would prevent submission
      if(form && !form.checkValidity()) return;
      btn.dataset.busy = '1';
      const orig = btn.textContent;
      btn.dataset.origText = orig;
      const lower = orig.trim().toLowerCase();
      if(lower.includes('publish')) btn.textContent = 'Publishing…';
      else if(lower.includes('save')) btn.textContent = 'Saving…';
      else if(lower === 'create' || lower.includes('create')) btn.textContent = 'Creating…';
      else if(lower.includes('delete') || lower.includes('trash') || lower.includes('remove')) btn.textContent = 'Deleting…';
      else if(lower.includes('add')) btn.textContent = 'Adding…';
      else if(lower.includes('update')) btn.textContent = 'Updating…';
      else btn.textContent = orig + '…';
      btn.disabled = true;
      // Re-enable after response (Datastar SSE will patch DOM, but for safety timeout)
      const reenable = function(){
        if(btn.dataset.busy !== '1') return;
        btn.textContent = orig;
        btn.disabled = false;
        btn.dataset.busy = '';
      };
      // Listen for Datastar toast or flash to know completion
      const region = document.getElementById('admin-toast-region');
      let obs;
      if(region){
        obs = new MutationObserver(function(){
          // If toast appeared, re-enable after short delay
          reenable();
          setTimeout(function(){ if(obs) obs.disconnect(); }, 500);
        });
        obs.observe(region, {childList:true});
      }
      // Fallback timeout 8s
      setTimeout(reenable, 8000);
      // Also re-enable on fetch error completion via fetch wrapper once
      const origFetch = window.fetch;
      let fetched = false;
      window.fetch = function(...args){
        const p = origFetch.apply(this, args);
        p.then(function(){ if(!fetched){ fetched=true; setTimeout(reenable, 400); } }, function(){ if(!fetched){ fetched=true; setTimeout(reenable, 400);} });
        setTimeout(function(){ window.fetch = origFetch; }, 6000);
        return p;
      };
    }, 0);
  }, true);

  // Enhance field-card headers for keyboard accessibility (content_types)
  document.querySelectorAll('.field-card__header').forEach(function(header){
    if(header.tagName.toLowerCase() === 'button') return;
    header.setAttribute('tabindex','0');
    header.setAttribute('role','button');
    header.setAttribute('aria-expanded','false');
    const body = header.nextElementSibling;
    if(body) body.hidden = body.hidden; // preserve
    header.addEventListener('keydown', function(e){
      if(e.key === 'Enter' || e.key === ' '){ e.preventDefault(); header.click(); }
    });
    header.addEventListener('click', function(){
      const expanded = header.getAttribute('aria-expanded') === 'true';
      header.setAttribute('aria-expanded', String(!expanded));
    });
  });

  // Media picker focus return
  const origOpenMediaPicker = window.openMediaPicker;
  if(origOpenMediaPicker){
    window.openMediaPicker = function(opts){
      const active = document.activeElement;
      const wrappedOnSelect = opts.onSelect;
      opts.onSelect = function(asset){
        if(wrappedOnSelect) wrappedOnSelect(asset);
        // Return focus to trigger or next logical control
        setTimeout(function(){
          if(active && active.focus) active.focus();
          else {
            const fallback = document.querySelector('[data-custom-media-picker], #entry-featured-choose, #entry-social-choose, #site-icon-choose, #site-social-choose');
            if(fallback) fallback.focus();
          }
        }, 0);
      };
      return origOpenMediaPicker(opts);
    };
  }

  // Form editor: add Ctrl+S and dirty guard
  const formEditor = document.getElementById('form-editor');
  if(formEditor){
    const saveBtn = document.querySelector('button[form="form-editor"]') || formEditor.querySelector('button[type="submit"]');
    const initial = JSON.stringify(Array.from(new FormData(formEditor).entries()));
    function isFormDirty(){
      return JSON.stringify(Array.from(new FormData(formEditor).entries())) !== initial;
    }
    window.addEventListener('beforeunload', function(e){ if(isFormDirty()){ e.preventDefault(); e.returnValue=''; } });
    document.addEventListener('keydown', function(e){
      if((e.metaKey || e.ctrlKey) && e.key.toLowerCase()==='s'){
        e.preventDefault();
        if(saveBtn) saveBtn.click();
      }
    });
    // Focus first field after Add field
    // hook into existing addField already focuses label, but ensure
  }

  // Entry form tabs keyboard support (ArrowLeft/Right/Home/End)
  const tablist = document.querySelector('[role="tablist"].entry-tabs');
  if(tablist){
    const tabs = Array.from(tablist.querySelectorAll('[role="tab"]'));
    tablist.addEventListener('keydown', function(e){
      const idx = tabs.indexOf(document.activeElement);
      if(idx === -1) return;
      let next = null;
      if(e.key === 'ArrowRight') next = tabs[(idx+1)%tabs.length];
      else if(e.key === 'ArrowLeft') next = tabs[(idx-1+tabs.length)%tabs.length];
      else if(e.key === 'Home') next = tabs[0];
      else if(e.key === 'End') next = tabs[tabs.length-1];
      if(next){ e.preventDefault(); next.focus(); next.click(); }
    });
  }

  // Ensure menus select navigation doesn't lose focus; keep as is but ensure onchange is accessible via keyboard (already native select)

})();
