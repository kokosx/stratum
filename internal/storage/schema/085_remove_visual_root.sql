-- Remove premature visualRoot abstraction. Editor selects natural SDT block box.
UPDATE block_definitions SET schema_json = json_remove(schema_json, '$.editor.visualRoot'), updated_at = unixepoch() WHERE namespace = 'core' AND name = 'button' AND version = 1;
UPDATE block_definitions SET schema_json = json_remove(schema_json, '$.editor.visualRoot'), updated_at = unixepoch() WHERE namespace = 'core' AND name = 'image' AND version = 1;
