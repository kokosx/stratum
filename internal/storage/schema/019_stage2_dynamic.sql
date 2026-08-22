-- Stage 2 dynamic blocks: Featured Image, Site Logo, Social Links, Breadcrumbs.
-- Also extends site_settings with the branding columns these blocks read.

ALTER TABLE site_settings ADD COLUMN site_logo_media_id TEXT;
ALTER TABLE site_settings ADD COLUMN social_links TEXT;

-- ============================================================
-- Featured Image: the current entry's featured media
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-featured-image-v1', 'core', 'featured-image', 1, 'Featured Image', 'The featured image of the current entry.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"aspectRatio":{"type":"string","enum":["auto","1:1","4:3","16:9","3:2"],"default":"16:9"},"objectFit":{"type":"string","enum":["cover","contain"],"default":"cover"},"align":{"type":"string","enum":["left","center","right","full"],"default":"full"}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"image","fields":{"settings.aspectRatio":{"label":"Aspect ratio","control":"select","group":"Style"},"settings.objectFit":{"label":"Object fit","control":"segmented","group":"Style"},"settings.align":{"label":"Alignment","control":"segmented","group":"Layout"}}}}',
    'template',
    '{{ $m := media .Context.Entry.FeaturedImage }}{{ if $m.Src }}<figure class="stratum-featured-image stratum-featured-image-fit-{{ .Settings.objectFit }} stratum-featured-image-ar-{{ .Settings.aspectRatio }} stratum-featured-image-align-{{ .Settings.align }}"><img src="{{ $m.Src }}"{{ if $m.SrcSet }} srcset="{{ $m.SrcSet }}" sizes="(min-width:992px) 768px,(min-width:640px) 480px,100vw"{{ end }}{{ if $m.Width }} width="{{ $m.Width }}"{{ end }}{{ if $m.Height }} height="{{ $m.Height }}"{{ end }} alt="{{ $m.Alt }}" loading="lazy" decoding="async"></figure>{{ else }}<div class="stratum-featured-image stratum-featured-image-missing">Featured image</div>{{ end }}',
    '.stratum-featured-image{margin:0}.stratum-featured-image img{display:block;max-width:100%;height:auto}.stratum-featured-image-fit-cover img{object-fit:cover;width:100%;height:100%}.stratum-featured-image-fit-contain img{object-fit:contain}.stratum-featured-image-ar-1-1{aspect-ratio:1/1}.stratum-featured-image-ar-4-3{aspect-ratio:4/3}.stratum-featured-image-ar-16-9{aspect-ratio:16/9}.stratum-featured-image-ar-3-2{aspect-ratio:3/2}.stratum-featured-image-ar-1-1 img,.stratum-featured-image-ar-4-3 img,.stratum-featured-image-ar-16-9 img,.stratum-featured-image-ar-3-2 img{width:100%;height:100%}.stratum-featured-image-align-center{text-align:center}.stratum-featured-image-align-center img{margin-inline:auto}.stratum-featured-image-missing{padding:var(--st-space-lg);border:1px dashed var(--st-color-border);text-align:center;color:var(--st-color-text-muted)}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Site Logo: the site logo from Site Settings
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-logo-v1', 'core', 'logo', 1, 'Site Logo', 'The site logo from Site Settings.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"size":{"type":"string","enum":["sm","md","lg"],"default":"md"},"link":{"type":"boolean","default":true}}},"children":{"mode":"none"},"editor":{"category":"branding","icon":"site","fields":{"settings.size":{"label":"Size","control":"segmented","group":"Style"},"settings.link":{"label":"Link to home","control":"checkbox","group":"Style"}}}}',
    'template',
    '{{ if .Context.Site.LogoURL }}{{ if .Settings.link }}<a class="stratum-logo" href="{{ if .Context.Site.URL }}{{ .Context.Site.URL }}{{ else }}/{{ end }}">{{ end }}<img class="stratum-logo-img stratum-logo-size-{{ .Settings.size }}" src="{{ .Context.Site.LogoURL }}" alt="{{ .Context.Site.Name }}">{{ if .Settings.link }}</a>{{ end }}{{ else if .Context.Site.Name }}{{ if .Settings.link }}<a class="stratum-logo stratum-logo-text" href="{{ if .Context.Site.URL }}{{ .Context.Site.URL }}{{ else }}/{{ end }}">{{ end }}{{ .Context.Site.Name }}{{ if .Settings.link }}</a>{{ end }}{{ else }}<span class="stratum-placeholder">Site logo</span>{{ end }}',
    '.stratum-logo{display:inline-flex;align-items:center;text-decoration:none;color:inherit;font-weight:700}.stratum-logo-img{display:block;max-width:100%}.stratum-logo-size-sm .stratum-logo-img,.stratum-logo-img.stratum-logo-size-sm{height:1.75rem}.stratum-logo-size-md .stratum-logo-img,.stratum-logo-img.stratum-logo-size-md{height:2.25rem}.stratum-logo-size-lg .stratum-logo-img,.stratum-logo-img.stratum-logo-size-lg{height:3rem}.stratum-logo-text{font-size:1.25rem}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Social Links: configured site social profiles
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-social-links-v1', 'core', 'social-links', 1, 'Social Links', 'Social profile links configured in Site Settings.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"align":{"type":"string","enum":["start","center","end"],"default":"start"}}},"children":{"mode":"none"},"editor":{"category":"branding","icon":"social","fields":{"settings.align":{"label":"Alignment","control":"segmented","group":"Layout"}}}}',
    'template',
    '{{ if .Context.Site.SocialLinks }}<ul class="stratum-social stratum-social-align-{{ .Settings.align }}">{{ range .Context.Site.SocialLinks }}<li class="stratum-social-item"><a class="stratum-social-link" href="{{ .URL }}" target="_blank" rel="noopener noreferrer">{{ if .Label }}{{ .Label }}{{ else }}{{ .Platform }}{{ end }}</a></li>{{ end }}</ul>{{ else }}<span class="stratum-placeholder">Social links</span>{{ end }}',
    '.stratum-social{display:flex;flex-wrap:wrap;gap:var(--st-space-md);list-style:none;margin:0;padding:0}.stratum-social-align-center{justify-content:center}.stratum-social-align-end{justify-content:flex-end}.stratum-social-link{display:inline-flex;align-items:center;gap:.35rem;color:var(--st-color-primary);text-decoration:none}.stratum-social-link:hover{text-decoration:underline}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Breadcrumbs: system-generated from the current entry
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-breadcrumbs-v1', 'core', 'breadcrumbs', 1, 'Breadcrumbs', 'A breadcrumb trail for the current entry.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"showHome":{"type":"boolean","default":true}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"breadcrumbs","fields":{"settings.showHome":{"label":"Show home","control":"checkbox","group":"Content"}}}}',
    'template',
    '{{ $home := "/" }}{{ if .Context.Site.URL }}{{ $home = .Context.Site.URL }}{{ end }}<nav class="stratum-breadcrumbs" aria-label="Breadcrumb"><ol class="stratum-breadcrumbs-list">{{ if .Settings.showHome }}<li class="stratum-breadcrumbs-item"><a href="{{ $home }}">Home</a></li>{{ end }}{{ if .Context.Entry.Title }}<li class="stratum-breadcrumbs-item stratum-breadcrumbs-current" aria-current="page">{{ .Context.Entry.Title }}</li>{{ end }}</ol></nav>',
    '.stratum-breadcrumbs{margin:0}.stratum-breadcrumbs-list{display:flex;flex-wrap:wrap;align-items:center;gap:.5rem;list-style:none;margin:0;padding:0;font-size:var(--st-small-size,.875rem);color:var(--st-color-text-muted)}.stratum-breadcrumbs-item:not(:first-child)::before{content:"/";margin-right:.5rem;color:var(--st-color-text-muted)}.stratum-breadcrumbs-item a{color:inherit;text-decoration:none}.stratum-breadcrumbs-item a:hover{text-decoration:underline}.stratum-breadcrumbs-current{color:var(--st-color-text)}',
    'core', 1, unixepoch(), unixepoch()
);
