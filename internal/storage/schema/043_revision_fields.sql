ALTER TABLE entry_revisions ADD COLUMN fields_json TEXT NOT NULL DEFAULT '{}'
    CHECK (json_valid(fields_json) AND json_type(fields_json) = 'object');
