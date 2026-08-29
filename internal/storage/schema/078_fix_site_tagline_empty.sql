-- Fix site-tagline to render nothing when tagline empty but site context present.
-- Editor placeholder only when no real SiteContext (no Site.Name).
UPDATE block_definitions
SET template = '{{if .Context.Site.Tagline}}<p class="stratum-site-tagline stratum-align-{{ .Settings.align }}">{{ .Context.Site.Tagline }}</p>{{else if not .Context.Site.Name}}<p class="stratum-site-tagline stratum-align-{{ .Settings.align }}"><span class="stratum-placeholder">Site tagline</span></p>{{end}}',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'site-tagline' AND version = 1;
