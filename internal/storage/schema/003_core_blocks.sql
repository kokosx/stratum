INSERT INTO block_definitions (
    id,
    namespace,
    name,
    version,
    display_name,
    schema_json,
    renderer_type,
    template,
    source,
    enabled,
    created_at,
    updated_at
)
VALUES
    (
        'core-heading-v1',
        'core',
        'heading',
        1,
        'Heading',
        '{"type":"object","required":["text"],"properties":{"text":{"type":"string"},"level":{"type":"integer","minimum":1,"maximum":6}}}',
        'template',
        '<h2>{{ .Props.text }}</h2>',
        'core',
        1,
        unixepoch(),
        unixepoch()
    ),
    (
        'core-text-v1',
        'core',
        'text',
        1,
        'Text',
        '{"type":"object","required":["text"],"properties":{"text":{"type":"string"}}}',
        'template',
        '<p>{{ .Props.text }}</p>',
        'core',
        1,
        unixepoch(),
        unixepoch()
    )
ON CONFLICT(namespace, name, version) DO UPDATE SET
    display_name = excluded.display_name,
    schema_json = excluded.schema_json,
    renderer_type = excluded.renderer_type,
    template = excluded.template,
    source = excluded.source,
    enabled = excluded.enabled,
    updated_at = excluded.updated_at;
