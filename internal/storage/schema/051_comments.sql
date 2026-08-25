-- Comments belong to Entry, comments_enabled is revision-scoped.

ALTER TABLE entry_revisions ADD COLUMN comments_enabled INTEGER NOT NULL DEFAULT 0 CHECK (comments_enabled IN (0, 1));

-- Backfill existing revisions: Posts default true, Pages false, others false.
UPDATE entry_revisions SET comments_enabled = 1 WHERE entry_id IN (SELECT id FROM entries WHERE content_type_id = 'post');

CREATE TABLE comments (
    id TEXT PRIMARY KEY,
    entry_id TEXT NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    parent_id TEXT REFERENCES comments(id) ON DELETE SET NULL,
    status TEXT NOT NULL CHECK (status IN ('pending','approved','spam','trash')),
    author_name TEXT NOT NULL,
    author_email TEXT NOT NULL,
    author_url TEXT NOT NULL DEFAULT '',
    user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    body TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    imported_source TEXT,
    imported_external_id TEXT,
    CHECK (length(author_name) > 0),
    CHECK (length(body) > 0)
);

CREATE INDEX idx_comments_entry_status_created ON comments(entry_id, status, created_at);
CREATE INDEX idx_comments_parent ON comments(parent_id);
CREATE INDEX idx_comments_status_created ON comments(status, created_at);
CREATE UNIQUE INDEX idx_comments_import_identity ON comments(imported_source, imported_external_id) WHERE imported_source IS NOT NULL AND imported_external_id IS NOT NULL;

-- Dynamic block: core/comments
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-comments-v1', 'core', 'comments', 1, 'Comments', 'Displays approved comments and the comment form.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"comment","contexts":["entry"]}}',
    'template',
    '<section id="comments" class="stratum-comments"><h2 class="stratum-comments-title">Comments ({{ .Context.CommentsCount }})</h2><div id="comment-list">{{ if .Context.Comments }}{{ range .Context.Comments }}<article class="stratum-comment" id="comment-{{ .ID }}" style="margin-left:{{ .Depth }}rem"><header class="stratum-comment-header"><span class="stratum-comment-author">{{ .AuthorName }}</span> <time datetime="{{ .CreatedISO }}">{{ .CreatedAt }}</time>{{ if .ParentID }} <a href="#comment-{{ .ParentID }}">in reply</a>{{ end }}</header><div class="stratum-comment-body">{{ .BodyHTML }}</div></article>{{ end }}{{ else }}<p class="stratum-comments-empty">No comments yet.</p>{{ end }}</div>{{ if .Context.CommentsEnabled }}<form id="comment-form" method="post" action="/comments" class="stratum-comment-form" data-on:submit__prevent="@post(''/comments'', {contentType: ''form''})"><input type="hidden" name="entry_id" value="{{ .Context.CommentsEntryID }}"><input type="hidden" name="parent_id" value=""><div id="comment-form-errors"></div><p><label>Name <input name="author_name" required maxlength="100"></label></p><p><label>Email <input name="author_email" type="email" required maxlength="254"></label></p><p><label>Website <input name="author_url" type="url" maxlength="2048"></label></p><p style="display:none"><label>Leave empty <input name="website_confirm" autocomplete="off"></label></p><p><label>Comment <textarea name="body" required maxlength="5000"></textarea></label></p><p><button type="submit">Post Comment</button></p><div id="comment-submit-status"></div></form>{{ else }}<p class="stratum-comments-disabled">Comments are closed.</p>{{ end }}</section>',
    '.stratum-comments{margin-top:var(--st-space-xl)}.stratum-comment{border-top:1px solid var(--st-color-border);padding:var(--st-space-md) 0}.stratum-comment-body{white-space:pre-wrap;word-wrap:break-word}',
    'core', 1, unixepoch(), unixepoch()
) ON CONFLICT(namespace, name, version) DO NOTHING;
