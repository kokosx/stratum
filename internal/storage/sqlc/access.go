package db

// DB exposes the underlying query executor to narrowly-scoped runtime services
// that need SQLite features not represented by generated CRUD queries.
func (q *Queries) DB() DBTX { return q.db }
