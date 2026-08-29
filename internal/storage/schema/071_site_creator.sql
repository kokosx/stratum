-- Existing installations must remain in normal admin after this migration.
ALTER TABLE site_settings
ADD COLUMN onboarding_completed INTEGER NOT NULL DEFAULT 1
CHECK (onboarding_completed IN (0, 1));
