-- Agent-native MCP foundation
-- Agents are service identities distinct from human users.

-- Revision actor attribution: distinguish user/agent/system creators.
ALTER TABLE entry_revisions ADD COLUMN created_by_kind TEXT NOT NULL DEFAULT 'user'
    CHECK (created_by_kind IN ('user', 'agent', 'system'));
-- Historical revisions were created by humans (users).
UPDATE entry_revisions SET created_by_kind = 'user' WHERE created_by_kind = 'user';

CREATE TABLE agents (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL CHECK (length(name) <= 120),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    default_author_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    CHECK (length(name) >= 1)
);
CREATE INDEX idx_agents_status ON agents(status);

CREATE TABLE agent_tokens (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    token_prefix TEXT NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    expires_at INTEGER,
    last_used_at INTEGER,
    revoked_at INTEGER
);
CREATE INDEX idx_agent_tokens_hash ON agent_tokens(token_hash);
CREATE INDEX idx_agent_tokens_agent ON agent_tokens(agent_id);

CREATE TABLE agent_grants (
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    permission TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT '*',
    PRIMARY KEY(agent_id, permission, scope),
    CHECK (scope = '*' OR scope LIKE 'content_type:%')
);
CREATE INDEX idx_agent_grants_agent ON agent_grants(agent_id);

CREATE TABLE audit_events (
    id TEXT PRIMARY KEY,
    actor_kind TEXT NOT NULL CHECK (actor_kind IN ('user', 'agent', 'system')),
    actor_id TEXT,
    transport TEXT NOT NULL CHECK (transport IN ('admin', 'mcp', 'system', 'cli')),
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT,
    revision_id TEXT,
    metadata_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(metadata_json)),
    created_at INTEGER NOT NULL
);
CREATE INDEX idx_audit_resource ON audit_events(resource_type, resource_id, created_at);
CREATE INDEX idx_audit_actor ON audit_events(actor_kind, actor_id, created_at);
CREATE INDEX idx_audit_recent ON audit_events(created_at DESC);
CREATE INDEX idx_audit_revision ON audit_events(revision_id);
