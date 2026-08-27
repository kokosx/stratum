-- 055_entry_link_route_less.sql
-- Entry link must not synthesize a link when the current entry has no permalink (route-less content type).
-- Public renderer: omit link wrapper, render inner text only. Preview: show unavailable warning.
PRAGMA foreign_keys = ON;

UPDATE block_definitions
SET template = '{{ if .Context.Entry.Permalink }}<a class="stratum-entry-link" href="{{ .Context.Entry.Permalink }}"{{ if .Settings.openInNewTab }} target="_blank" rel="noopener"{{ end }}>{{ if .Props.text }}{{ .Props.text }}{{ else }}{{ .Context.Entry.Title }}{{ end }}</a>{{ else }}{{ if .Context.IsPreview }}<span class="stratum-placeholder">This content type does not have individual pages.</span>{{ else }}{{ if .Props.text }}{{ .Props.text }}{{ else }}{{ .Context.Entry.Title }}{{ end }}{{ end }}{{ end }}',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'entry-link' AND version = 1;
