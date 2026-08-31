-- Editor visual root metadata for accurate selection bounds.
-- Button's natural root is a wrapper (stratum-btn-wrap) but the visual widget is the inner link/button.
-- Image's natural root is figure wrapper but the visual is the inner img.
-- visualRoot is optional CSS selector stored in block schema editor metadata; renderer remains generic.

UPDATE block_definitions SET schema_json = json_set(schema_json, '$.editor.visualRoot', '.stratum-button'), updated_at = unixepoch() WHERE namespace = 'core' AND name = 'button' AND version = 1;

UPDATE block_definitions SET schema_json = json_set(schema_json, '$.editor.visualRoot', 'img'), updated_at = unixepoch() WHERE namespace = 'core' AND name = 'image' AND version = 1;
