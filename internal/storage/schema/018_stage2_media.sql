-- Stage 2 media + structured blocks: Table, Gallery, Video.

-- ============================================================
-- Table: responsive structured content
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-table-v1', 'core', 'table', 1, 'Table', 'A responsive table with optional header and striped rows.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{"header":{"type":"string","default":""},"body":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"caption":{"type":"string","default":""},"striped":{"type":"boolean","default":false},"variant":{"type":"string","enum":["bordered","plain"],"default":"bordered"}}},"children":{"mode":"none"},"editor":{"category":"content","icon":"table","fields":{"props.header":{"label":"Header row (cells separated by |)","control":"text","group":"Content"},"props.body":{"label":"Rows (one per line, cells separated by |)","control":"textarea","group":"Content"},"settings.caption":{"label":"Caption","control":"text","group":"Content"},"settings.striped":{"label":"Striped","control":"checkbox","group":"Style"},"settings.variant":{"label":"Variant","control":"segmented","group":"Style"}}}}',
    'template',
    '<figure class="stratum-table-fig stratum-table-variant-{{ .Settings.variant }}">{{ if .Props.caption }}<figcaption class="stratum-table-caption">{{ .Props.caption }}</figcaption>{{ end }}<div class="stratum-table-scroll"><table class="stratum-table">{{ if .Props.header }}<thead><tr>{{ range split "|" .Props.header }}<th scope="col">{{ . }}</th>{{ end }}</tr></thead>{{ end }}<tbody>{{ range lines .Props.body }}<tr>{{ range split "|" . }}<td>{{ . }}</td>{{ end }}</tr>{{ end }}</tbody></table></div></figure>',
    '.stratum-table-fig{margin:0}.stratum-table{width:100%;border-collapse:collapse}.stratum-table-variant-bordered{border:1px solid var(--st-color-border)}.stratum-table-variant-bordered th,.stratum-table-variant-bordered td{border:1px solid var(--st-color-border)}.stratum-table th,.stratum-table td{padding:var(--st-space-sm) var(--st-space-md);text-align:left}.stratum-table thead th{background:var(--st-color-surface-muted);font-weight:600}.stratum-table-striped tbody tr:nth-child(even){background:var(--st-color-surface-muted)}.stratum-table-caption{margin-bottom:var(--st-space-xs);font-size:var(--st-small-size,.875rem);color:var(--st-color-text-muted)}.stratum-table-scroll{overflow-x:auto}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Gallery: responsive image grid
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-gallery-v1', 'core', 'gallery', 1, 'Gallery', 'A responsive grid of images from the Media Library.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{"images":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"columns":{"type":"integer","enum":[2,3,4],"default":3},"gap":{"type":"string","enum":["none","xs","sm","md","lg"],"default":"sm"},"aspectRatio":{"type":"string","enum":["auto","1:1","4:3","16:9","3:2"],"default":"auto"},"captions":{"type":"boolean","default":false}}},"children":{"mode":"none"},"editor":{"category":"media","icon":"gallery","fields":{"props.images":{"label":"Image IDs (comma separated)","control":"text","group":"Content"},"settings.columns":{"label":"Columns","control":"segmented","group":"Layout"},"settings.gap":{"label":"Gap","control":"select","group":"Layout"},"settings.aspectRatio":{"label":"Aspect ratio","control":"select","group":"Style"},"settings.captions":{"label":"Show captions","control":"checkbox","group":"Content"}}}}',
    'template',
    '{{ $ids := split "," .Props.images }}{{ if $ids }}<div class="stratum-gallery stratum-gallery-cols-{{ .Settings.columns }} stratum-gallery-gap-{{ .Settings.gap }}{{ if ne .Settings.aspectRatio "auto" }} stratum-gallery-ar-{{ .Settings.aspectRatio }}{{ end }}">{{ range $ids }}{{ $m := media . }}{{ if $m.Src }}<figure class="stratum-gallery-item">{{ if $m.SrcSet }}<img src="{{ $m.Src }}" srcset="{{ $m.SrcSet }}" sizes="(min-width:992px) 33vw,(min-width:640px) 50vw,100vw" alt="{{ $m.Alt }}" loading="lazy" decoding="async">{{ else }}<img src="{{ $m.Src }}" alt="{{ $m.Alt }}" loading="lazy" decoding="async">{{ end }}</figure>{{ end }}{{ end }}</div>{{ else }}<div class="stratum-gallery stratum-gallery-empty">No images selected</div>{{ end }}',
    '.stratum-gallery{display:grid;grid-template-columns:repeat(2,1fr);gap:var(--st-space-sm)}.stratum-gallery-cols-2{grid-template-columns:repeat(2,1fr)}.stratum-gallery-cols-3{grid-template-columns:repeat(3,1fr)}.stratum-gallery-cols-4{grid-template-columns:repeat(4,1fr)}.stratum-gallery-gap-none{gap:0}.stratum-gallery-gap-xs{gap:var(--st-space-xs)}.stratum-gallery-gap-sm{gap:var(--st-space-sm)}.stratum-gallery-gap-md{gap:var(--st-space-md)}.stratum-gallery-gap-lg{gap:var(--st-space-lg)}.stratum-gallery-ar-1-1 .stratum-gallery-item{aspect-ratio:1/1}.stratum-gallery-ar-4-3 .stratum-gallery-item{aspect-ratio:4/3}.stratum-gallery-ar-16-9 .stratum-gallery-item{aspect-ratio:16/9}.stratum-gallery-ar-3-2 .stratum-gallery-item{aspect-ratio:3/2}.stratum-gallery-item{margin:0;overflow:hidden}.stratum-gallery-item img{display:block;width:100%;height:100%;object-fit:cover}.stratum-gallery-empty{padding:var(--st-space-lg);border:1px dashed var(--st-color-border);text-align:center;color:var(--st-color-text-muted)}@media(max-width:640px){.stratum-gallery-cols-3,.stratum-gallery-cols-4{grid-template-columns:repeat(2,1fr)}}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Video: YouTube, Vimeo or self-hosted file
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-video-v1', 'core', 'video', 1, 'Video', 'Embed a YouTube, Vimeo or self-hosted video.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{"url":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"provider":{"type":"string","enum":["youtube","vimeo","file"],"default":"youtube"},"aspectRatio":{"type":"string","enum":["16:9","4:3","1:1"],"default":"16:9"},"autoplay":{"type":"boolean","default":false},"mute":{"type":"boolean","default":false},"loop":{"type":"boolean","default":false},"controls":{"type":"boolean","default":true},"poster":{"type":"string","default":""}}},"children":{"mode":"none"},"editor":{"category":"media","icon":"video","fields":{"props.url":{"label":"Video URL","control":"text","group":"Content"},"settings.provider":{"label":"Provider","control":"segmented","group":"Source"},"settings.aspectRatio":{"label":"Aspect ratio","control":"select","group":"Style"},"settings.autoplay":{"label":"Autoplay","control":"checkbox","group":"Playback"},"settings.mute":{"label":"Muted","control":"checkbox","group":"Playback"},"settings.loop":{"label":"Loop","control":"checkbox","group":"Playback"},"settings.controls":{"label":"Show controls","control":"checkbox","group":"Playback"},"settings.poster":{"label":"Poster image ID (file only)","control":"text","group":"Source"}}}}',
    'template',
    '{{ $id := "" }}{{ if eq .Settings.provider "youtube" }}{{ $id = youtubeID .Props.url }}{{ else if eq .Settings.provider "vimeo" }}{{ $id = vimeoID .Props.url }}{{ end }}<div class="stratum-video stratum-video-ar-{{ .Settings.aspectRatio }}">{{ if and (eq .Settings.provider "youtube") $id }}<iframe class="stratum-video-frame" src="https://www.youtube.com/embed/{{ $id }}?rel=0{{ if .Settings.autoplay }}&autoplay=1{{ end }}{{ if .Settings.mute }}&mute=1{{ end }}{{ if .Settings.loop }}&loop=1{{ end }}" title="YouTube video" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture" allowfullscreen loading="lazy"></iframe>{{ else if and (eq .Settings.provider "vimeo") $id }}<iframe class="stratum-video-frame" src="https://player.vimeo.com/video/{{ $id }}{{ if .Settings.autoplay }}?autoplay=1{{ end }}{{ if .Settings.mute }}&muted=1{{ end }}{{ if .Settings.loop }}&loop=1{{ end }}" title="Vimeo video" allow="autoplay; fullscreen; picture-in-picture" allowfullscreen loading="lazy"></iframe>{{ else if eq .Settings.provider "file" }}{{ $p := media .Settings.poster }}<video class="stratum-video-frame"{{ if .Settings.controls }} controls{{ end }}{{ if .Settings.autoplay }} autoplay{{ end }}{{ if .Settings.mute }} muted{{ end }}{{ if .Settings.loop }} loop{{ end }}{{ if $p.Src }} poster="{{ $p.Src }}"{{ end }} src="{{ .Props.url }}"></video>{{ else }}<div class="stratum-video-missing">Video unavailable</div>{{ end }}</div>',
    '.stratum-video{position:relative;width:100%;margin:0}.stratum-video-ar-16-9{aspect-ratio:16/9}.stratum-video-ar-4-3{aspect-ratio:4/3}.stratum-video-ar-1-1{aspect-ratio:1/1}.stratum-video-frame{position:absolute;inset:0;width:100%;height:100%;border:0;display:block}.stratum-video-missing{aspect-ratio:16/9;display:flex;align-items:center;justify-content:center;border:1px dashed var(--st-color-border);color:var(--st-color-text-muted)}',
    'core', 1, unixepoch(), unixepoch()
);
