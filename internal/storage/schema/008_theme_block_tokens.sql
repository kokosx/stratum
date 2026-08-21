UPDATE block_definitions SET styles = '.stratum-tone-muted{color:var(--st-color-text-muted,#667085)}.stratum-tone-accent{color:var(--st-color-primary,#175cd3)}', updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'text' AND version = 1;

UPDATE block_definitions SET styles = '.stratum-button{display:inline-block;padding:var(--st-button-padding-y,.65rem) var(--st-button-padding-x,1rem);border-radius:var(--st-button-radius,.35rem);font-weight:var(--st-button-font-weight,600);text-decoration:none}.stratum-button-primary{background:var(--st-color-primary,#175cd3);color:var(--st-color-primary-contrast,#fff)}.stratum-button-secondary{border:var(--st-border-width,1px) solid currentColor;color:var(--st-color-secondary,currentColor)}', updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'button' AND version = 1;

UPDATE block_definitions SET styles = '.stratum-section{margin-inline:auto}.stratum-width-normal{max-width:var(--st-content-width,720px)}.stratum-width-wide{max-width:var(--st-wide-width,1100px)}.stratum-width-full{max-width:none}.stratum-spacing-none{padding-block:0}.stratum-spacing-sm{padding-block:var(--st-space-lg,1rem)}.stratum-spacing-md{padding-block:var(--st-space-2xl,2rem)}.stratum-spacing-lg{padding-block:var(--st-space-3xl,4rem)}', updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'section' AND version = 1;

UPDATE block_definitions SET styles = '.stratum-stack{display:flex;flex-direction:column}.stratum-gap-none{gap:0}.stratum-gap-sm{gap:var(--st-space-sm,.5rem)}.stratum-gap-md{gap:var(--st-space-md,1rem)}.stratum-gap-lg{gap:var(--st-space-lg,2rem)}', updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'stack' AND version = 1;
