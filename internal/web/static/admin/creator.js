// Creator studio client — no framework, preview uses real theme via /admin/creator/preview
(function(){
  var form = document.getElementById('creator-form');
  if(!form) return;
  document.documentElement.classList.add('has-js');
  var steps=[1,2,3,4,5].map(function(n){return form.querySelector('[data-step="'+n+'"]')});
  var backBtn=document.getElementById('creator-back');
  var contBtn=document.getElementById('creator-continue');
  var submitBtn=document.getElementById('creator-submit');
  var stepper=document.querySelectorAll('.creator-stepper li');
  var presetInputs=form.querySelectorAll('input[name="preset"]');
  var paletteInputs=form.querySelectorAll('input[name="palette"]');
  var headerInputs=form.querySelectorAll('input[name="header_style"]');
  var footerInputs=form.querySelectorAll('input[name="footer_style"]');
  var current=1;
  var completed=new Set([1]);
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
    }
    return true;
  }
  function applyFocusForStep(step){
    // transient focus, UI state only
    var surfaceSel=document.getElementById('preview-surface');
    if(!surfaceSel) return;
    if(step===3){
      surfaceSel.value='home';
      lastSurface='home';
      schedulePreview(function(){
        try{ var ifr=document.getElementById('creator-preview'); if(ifr&&ifr.contentWindow) ifr.contentWindow.scrollTo(0,0);}catch(e){}
      });
    } else if(step===4){
      surfaceSel.value='home';
      lastSurface='home';
      schedulePreview(function(){
        scrollPreviewToFooter();
      });
    } else if(step===5){
      // Determine focus from edited setting or preset
      var presetEl=form.querySelector('input[name="preset"]:checked');
      var preset=presetEl?presetEl.value:'blog';
      var activeDetail=document.activeElement;
      var name=activeDetail?activeDetail.getAttribute('name'):'';
      if(name==='product_cols' || name==='service_cols'){
        surfaceSel.value='archive'; lastSurface='archive';
      } else if(name==='product_media'){
        surfaceSel.value='single'; lastSurface='single';
      } else if(name==='blog_latest' || name==='blog_archive'){
        // blog count -> archive
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
      schedulePreview();
    }
  }
  function scrollPreviewToFooter(){
    var ifr=document.getElementById('creator-preview');
    if(!ifr||!ifr.contentWindow) return;
    try{
      var doc=ifr.contentWindow.document;
      var footer=doc.querySelector('footer, .site-footer, [role="contentinfo"]');
      if(footer) footer.scrollIntoView({behavior:'smooth',block:'start'});
      else ifr.contentWindow.scrollTo(0, ifr.contentWindow.document.body.scrollHeight);
    }catch(e){}
  }
  function showStep(n){
    if(n<1||n>5) return;
    if(n>current){
      for(var k=current;k<n;k++){ if(!isStepValid(k)) return; }
    }
    current=n;
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
  }
  if(backBtn) backBtn.addEventListener('click',function(){ if(current>1) showStep(current-1); });
  if(contBtn) contBtn.addEventListener('click',function(){
    if(!isStepValid(current)) return;
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
  if(surfaceSel) surfaceSel.addEventListener('change',function(){ lastSurface=surfaceSel.value; schedulePreview(); });
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
    fetch('/admin/creator/preview?'+params.toString(),{method:'GET',credentials:'same-origin'})
      .then(function(res){ if(!res.ok) throw new Error('preview '+res.status); return res.text(); })
      .then(function(html){
        if(iframe) {
          iframe.onload=function(){
            if(cb) cb();
            // Auto focus footer when step 4 (after every reload)
            if(current===4) setTimeout(scrollPreviewToFooter, 80);
            // Step3 already scrolled to top via cb
          };
          iframe.srcdoc=html;
        } else if(cb) cb();
      })
      .catch(function(){ if(cb) cb(); });
  }
  form.addEventListener('change',function(e){
    var t=e.target;
    if(t.matches('input[type="radio"], select, input[type="checkbox"]')){ doPreviewImmediate(); syncSelected(); if(t.name==='preset') filterPresentation(); }
    if(t.name==='preset'||t.name==='language') filterPresentation();
    if(contBtn) contBtn.disabled=!isStepValid(current);
  });
  var typingInputs=form.querySelectorAll('input[name="site_name"], input[name="tagline"], input[name="site_url"]');
  typingInputs.forEach(function(inp){
    inp.addEventListener('input',function(){ schedulePreview(); if(contBtn) contBtn.disabled=!isStepValid(current); });
  });
  try{
    var tz=Intl.DateTimeFormat().resolvedOptions().timeZone;
    var sel=form.querySelector('select[name="timezone"]');
    if(sel && tz){
      var opts=Array.from(sel.options).map(function(o){return o.value});
      if(opts.indexOf(tz)!==-1 && !sel.value) sel.value=tz;
    }
  }catch(e){}
  // Mobile tabs
  var studio=document.querySelector('[data-creator-studio]');
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
      if(surfaceSel){ surfaceSel.value='archive'; lastSurface='archive'; schedulePreview(); }
    } else if(n==='product_media'){
      if(surfaceSel){ surfaceSel.value='single'; lastSurface='single'; schedulePreview(); }
    } else if(n==='blog_latest' || n==='blog_archive'){
      if(surfaceSel){ surfaceSel.value='archive'; lastSurface='archive'; schedulePreview(); }
    }
  });
  filterPresentation();
  // Ensure only active preset controls are successful on first load
  (function(){ var sel=form.querySelector('input[name="preset"]:checked'); if(sel) filterPresentation(); })();
  showStep(1);
  syncSelected();
  doPreviewImmediate();
  form.querySelectorAll('input,select,textarea').forEach(function(el){
    el.addEventListener('change',function(){ if(contBtn) contBtn.disabled=!isStepValid(current); });
  });
})();
