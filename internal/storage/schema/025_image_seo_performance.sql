-- ===== 0007_image_seo_performance.sql =====
-- Image SEO/performance pass:
--   1. media_variants gains content_hash so derivative URLs can be versioned.
--      Derivatives are served immutable (max-age=31536000), but regeneration
--      (social preview focal change, favicon rebuild) can replace the bytes
--      behind the same kind. Resolvers append ?v=<hash> so changed bytes never
--      hide behind an already-cached URL.
--   2. Block templates gain WebP derivative sources (<picture>, emitted only
--      when WebP variants exist), width/height on every core <img> whose
--      metadata is known, and keep the LCP rules: exactly one prioritized
--      image gets loading=eager + fetchpriority=high, everything else gets
--      loading=lazy + decoding=async.

ALTER TABLE media_variants ADD COLUMN content_hash TEXT NOT NULL DEFAULT '';

UPDATE block_definitions SET
    template = '{{ $m := media .Props.mediaId }}{{ if $m.Src }}{{ $sizes := "(min-width: 768px) min(100vw, 1200px), 100vw" }}{{ if .Settings.sizes }}{{ $sizes = .Settings.sizes }}{{ end }}<figure class="stratum-image stratum-image-align-{{ .Settings.align }}">{{ if $m.WebPSrcSet }}<picture><source type="image/webp" srcset="{{ $m.WebPSrcSet }}" sizes="{{ $sizes }}">{{ end }}<img src="{{ $m.Src }}"{{ if $m.SrcSet }} srcset="{{ $m.SrcSet }}" sizes="{{ $sizes }}"{{ end }}{{ if $m.Width }} width="{{ $m.Width }}"{{ end }}{{ if $m.Height }} height="{{ $m.Height }}"{{ end }} alt="{{ if not .Settings.decorative }}{{ if .Props.alt }}{{ .Props.alt }}{{ else }}{{ $m.Alt }}{{ end }}{{ end }}"{{ if .Priority }} loading="eager" fetchpriority="high" decoding="async"{{ else }} loading="lazy" decoding="async"{{ end }}>{{ if $m.WebPSrcSet }}</picture>{{ end }}{{ if .Props.caption }}<figcaption class="stratum-image-caption">{{ .Props.caption }}</figcaption>{{ end }}</figure>{{ else }}<div class="stratum-image stratum-image-missing" role="img" aria-label="Missing image">Image unavailable</div>{{ end }}',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'image' AND version = 1;

UPDATE block_definitions SET
    template = '{{ $m := media .Context.Entry.FeaturedImage }}{{ if $m.Src }}<figure class="stratum-featured-image stratum-featured-image-fit-{{ .Settings.objectFit }} stratum-featured-image-ar-{{ .Settings.aspectRatio }} stratum-featured-image-align-{{ .Settings.align }}">{{ if $m.WebPSrcSet }}<picture><source type="image/webp" srcset="{{ $m.WebPSrcSet }}" sizes="(min-width:992px) 768px,(min-width:640px) 480px,100vw">{{ end }}<img src="{{ $m.Src }}"{{ if $m.SrcSet }} srcset="{{ $m.SrcSet }}" sizes="(min-width:992px) 768px,(min-width:640px) 480px,100vw"{{ end }}{{ if $m.Width }} width="{{ $m.Width }}"{{ end }}{{ if $m.Height }} height="{{ $m.Height }}"{{ end }} alt="{{ $m.Alt }}"{{ if .Priority }} loading="eager" fetchpriority="high" decoding="async"{{ else }} loading="lazy" decoding="async"{{ end }}>{{ if $m.WebPSrcSet }}</picture>{{ end }}</figure>{{ else }}<div class="stratum-featured-image stratum-featured-image-missing">Featured image</div>{{ end }}',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'featured-image' AND version = 1;

UPDATE block_definitions SET
    template = '{{ $ids := split "," .Props.images }}{{ if $ids }}<div class="stratum-gallery stratum-gallery-cols-{{ .Settings.columns }} stratum-gallery-gap-{{ .Settings.gap }}{{ if ne .Settings.aspectRatio "auto" }} stratum-gallery-ar-{{ .Settings.aspectRatio }}{{ end }}">{{ range $ids }}{{ $m := media . }}{{ if $m.Src }}<figure class="stratum-gallery-item">{{ if $m.WebPSrcSet }}<picture><source type="image/webp" srcset="{{ $m.WebPSrcSet }}" sizes="(min-width:992px) 33vw,(min-width:640px) 50vw,100vw">{{ end }}<img src="{{ $m.Src }}"{{ if $m.SrcSet }} srcset="{{ $m.SrcSet }}" sizes="(min-width:992px) 33vw,(min-width:640px) 50vw,100vw"{{ end }}{{ if $m.Width }} width="{{ $m.Width }}"{{ end }}{{ if $m.Height }} height="{{ $m.Height }}"{{ end }} alt="{{ $m.Alt }}" loading="lazy" decoding="async">{{ if $m.WebPSrcSet }}</picture>{{ end }}</figure>{{ end }}{{ end }}</div>{{ else }}<div class="stratum-gallery stratum-gallery-empty">No images selected</div>{{ end }}',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'gallery' AND version = 1;

UPDATE block_definitions SET
    template = '{{ if .Context.Site.LogoURL }}{{ if .Settings.link }}<a class="stratum-logo" href="{{ if .Context.Site.URL }}{{ .Context.Site.URL }}{{ else }}/{{ end }}">{{ end }}<img class="stratum-logo-img stratum-logo-size-{{ .Settings.size }}" src="{{ .Context.Site.LogoURL }}" alt="{{ .Context.Site.Name }}"{{ if .Context.Site.LogoWidth }} width="{{ .Context.Site.LogoWidth }}"{{ end }}{{ if .Context.Site.LogoHeight }} height="{{ .Context.Site.LogoHeight }}"{{ end }}>{{ if .Settings.link }}</a>{{ end }}{{ else if .Context.Site.Name }}{{ if .Settings.link }}<a class="stratum-logo stratum-logo-text" href="{{ if .Context.Site.URL }}{{ .Context.Site.URL }}{{ else }}/{{ end }}">{{ end }}{{ .Context.Site.Name }}{{ if .Settings.link }}</a>{{ end }}{{ else }}<span class="stratum-placeholder">Site logo</span>{{ end }}',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'logo' AND version = 1;