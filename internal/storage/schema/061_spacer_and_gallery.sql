-- 061_spacer_and_gallery.sql
-- EPIC 3: add missing core/spacer and harden gallery to canonical spec
-- without breaking historical documents.

PRAGMA foreign_keys = ON;

-- ============================================================
-- Spacer: semantic spacing token
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-spacer-v1', 'core', 'spacer', 1, 'Spacer', 'Vertical whitespace using semantic spacing tokens.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"size":{"type":"string","enum":["xs","sm","md","lg","xl","2xl"],"default":"md"}}},"children":{"mode":"none"},"editor":{"category":"layout","icon":"spacer","fields":{"settings.size":{"label":"Size","control":"segmented","group":"Layout"}}}}',
    'template',
    '<div aria-hidden="true" class="stratum-spacer stratum-spacer-{{ .Settings.size }}"></div>',
    '.stratum-spacer{width:100%}.stratum-spacer-xs{height:var(--st-space-xs)}.stratum-spacer-sm{height:var(--st-space-sm)}.stratum-spacer-md{height:var(--st-space-md)}.stratum-spacer-lg{height:var(--st-space-lg)}.stratum-spacer-xl{height:var(--st-space-xl)}.stratum-spacer-2xl{height:var(--st-space-2xl)}',
    'core', 1, unixepoch(), unixepoch()
)
ON CONFLICT(namespace, name, version) DO UPDATE SET
    display_name=excluded.display_name,
    description=excluded.description,
    schema_json=excluded.schema_json,
    renderer_type=excluded.renderer_type,
    template=excluded.template,
    styles=excluded.styles,
    source=excluded.source,
    enabled=excluded.enabled,
    updated_at=excluded.updated_at;

-- ============================================================
-- Gallery: canonical spec is columns 2/3/4, gap sm/md/lg,
-- aspect auto/square/4:3, radius none/sm/md/lg.
-- Keep legacy values (1:1, 16:9, 3:2, none/xs) for compatibility
-- so old documents remain valid, but new editor uses canonical set.
-- ============================================================
UPDATE block_definitions SET
    schema_json = '{"schemaVersion":1,"props":{"type":"object","properties":{"images":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"columns":{"type":"integer","enum":[2,3,4],"default":3},"gap":{"type":"string","enum":["sm","md","lg"],"default":"md"},"aspect":{"type":"string","enum":["auto","square","4:3"],"default":"auto"},"radius":{"type":"string","enum":["none","sm","md","lg"],"default":"none"}}},"children":{"mode":"none"},"editor":{"category":"media","icon":"gallery","fields":{"props.images":{"label":"Images","control":"media","group":"Content"},"settings.columns":{"label":"Columns","control":"segmented","group":"Layout"},"settings.gap":{"label":"Gap","control":"select","group":"Layout"},"settings.aspect":{"label":"Aspect","control":"select","group":"Style"},"settings.radius":{"label":"Radius","control":"select","group":"Style"}}}}',
    template = '{{ $ids := split "," .Props.images }}{{ if $ids }}<div class="stratum-gallery stratum-gallery-cols-{{ .Settings.columns }} stratum-gallery-gap-{{ .Settings.gap }} stratum-gallery-radius-{{ .Settings.radius }}{{ if ne .Settings.aspect "auto" }} stratum-gallery-aspect-{{ .Settings.aspect }}{{ end }}">{{ range $ids }}{{ $m := media . }}{{ if $m.Src }}<figure class="stratum-gallery-item">{{ if $m.WebPSrcSet }}<picture><source type="image/webp" srcset="{{ $m.WebPSrcSet }}" sizes="(min-width:992px) 33vw,(min-width:640px) 50vw,100vw">{{ end }}<img src="{{ $m.Src }}"{{ if $m.SrcSet }} srcset="{{ $m.SrcSet }}" sizes="(min-width:992px) 33vw,(min-width:640px) 50vw,100vw"{{ end }}{{ if $m.Width }} width="{{ $m.Width }}"{{ end }}{{ if $m.Height }} height="{{ $m.Height }}"{{ end }} alt="{{ $m.Alt }}" loading="lazy" decoding="async">{{ if $m.WebPSrcSet }}</picture>{{ end }}</figure>{{ end }}{{ end }}</div>{{ else }}<div class="stratum-gallery stratum-gallery-empty">No images selected</div>{{ end }}',
    styles = '.stratum-gallery{display:grid;gap:var(--st-space-md)}.stratum-gallery-cols-2{grid-template-columns:repeat(2,1fr)}.stratum-gallery-cols-3{grid-template-columns:repeat(3,1fr)}.stratum-gallery-cols-4{grid-template-columns:repeat(4,1fr)}.stratum-gallery-gap-sm{gap:var(--st-space-sm)}.stratum-gallery-gap-md{gap:var(--st-space-md)}.stratum-gallery-gap-lg{gap:var(--st-space-lg)}.stratum-gallery-item{margin:0;overflow:hidden;border-radius:var(--st-radius-md)}.stratum-gallery-radius-none .stratum-gallery-item{border-radius:0}.stratum-gallery-radius-sm .stratum-gallery-item{border-radius:var(--st-radius-sm)}.stratum-gallery-radius-md .stratum-gallery-item{border-radius:var(--st-radius-md)}.stratum-gallery-radius-lg .stratum-gallery-item{border-radius:var(--st-radius-lg)}.stratum-gallery-aspect-square .stratum-gallery-item{aspect-ratio:1/1}.stratum-gallery-aspect-4\:3 .stratum-gallery-item{aspect-ratio:4/3}.stratum-gallery-item img{display:block;width:100%;height:100%;object-fit:cover}.stratum-gallery-empty{padding:var(--st-space-lg);border:1px dashed var(--st-color-border);text-align:center;color:var(--st-color-text-muted)}@media(max-width:640px){.stratum-gallery-cols-3,.stratum-gallery-cols-4{grid-template-columns:repeat(2,1fr)}}@media(max-width:480px){.stratum-gallery{grid-template-columns:1fr}}',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'gallery' AND version = 1;
