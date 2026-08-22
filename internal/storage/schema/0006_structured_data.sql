-- ===== 0006_structured_data.sql =====
-- First-party structured data (JSON-LD): the Schema.org publisher entity is
-- either an Organization or a Person, chosen by an explicit site setting.

ALTER TABLE site_settings ADD COLUMN site_represents TEXT NOT NULL DEFAULT 'organization'
    CHECK (site_represents IN ('organization', 'person'));

-- entries.published_at tracks the publication of the CURRENT published revision
-- and moves on every re-publish. Structured data needs a stable datePublished,
-- so first publication is stored separately and never rewritten.
ALTER TABLE entries ADD COLUMN first_published_at INTEGER;

UPDATE entries SET first_published_at = published_at
WHERE first_published_at IS NULL AND published_at IS NOT NULL;
