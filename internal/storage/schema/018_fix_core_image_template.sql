-- Fix the core/image template: it referenced props/settings keys
-- (mediaID, objectFit, aspectRatio, loading) that do not exist on the block's
-- actual schema (mediaId, align, decorative, sizes, eager), so .Props.mediaID was
-- always empty and images never resolved. It also emitted a static attribute from
-- inside an {{ if }} action, which breaks html/template. Align the template with the
-- real schema and make fetchpriority a static attribute with a conditional value.
UPDATE block_definitions
SET template = '{{ $m := media .Props.mediaId }}{{ if $m.Src }}<figure class="stratum-image stratum-image-align-{{ .Settings.align }}"><img src="{{ $m.Src }}"{{ if $m.SrcSet }} srcset="{{ $m.SrcSet }}"{{ if .Settings.sizes }} sizes="{{ .Settings.sizes }}"{{ end }}{{ end }}{{ if $m.Width }} width="{{ $m.Width }}"{{ end }}{{ if $m.Height }} height="{{ $m.Height }}"{{ end }} alt="{{ if not .Settings.decorative }}{{ if .Props.alt }}{{ .Props.alt }}{{ else }}{{ $m.Alt }}{{ end }}{{ end }}" decoding="async" fetchpriority="{{ if .Settings.eager }}high{{ end }}">{{ if .Props.caption }}<figcaption class="stratum-image-caption">{{ .Props.caption }}</figcaption>{{ end }}</figure>{{ else }}<div class="stratum-image stratum-image-missing">Image unavailable</div>{{ end }}',
    updated_at = unixepoch()
WHERE id = 'core-image-v1';
