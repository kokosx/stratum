// Creator studio client — no framework, preview uses real theme via /admin/creator/preview
(function(){
  var form = document.getElementById('creator-form');
  if(!form) return;
  document.documentElement.classList.add('has-js');
  var studio = document.querySelector('[data-creator-studio]');
  var initialStepRaw = studio ? parseInt(studio.getAttribute('data-initial-step')||"1",10) : 1;
  if(!(initialStepRaw>=1 && initialStepRaw<=5)) initialStepRaw=1;
  var hasValidationErr = studio && studio.getAttribute('data-has-validation-err')==="1";
  // wizard_step hidden input
  var wizardStepInput = document.getElementById('wizard-step-input');
  var steps=[1,2,3,4,5].map(function(n){return form.querySelector('[data-step="'+n+'"]')});
  var backBtn=document.getElementById('creator-back');
  var contBtn=document.getElementById('creator-continue');
  var submitBtn=document.getElementById('creator-submit');
  var stepper=document.querySelectorAll('.creator-stepper li');
  var presetInputs=form.querySelectorAll('input[name="preset"]');
  var paletteInputs=form.querySelectorAll('input[name="palette"]');
  var headerInputs=form.querySelectorAll('input[name="header_style"]');
  var footerInputs=form.querySelectorAll('input[name="footer_style"]');
  var current=initialStepRaw;
  // Completed set should include all steps up to initial on validation error, so user can navigate back.
  var completed=new Set([1]);
  if(hasValidationErr && initialStepRaw>1){
    for(var s=1;s<=initialStepRaw;s++) completed.add(s);
  }
  var defaults={"blog":{"palette":"clay","header":"classic","footer":"simple"},"portfolio":{"palette":"ink","header":"minimal","footer":"split"},"landing":{"palette":"indigo","header":"minimal","footer":"centered"},"products":{"palette":"ink","header":"classic","footer":"split"},"local-business":{"palette":"forest","header":"classic","footer":"split"},"simple-site":{"palette":"ink","header":"minimal","footer":"simple"},"magazine":{"palette":"clay","header":"centered","footer":"split"},"agency":{"palette":"indigo","header":"bold","footer":"centered"},"knowledge-base":{"palette":"forest","header":"stacked","footer":"editorial"}};
  var touched={palette:false,header:false,footer:false};
  paletteInputs.forEach(function(el){el.addEventListener('change',function(){touched.palette=true;})});
  headerInputs.forEach(function(el){el.addEventListener('change',function(){touched.header=true;})});
  footerInputs.forEach(function(el){el.addEventListener('change',function(){touched.footer=true;})});
  function setChecked(name,val){
    var ins=document.querySelectorAll('input[name="'+name+'"]');
    ins.forEach(function(el){
      el.checked=el.value===val;
      var lab=el.closest('label');
      if(lab){lab.classList.toggle('is-selected',el.checked)}
    });
  }
  function syncSelected(){
    document.querySelectorAll('.creator-palette,.creator-option,.creator-preset').forEach(function(l){
      var inp=l.querySelector('input');
      if(inp){l.classList.toggle('is-selected',inp.checked)}
    });
  }
  paletteInputs.forEach(function(el){el.addEventListener('change',syncSelected)});
  headerInputs.forEach(function(el){el.addEventListener('change',syncSelected)});
  footerInputs.forEach(function(el){el.addEventListener('change',syncSelected)});
  // Preset group filter
  var filterBtns=document.querySelectorAll('.creator-filter');
  filterBtns.forEach(function(btn){
    btn.addEventListener('click', function(){
      var f=btn.getAttribute('data-filter');
      filterBtns.forEach(function(b){ b.classList.toggle('button-ghost', b!==btn); b.classList.toggle('is-active', b===btn); });
      document.querySelectorAll('.creator-preset').forEach(function(l){
        var g=l.getAttribute('data-group');
        l.hidden = !(f==='*' || g===f);
      });
    });
  });

  presetInputs.forEach(function(el){
    el.addEventListener('change',function(){
      if(!el.checked) return;
      var d=defaults[el.value];
      if(!d) return;
      if(!touched.palette) setChecked('palette',d.palette);
      if(!touched.header) setChecked('header_style',d.header);
      if(!touched.footer) setChecked('footer_style',d.footer);
      syncSelected();
      filterPresentation();
      schedulePreview();
      applyFocusForStep(current);
    });
  });
  function filterPresentation(){
    var sel=form.querySelector('input[name="preset"]:checked');
    var preset=sel?sel.value:'blog';
    form.querySelectorAll('.creator-presentation .field-row').forEach(function(row){
      var p=row.getAttribute('data-preset');
      var active = p===preset;
      row.hidden = !active;
      row.querySelectorAll('select,input,textarea').forEach(function(el){
        el.disabled = !active;
      });
    });
  }
  function validateSiteURLValue(v){
    v=(v||"").trim();
    if(!v) return "";
    try{
      var u=new URL(v);
      if(u.protocol!=="http:" && u.protocol!=="https:") return "Site URL must start with http:// or https://";
      if(!u.host) return "Site URL must include a host";
      if(u.hash) return "Site URL must not contain a fragment";
      if(u.search) return "Site URL must not contain a query string";
      return "";
    }catch(e){ return "Enter a valid public URL, e.g. https://example.com"; }
  }
  function showSiteURLError(msg){
    var errEl=document.getElementById('site-url-error');
    var input=document.getElementById('creator-site-url');
    if(!errEl||!input) return;
    if(msg){
      errEl.textContent=msg;
      errEl.hidden=false;
      input.classList.add('is-invalid');
      input.setAttribute('aria-invalid','true');
    }else{
      errEl.textContent="";
      errEl.hidden=true;
      input.classList.remove('is-invalid');
      input.removeAttribute('aria-invalid');
    }
  }
  function isStepValid(n){
    var fs=steps[n-1];
    if(!fs) return false;
    var req=fs.querySelectorAll('[required]');
    for(var i=0;i<req.length;i++){
      if(req[i].type==='radio'){
        var grp=fs.querySelectorAll('input[name="'+req[i].name+'"]');
        var one=false;
        for(var j=0;j<grp.length;j++){if(grp[j].checked)one=true}
        if(!one) return false;
      } else if(!req[i].value) return false;
    }
    if(n===1){ var sel=form.querySelector('input[name="preset"]:checked'); if(!sel) return false; }
    if(n===5){
      var sn=form.querySelector('input[name="site_name"]');
      if(!sn||!sn.value.trim()) return false;
      // Site URL client validation only when non-empty; don't block empty allowed
      var su=form.querySelector('input[name="site_url"]');
      if(su && su.value.trim()){
        if(validateSiteURLValue(su.value)) return false;
      }
    }
    return true;
  }
  // ---- SessionStorage checkpoint ----
  var CHECKPOINT_KEY="stratum:creator:checkpoint";
  function saveCheckpoint(){
    try{
      var data = {
        currentStep: current,
        preset: (form.querySelector('input[name="preset"]:checked')||{}).value || "",
        palette: (form.querySelector('input[name="palette"]:checked')||{}).value || "",
        header: (form.querySelector('input[name="header_style"]:checked')||{}).value || "",
        footer: (form.querySelector('input[name="footer_style"]:checked')||{}).value || "",
        site_name: (form.querySelector('input[name="site_name"]')||{}).value || "",
        tagline: (form.querySelector('input[name="tagline"]')||{}).value || "",
        language: (form.querySelector('select[name="language"]')||{}).value || "",
        timezone: (form.querySelector('select[name="timezone"]')||{}).value || "",
        site_represents: (form.querySelector('select[name="site_represents"]')||{}).value || "",
        site_url: (form.querySelector('input[name="site_url"]')||{}).value || "",
        discourage: (form.querySelector('input[name="discourage_search_engines"]')||{}).checked ? "on" : "",
        blog_latest: (form.querySelector('select[name="blog_latest"]')||{}).value || "",
        blog_archive: (form.querySelector('select[name="blog_archive"]')||{}).value || "",
        portfolio_cols: (form.querySelector('select[name="portfolio_cols"]')||{}).value || "",
        product_cols: (form.querySelector('select[name="product_cols"]')||{}).value || "",
        product_media: (form.querySelector('select[name="product_media"]')||{}).value || "",
        testimonials_cols: (form.querySelector('select[name="testimonials_cols"]')||{}).value || "",
        service_cols: (form.querySelector('select[name="service_cols"]')||{}).value || ""
      };
      sessionStorage.setItem(CHECKPOINT_KEY, JSON.stringify(data));
    }catch(e){}
  }
  function clearCheckpoint(){ try{ sessionStorage.removeItem(CHECKPOINT_KEY); }catch(e){} }
  function restoreCheckpointIfNeeded(){
    // Server values have priority after validation error; only restore on fresh GET without server error
    if(hasValidationErr) return;
    try{
      var raw=sessionStorage.getItem(CHECKPOINT_KEY);
      if(!raw) return;
      var cp=JSON.parse(raw);
      if(!cp || typeof cp!=="object") return;
      // Don't restore if onboarding completed? caller already redirected. For safety check if form still there.
      if(cp.preset){ var el=form.querySelector('input[name="preset"][value="'+cp.preset+'"]'); if(el){ el.checked=true; } }
      if(cp.palette){ var pel=form.querySelector('input[name="palette"][value="'+cp.palette+'"]'); if(pel) pel.checked=true; }
      if(cp.header){ var hel=form.querySelector('input[name="header_style"][value="'+cp.header+'"]'); if(hel) hel.checked=true; }
      if(cp.footer){ var fel=form.querySelector('input[name="footer_style"][value="'+cp.footer+'"]'); if(fel) fel.checked=true; }
      var map={site_name: cp.site_name, tagline: cp.tagline, site_url: cp.site_url};
      Object.keys(map).forEach(function(k){
        var inp=form.querySelector('[name="'+k+'"]');
        if(inp && map[k]!==undefined && !inp.value) inp.value=map[k];
        else if(inp && map[k]!==undefined && k==="site_url" && inp.value.trim()==="" && map[k].trim()!=="") inp.value=map[k];
      });
      var selMap={language: cp.language, timezone: cp.timezone, site_represents: cp.site_represents, blog_latest: cp.blog_latest, blog_archive: cp.blog_archive, portfolio_cols: cp.portfolio_cols, product_cols: cp.product_cols, product_media: cp.product_media, testimonials_cols: cp.testimonials_cols, service_cols: cp.service_cols};
      Object.keys(selMap).forEach(function(k){
        var s=form.querySelector('[name="'+k+'"]');
        if(s && selMap[k]) s.value=selMap[k];
      });
      if(cp.discourage){ var d=form.querySelector('input[name="discourage_search_engines"]'); if(d) d.checked=true; }
      if(cp.currentStep && cp.currentStep>=1 && cp.currentStep<=5){
        current=cp.currentStep;
        for(var i=1;i<=current;i++) completed.add(i);
        if(wizardStepInput) wizardStepInput.value=String(current);
      }
      syncSelected();
      filterPresentation();
    }catch(e){}
  }
  // Detect clearing after successful create / skip — presence of .creator-complete indicates success
  if(document.querySelector('.creator-complete')){
    clearCheckpoint();
  } else {
    // restore before first showStep
    restoreCheckpointIfNeeded();
    // apply initialStep from checkpoint if it overrode current? Already set
    // But if server provided initialStep 5 and checkpoint says 2, server wins - we already handled hasValidationErr case.
  }
  // Auto suggestion for Site URL from window.location.origin
  (function autoSuggestSiteURL(){
    var input=document.getElementById('creator-site-url');
    var suggestRow=document.getElementById('site-url-suggest');
    var suggestVal=document.getElementById('site-url-suggest-val');
    var useBtn=document.getElementById('site-url-use-suggest');
    if(!input) return;
    var hasServerURL = studio && studio.getAttribute('data-has-server-site-url')==="1";
    var origin="";
    try{ origin=window.location.origin; }catch(e){}
    if(!origin) return;
    // If input empty and no server URL, auto-fill
    if(!input.value.trim() && !hasServerURL){
      // Only auto-fill if not restored from checkpoint with value
      input.value=origin;
      // show subtle hint? optional
      // Don't mark as error
      saveCheckpoint();
      if(suggestRow && suggestVal){
        // no need to show suggestion when auto-filled; keep hidden
      }
    } else if(!input.value.trim() && hasServerURL){
      // has server but empty? shouldn't happen; still allow suggestion button
      if(suggestRow && suggestVal){
        suggestVal.textContent=origin;
        suggestRow.hidden=false;
      }
    } else if(input.value.trim() && suggestRow){
      // keep hidden when already has value
      suggestRow.hidden=true;
    }
    if(useBtn){
      useBtn.addEventListener('click', function(){
        var inp=document.getElementById('creator-site-url');
        if(inp){ inp.value=origin; showSiteURLError(""); schedulePreview(); saveCheckpoint(); suggestRow.hidden=true; inp.focus(); }
      });
    }
    // When user types, hide suggestion if matches origin? just keep logic
    if(input){
      input.addEventListener('input', function(){
        if(suggestRow){
          if(!input.value.trim() && !hasServerURL){
            // keep filled? auto-fill again? no
          }
        }
        // clear error on typing
        if(!validateSiteURLValue(input.value)) showSiteURLError("");
      });
    }
  })();
  // previewFocus state
  var previewFocus="page";
  function applyFocusForStep(step){
    var surfaceSel=document.getElementById('preview-surface');
    if(!surfaceSel) return;
    if(step===3){
      surfaceSel.value='home';
      lastSurface='home';
      previewFocus="page";
      schedulePreview(function(){
        try{ var ifr=document.getElementById('creator-preview'); if(ifr&&ifr.contentWindow) ifr.contentWindow.scrollTo(0,0);}catch(e){}
      });
    } else if(step===4){
      surfaceSel.value='home';
      lastSurface='home';
      previewFocus="footer";
      schedulePreview();
    } else if(step===5){
      previewFocus="page";
      var presetEl=form.querySelector('input[name="preset"]:checked');
      var preset=presetEl?presetEl.value:'blog';
      var activeDetail=document.activeElement;
      var name=activeDetail?activeDetail.getAttribute('name'):'';
      if(name==='product_cols' || name==='service_cols'){
        surfaceSel.value='archive'; lastSurface='archive';
      } else if(name==='product_media'){
        surfaceSel.value='single'; lastSurface='single';
      } else if(name==='blog_latest' || name==='blog_archive'){
        if(preset==='blog' || preset==='magazine') { surfaceSel.value='archive'; lastSurface='archive'; }
        else { surfaceSel.value='home'; lastSurface='home'; }
      } else if(preset==='knowledge-base' || preset==='simple-site'){
        surfaceSel.value='home'; lastSurface='home';
      } else {
        surfaceSel.value='home'; lastSurface='home';
      }
      schedulePreview();
    } else {
      surfaceSel.value='home'; lastSurface='home';
      previewFocus="page";
      schedulePreview();
    }
  }
  function showStep(n){
    if(n<1||n>5) return;
    if(n>current){
      for(var k=current;k<n;k++){ if(!isStepValid(k)) return; }
    }
    current=n;
    if(wizardStepInput) wizardStepInput.value=String(current);
    steps.forEach(function(fs,i){
      var idx=i+1;
      var active=idx===current;
      if(fs){ fs.hidden=!active; fs.classList.toggle('is-active-step',active); }
    });
    stepper.forEach(function(li){
      var s=parseInt(li.getAttribute('data-step'),10);
      li.classList.toggle('is-active',s===current);
      li.classList.toggle('is-complete',completed.has(s)&&s!==current);
      var btn=li.querySelector('button');
      if(btn) btn.disabled = s>current && !completed.has(s-1);
    });
    if(backBtn) backBtn.disabled=current===1;
    if(current===5){
      if(contBtn) contBtn.hidden=true;
      if(submitBtn) submitBtn.hidden=false;
    } else {
      if(contBtn) { contBtn.hidden=false; contBtn.disabled=!isStepValid(current); }
      if(submitBtn) submitBtn.hidden=true;
    }
    filterPresentation();
    applyFocusForStep(current);
    saveCheckpoint();
  }
  if(backBtn) backBtn.addEventListener('click',function(){ if(current>1) showStep(current-1); });
  if(contBtn) contBtn.addEventListener('click',function(){
    if(!isStepValid(current)) return;
    // client URL validation before leaving step 5? only on continue, not relevant since step5 is final
    if(current===5){
      var su=form.querySelector('input[name="site_url"]');
      if(su){
        var msg=validateSiteURLValue(su.value);
        if(msg){ showSiteURLError(msg); su.focus(); return; } else showSiteURLError("");
      }
    }
    completed.add(current+1);
    showStep(current+1);
  });
  stepper.forEach(function(li){
    var btn=li.querySelector('button');
    if(!btn) return;
    btn.addEventListener('click',function(){
      var target=parseInt(btn.getAttribute('data-goto'),10);
      if(completed.has(target)||target<current) showStep(target);
    });
  });
  // Preview
  var iframe=document.getElementById('creator-preview');
  var frame=document.getElementById('preview-frame');
  var surfaceSel=document.getElementById('preview-surface');
  var vpBtns=document.querySelectorAll('[data-viewport]');
  var debounceTimer=null;
  var lastSurface='home';
  if(vpBtns) vpBtns.forEach(function(b){
    b.addEventListener('click',function(){
      vpBtns.forEach(function(x){x.classList.remove('is-active')});
      b.classList.add('is-active');
      if(b.getAttribute('data-viewport')==='mobile') frame && frame.classList.add('is-mobile');
      else frame && frame.classList.remove('is-mobile');
    });
  });
  if(surfaceSel) surfaceSel.addEventListener('change',function(){ lastSurface=surfaceSel.value; if(current!==4) previewFocus="page"; schedulePreview(); });
  function collectFormData(){
    var fd=new FormData(form);
    fd.set('surface', lastSurface);
    return fd;
  }
  function schedulePreview(cb){
    clearTimeout(debounceTimer);
    debounceTimer=setTimeout(function(){ doPreview(cb); }, 240);
  }
  function doPreviewImmediate(){ clearTimeout(debounceTimer); doPreview(); }
  function doPreview(cb){
    var fd=collectFormData();
    var params=new URLSearchParams();
    fd.forEach(function(v,k){ params.append(k,v); });
    // Include wizard_step for preview consistency
    if(wizardStepInput && wizardStepInput.value) params.set('wizard_step', wizardStepInput.value);
    fetch('/admin/creator/preview?'+params.toString(),{method:'GET',credentials:'same-origin'})
      .then(function(res){ if(!res.ok) throw new Error('preview '+res.status); return res.text(); })
      .then(function(html){
        if(iframe) {
          // Hide until scrolled to avoid flash from top
          iframe.style.visibility="hidden";
          iframe.onload=function(){
            try{
              if(previewFocus==="footer"){
                var doc=iframe.contentDocument;
                if(doc){
                  var footer=doc.querySelector('footer, .site-footer, [role="contentinfo"]');
                  if(footer){
                    // Instant, no smooth
                    var top = footer.getBoundingClientRect().top + (doc.documentElement.scrollTop || doc.body.scrollTop);
                    // Use instant scrollTo
                    if(iframe.contentWindow) iframe.contentWindow.scrollTo(0, top);
                  } else {
                    if(iframe.contentWindow) iframe.contentWindow.scrollTo(0, doc.body.scrollHeight);
                  }
                }
              } else {
                // page focus: ensure top when needed (step 3)
                if(current===3){
                  if(iframe.contentWindow) iframe.contentWindow.scrollTo(0,0);
                }
              }
            }catch(e){}
            iframe.style.visibility="visible";
            if(cb) cb();
          };
          iframe.srcdoc=html;
          // Fallback visibility in case onload not fired (cached)
          setTimeout(function(){ if(iframe.style.visibility==="hidden") iframe.style.visibility="visible"; }, 600);
        } else if(cb) cb();
      })
      .catch(function(){ if(iframe) iframe.style.visibility="visible"; if(cb) cb(); });
  }
  // Form submit validation for Site URL
  form.addEventListener('submit', function(e){
    var su=form.querySelector('input[name="site_url"]');
    if(su){
      var msg=validateSiteURLValue(su.value);
      if(msg){
        e.preventDefault();
        showSiteURLError(msg);
        // Ensure we stay on step 5
        if(current!==5) showStep(5);
        su.focus();
        return false;
      } else {
        showSiteURLError("");
      }
    }
    // Before submit, ensure wizard_step is current
    if(wizardStepInput) wizardStepInput.value=String(current);
    // Clear checkpoint on successful submit attempt? Actually clear only after success page; keep until response.
    // Don't clear yet — server will return 422 with preserved values or success page which clears via .creator-complete detection.
  });
  form.addEventListener('change',function(e){
    var t=e.target;
    if(t.matches('input[type="radio"], select, input[type="checkbox"]')){ doPreviewImmediate(); syncSelected(); if(t.name==='preset') filterPresentation(); }
    if(t.name==='preset'||t.name==='language') filterPresentation();
    if(contBtn) contBtn.disabled=!isStepValid(current);
    saveCheckpoint();
  });
  var typingInputs=form.querySelectorAll('input[name="site_name"], input[name="tagline"], input[name="site_url"]');
  typingInputs.forEach(function(inp){
    inp.addEventListener('input',function(){
      // site_url error live
      if(inp.name==="site_url"){
        var msg=validateSiteURLValue(inp.value);
        if(!msg) showSiteURLError("");
      }
      schedulePreview(); if(contBtn) contBtn.disabled=!isStepValid(current);
      saveCheckpoint();
    });
  });
  // also save on select changes
  form.querySelectorAll('select').forEach(function(s){ s.addEventListener('change', saveCheckpoint); });
  try{
    var tz=Intl.DateTimeFormat().resolvedOptions().timeZone;
    var sel=form.querySelector('select[name="timezone"]');
    if(sel && tz){
      var opts=Array.from(sel.options).map(function(o){return o.value});
      if(opts.indexOf(tz)!==-1 && !sel.value) sel.value=tz;
    }
  }catch(e){}
  // Mobile tabs
  var mobileTabs=document.querySelectorAll('[data-mobile-tab]');
  if(mobileTabs) mobileTabs.forEach(function(btn){
    btn.addEventListener('click', function(){
      var tab=btn.getAttribute('data-mobile-tab');
      mobileTabs.forEach(function(x){x.classList.remove('is-active')});
      btn.classList.add('is-active');
      if(studio){
        if(tab==='preview') studio.classList.add('has-preview-mobile');
        else studio.classList.remove('has-preview-mobile');
      }
    });
  });
  // Step5 focus mapping: when controls change, update preview surface
  document.addEventListener('focusin', function(e){
    if(current!==5) return;
    var n=e.target.getAttribute('name');
    if(!n) return;
    if(n==='product_cols' || n==='service_cols'){
      if(surfaceSel){ surfaceSel.value='archive'; lastSurface='archive'; previewFocus="page"; schedulePreview(); }
    } else if(n==='product_media'){
      if(surfaceSel){ surfaceSel.value='single'; lastSurface='single'; previewFocus="page"; schedulePreview(); }
    } else if(n==='blog_latest' || n==='blog_archive'){
      if(surfaceSel){ surfaceSel.value='archive'; lastSurface='archive'; previewFocus="page"; schedulePreview(); }
    }
  });
  filterPresentation();
  // Ensure only active preset controls are successful on first load
  (function(){ var sel=form.querySelector('input[name="preset"]:checked'); if(sel) filterPresentation(); })();
  // Show correct initial step (server-provided or checkpoint-restored)
  showStep(current);
  syncSelected();
  doPreviewImmediate();
  form.querySelectorAll('input,select,textarea').forEach(function(el){
    el.addEventListener('change',function(){ if(contBtn) contBtn.disabled=!isStepValid(current); });
  });
  // Clear checkpoint on Skip
  var skipForm=document.querySelector('.creator-topbar__skip');
  if(skipForm) skipForm.addEventListener('submit', function(){ clearCheckpoint(); });
})();
