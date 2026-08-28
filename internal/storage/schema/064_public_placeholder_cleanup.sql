-- 064_public_placeholder_cleanup.sql
-- Ensure authoring placeholders are preview-only, not public.

PRAGMA foreign_keys = ON;

-- Image: public with missing media renders nothing, preview shows placeholder
UPDATE block_definitions SET
    template = '{{ $m := media .Props.mediaId }}{{ if $m.Src }}{{ $sizes := "(min-width: 768px) min(100vw, 1200px), 100vw" }}{{ if .Settings.sizes }}{{ $sizes = .Settings.sizes }}{{ end }}<figure class="stratum-image stratum-image-align-{{ .Settings.align }} stratum-image-radius-{{ .Settings.radius }} stratum-image-aspect-{{ .Settings.aspect }} stratum-image-fit-{{ .Settings.fit }}">{{ if $m.WebPSrcSet }}<picture><source type="image/webp" srcset="{{ $m.WebPSrcSet }}" sizes="{{ $sizes }}">{{ end }}<img src="{{ $m.Src }}"{{ if $m.SrcSet }} srcset="{{ $m.SrcSet }}" sizes="{{ $sizes }}"{{ end }}{{ if $m.Width }} width="{{ $m.Width }}"{{ end }}{{ if $m.Height }} height="{{ $m.Height }}"{{ end }} alt="{{ if not .Settings.decorative }}{{ if .Props.alt }}{{ .Props.alt }}{{ else }}{{ $m.Alt }}{{ end }}{{ end }}"{{ if .Priority }} loading="eager" fetchpriority="high" decoding="async"{{ else }} loading="lazy" decoding="async"{{ end }}>{{ if $m.WebPSrcSet }}</picture>{{ end }}{{ if .Props.caption }}<figcaption class="stratum-image-caption">{{ .Props.caption }}</figcaption>{{ end }}</figure>{{ else }}{{ if .Context.IsPreview }}<div class="stratum-image stratum-image-missing" role="img" aria-label="Missing image">Image unavailable</div>{{ end }}{{ end }}',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'image' AND version = 1;

-- Site Logo: public with no logo renders nothing
UPDATE block_definitions SET
    template = '{{ if .Context.Site.LogoURL }}{{ if .Settings.link }}<a href="{{ if .Context.Site.URL }}{{ .Context.Site.URL }}{{ else }}/{{ end }}" class="stratum-site-logo-link">{{ end }}<img class="stratum-site-logo" src="{{ .Context.Site.LogoURL }}" alt="{{ .Context.Site.Name }}"{{ if .Context.Site.LogoWidth }} width="{{ .Context.Site.LogoWidth }}"{{ end }}{{ if .Context.Site.LogoHeight }} height="{{ .Context.Site.LogoHeight }}"{{ end }}{{ if .Settings.width }} style="max-width:{{ .Settings.width }}px"{{ end }}>{{ if .Settings.link }}</a>{{ end }}{{ else }}{{ if .Context.IsPreview }}<span class="stratum-placeholder">Site logo</span>{{ end }}{{ end }}',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'site-logo' AND version = 1;

-- Video (historical): preview shows placeholder, public empty
UPDATE block_definitions SET
    template = '{{ $id := "" }}{{ if eq .Settings.provider "youtube" }}{{ $id = youtubeID .Props.url }}{{ else if eq .Settings.provider "vimeo" }}{{ $id = vimeoID .Props.url }}{{ end }}<div class="stratum-video stratum-video-ar-{{ .Settings.aspectRatio }}">{{ if and (eq .Settings.provider "youtube") $id }}<iframe class="stratum-video-frame" src="https://www.youtube.com/embed/{{ $id }}?rel=0{{ if .Settings.autoplay }}&autoplay=1{{ end }}{{ if .Settings.mute }}&mute=1{{ end }}{{ if .Settings.loop }}&loop=1{{ end }}" title="YouTube video" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture" allowfullscreen loading="lazy"></iframe>{{ else if and (eq .Settings.provider "vimeo") $id }}<iframe class="stratum-video-frame" src="https://player.vimeo.com/video/{{ $id }}{{ if .Settings.autoplay }}?autoplay=1{{ end }}{{ if .Settings.mute }}&muted=1{{ end }}{{ if .Settings.loop }}&loop=1{{ end }}" title="Vimeo video" allow="autoplay; fullscreen; picture-in-picture" allowfullscreen loading="lazy"></iframe>{{ else if eq .Settings.provider "file" }}{{ $p := media .Settings.poster }}<video class="stratum-video-frame"{{ if .Settings.controls }} controls{{ end }}{{ if .Settings.autoplay }} autoplay{{ end }}{{ if .Settings.mute }} muted{{ end }}{{ if .Settings.loop }} loop{{ end }}{{ if $p.Src }} poster="{{ $p.Src }}"{{ end }} src="{{ .Props.url }}"></video>{{ else }}{{ if .Context.IsPreview }}<div class="stratum-video-missing">Video unavailable</div>{{ end }}{{ end }}</div>',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'video' AND version = 1;
