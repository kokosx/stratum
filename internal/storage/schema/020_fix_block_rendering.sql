-- ===== 0002_fix_block_rendering.sql =====
-- Repair-pass fixes to core blocks so the public frontend never emits broken
-- controls and images render and load correctly. Applied once for both fresh
-- installs and existing databases.

-- Button: never render an empty clickable rectangle, and never emit a live link
-- to "#". A label is required for any output; a missing/placeholder URL renders
-- a non-interactive styled element instead of a broken anchor.
UPDATE block_definitions SET
    schema_json = '{"schemaVersion":1,"props":{"type":"object","required":["label","url"],"properties":{"label":{"type":"string","default":""},"url":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"variant":{"type":"string","enum":["primary","secondary","outline","ghost"],"default":"primary"},"size":{"type":"string","enum":["sm","md","lg"],"default":"md"},"width":{"type":"string","enum":["auto","full"],"default":"auto"},"align":{"type":"string","enum":["left","center","right"],"default":"left"},"openInNewTab":{"type":"boolean","default":false}}},"children":{"mode":"none"},"editor":{"category":"design","icon":"button","fields":{"props.label":{"label":"Label","control":"text"},"props.url":{"label":"URL","control":"text"},"settings.variant":{"label":"Variant","control":"segmented","group":"Style"},"settings.size":{"label":"Size","control":"segmented","group":"Style"},"settings.width":{"label":"Width","control":"segmented","group":"Layout"},"settings.align":{"label":"Alignment","control":"segmented","group":"Layout"},"settings.openInNewTab":{"label":"Open in new tab","control":"checkbox","group":"Link"}}}}',
    template = '{{ if .Props.label }}<div class="stratum-btn-wrap stratum-align-{{ .Settings.align }}{{ if eq .Settings.width "full" }} stratum-btn-wrap-full{{ end }}">{{ $url := .Props.url }}{{ if and $url (ne $url "#") }}<a class="stratum-button stratum-button-{{ .Settings.variant }} stratum-button-size-{{ .Settings.size }}{{ if eq .Settings.width "full" }} stratum-button-full{{ end }}" href="{{ $url }}"{{ if .Settings.openInNewTab }} target="_blank" rel="noopener noreferrer"{{ end }}>{{ .Props.label }}</a>{{ else }}<span class="stratum-button stratum-button-{{ .Settings.variant }} stratum-button-size-{{ .Settings.size }}{{ if eq .Settings.width "full" }} stratum-button-full{{ end }}" aria-disabled="true">{{ .Props.label }}</span>{{ end }}</div>{{ end }}',
    styles = '',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'button' AND version = 1;

-- Image: correct loading strategy, only emit fetchpriority when eager, center a
-- block-level image via margin, and drop dead CSS that the schema never emits.
-- A missing media reference becomes a neutral, clearly-labelled placeholder
-- rather than a broken-looking content block.
UPDATE block_definitions SET
    template = '{{ $m := media .Props.mediaId }}{{ if $m.Src }}<figure class="stratum-image stratum-image-align-{{ .Settings.align }}"><img src="{{ $m.Src }}"{{ if $m.SrcSet }} srcset="{{ $m.SrcSet }}"{{ if .Settings.sizes }} sizes="{{ .Settings.sizes }}"{{ end }}{{ end }}{{ if $m.Width }} width="{{ $m.Width }}"{{ end }}{{ if $m.Height }} height="{{ $m.Height }}"{{ end }} alt="{{ if not .Settings.decorative }}{{ if .Props.alt }}{{ .Props.alt }}{{ else }}{{ $m.Alt }}{{ end }}{{ end }}"{{ if .Settings.eager }} loading="eager" fetchpriority="high"{{ else }} loading="lazy" decoding="async"{{ end }}>{{ if .Props.caption }}<figcaption class="stratum-image-caption">{{ .Props.caption }}</figcaption>{{ end }}</figure>{{ else }}<div class="stratum-image stratum-image-missing" role="img" aria-label="Missing image">{{ if .Settings.decorative }}Decorative image missing{{ else }}{{ if .Props.alt }}{{ .Props.alt }}{{ else }}Image unavailable{{ end }}{{ end }}</div>{{ end }}',
    styles = '.stratum-image{margin:0}.stratum-image img{display:block;max-width:100%;height:auto}.stratum-image-align-left img{margin-inline:auto 0}.stratum-image-align-center{text-align:center}.stratum-image-align-center img{margin-inline:auto}.stratum-image-caption{margin-top:var(--st-space-xs);font-size:var(--st-small-size,.875rem);color:var(--st-color-text-muted)}.stratum-image-missing{padding:var(--st-space-lg);border:1px dashed var(--st-color-border);text-align:center;color:var(--st-color-text-muted);border-radius:var(--st-radius-md)}',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'image' AND version = 1;
