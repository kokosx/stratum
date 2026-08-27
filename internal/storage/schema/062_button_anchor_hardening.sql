-- 062_button_anchor_hardening.sql
-- Harden Button URL safety and Section anchorID sanitization.
-- Uses new template helpers safeURL and anchorID.

PRAGMA foreign_keys = ON;

-- Button: use safeURL, do not render unsafe javascript/data URLs.
UPDATE block_definitions SET
    template = '{{ $url := safeURL .Props.url }}{{ if and .Props.label $url }}<div class="stratum-btn-wrap stratum-align-{{ .Settings.align }}{{ if eq .Settings.width "full" }} stratum-btn-wrap-full{{ end }}"><a class="stratum-button stratum-button-{{ .Settings.variant }} stratum-button-size-{{ .Settings.size }}{{ if eq .Settings.width "full" }} stratum-button-full{{ end }}" href="{{ $url }}"{{ if .Settings.openInNewTab }} target="_blank" rel="noopener noreferrer"{{ end }}>{{ .Props.label }}</a></div>{{ end }}',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'button' AND version = 1;

-- Section: sanitize anchorID via anchorID helper, never emit empty id=""
UPDATE block_definitions SET
    template = '{{ $aid := anchorID .Settings.anchorID }}<section{{ if $aid }} id="{{ $aid }}"{{ end }} class="stratum-section stratum-section-width-{{ .Settings.width }} stratum-section-vspace-{{ .Settings.verticalSpacing }} stratum-section-hpad-{{ .Settings.horizontalPadding }} stratum-section-align-{{ .Settings.align }} stratum-section-bg-{{ .Settings.background }} stratum-section-minh-{{ .Settings.minHeight }}">{{ .Children }}</section>',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'section' AND version = 1;
