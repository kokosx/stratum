-- Corrective: empty dynamic blocks must not create phantom gaps.
-- Editor placeholder never in public output; wrappers gone when value empty.
-- NOTE: entry-field and entry-media are runtime renderers (see internal/rendering/renderer.go
-- entryFieldRenderer / entryMediaRenderer) — they already return "" when value missing
-- and handle preview placeholders in Go. No template change needed.

-- entry-excerpt: template block — public: output nothing when excerpt empty (no phantom gap).
-- Editor preview: show genuine placeholder where useful (when IsPreview true).
-- This removes the phantom gap seen in pageTemplate when excerpt is "".
UPDATE block_definitions
SET template = '{{if .Context.Entry.Excerpt}}<p class="stratum-entry-excerpt stratum-align-{{ .Settings.align }} stratum-tone-{{ .Settings.tone }} stratum-text-size-{{ .Settings.size }}">{{ .Context.Entry.Excerpt }}</p>{{else if .Context.IsPreview}}<p class="stratum-entry-excerpt stratum-align-{{ .Settings.align }} stratum-tone-{{ .Settings.tone }} stratum-text-size-{{ .Settings.size }}"><span class="stratum-placeholder">Entry excerpt</span></p>{{end}}',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'entry-excerpt' AND version = 1;

-- Ensure form block style reflects correct gaps (fallback if server theme not reloaded, DB CSS is fallback)
UPDATE block_definitions
SET styles = '.stratum-form{display:grid;gap:var(--st-space-lg);width:100%;max-width:var(--st-prose-width, 720px)}.stratum-form-field{display:grid;gap:var(--st-space-xs)}.stratum-form-field label{font-weight:500;font-size:var(--st-small-size, 0.9rem);color:var(--st-color-text)}.stratum-form input:not([type=checkbox]):not([type=hidden]),.stratum-form textarea,.stratum-form select{width:100%;box-sizing:border-box;padding:var(--st-space-sm);color:var(--st-color-text);background:var(--st-color-background, #fff);border:1px solid var(--st-color-border);border-radius:var(--st-radius-sm, 6px);font:inherit;font-size:var(--st-body-size, 1rem);line-height:1.5}.stratum-form input[type=checkbox]{accent-color:var(--st-color-primary)}.stratum-form textarea{min-height:9rem;resize:vertical;line-height:1.6}.stratum-form select{appearance:none}.stratum-form input:focus-visible,.stratum-form textarea:focus-visible,.stratum-form select:focus-visible{outline:3px solid var(--st-color-focus);outline-offset:2px;border-color:var(--st-color-focus)}.stratum-form button[type=submit]{display:inline-flex;align-items:center;justify-content:center;min-height:42px;padding:var(--st-button-padding-y, 0.6rem) var(--st-button-padding-x, 1rem);background:var(--st-color-primary);color:var(--st-color-primary-contrast);border:2px solid transparent;border-radius:var(--st-button-radius, var(--st-radius-sm, 6px));font-weight:var(--st-button-font-weight, 600);font-size:0.95rem;line-height:1.2;cursor:pointer;transition:background-color 0.15s ease}.stratum-form button[type=submit]:hover{background:var(--st-color-primary-hover)}.stratum-form button[type=submit]:focus-visible{outline:3px solid var(--st-color-focus);outline-offset:2px}.stratum-form-honeypot{position:absolute!important;width:1px!important;height:1px!important;overflow:hidden!important;clip:rect(0,0,0,0)!important;white-space:nowrap!important}.stratum-form-success{color:var(--st-color-text);background:var(--st-color-surface-muted, #f6f6f6);border:1px solid var(--st-color-border);border-radius:var(--st-radius-sm, 6px);padding:14px 16px;margin-bottom:22px}.stratum-form--disabled{padding:var(--st-space-md);border:1px solid var(--st-color-border);border-radius:var(--st-radius-sm, 6px);background:var(--st-color-bg-muted, #f9fafb);color:var(--st-color-text-muted, #6b7280)}',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'form' AND version IN (1,2);
