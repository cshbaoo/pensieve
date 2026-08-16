package index

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Index SQLite 索引（FTS5 全文 + 向量 blob），是可随时重建的派生物
type Index struct {
	db *sql.DB
}

func Open(indexDir string) (*Index, error) {
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(indexDir, "index.db"))
	if err != nil {
		return nil, err
	}
	// WAL 模式 + 正常同步级别：索引可重建，不追求强一致写
	for _, pragma := range []string{"PRAGMA journal_mode=WAL", "PRAGMA synchronous=NORMAL"} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s 失败: %w", pragma, err)
		}
	}
	idx := &Index{db: db}
	if err := idx.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	// schema 迁移：mem_links 缺 rel 列时重建该表（链接是派生数据，可无损重建）
	var colExists int
	_ = db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('mem_links') WHERE name='rel'`).Scan(&colExists)
	if colExists == 0 {
		if _, err := db.Exec(`DROP TABLE IF EXISTS mem_links`); err != nil {
			db.Close()
			return nil, fmt.Errorf("mem_links 重建失败: %w", err)
		}
		if err := idx.migrate(); err != nil {
			db.Close()
			return nil, err
		}
		// 标记需要全量重建索引以恢复 links
		_ = idx.SetMeta(idx.dbContext(), "need_full_reindex", "1")
	}
	return idx, nil
}

func (i *Index) dbContext() context.Context { return context.Background() }

func (i *Index) migrate() error {
	ddl := `
CREATE TABLE IF NOT EXISTS memories (
    id          TEXT PRIMARY KEY,
    type        TEXT NOT NULL,
    title       TEXT NOT NULL,
    project     TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active',
    confidence  TEXT NOT NULL DEFAULT 'ai-inferred',
    votes       INTEGER NOT NULL DEFAULT 0,
    created     INTEGER NOT NULL,
    path        TEXT NOT NULL,
    body        TEXT NOT NULL
);
CREATE VIRTUAL TABLE IF NOT EXISTS mem_fts USING fts5(
    id UNINDEXED, title, body, tags, entities,
    tokenize='trigram'
);
CREATE TABLE IF NOT EXISTS mem_vec (
    id    TEXT PRIMARY KEY,
    dim   INTEGER NOT NULL,
    model TEXT NOT NULL,
    vec   BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS mem_entities (
    entity    TEXT NOT NULL,
    memory_id TEXT NOT NULL,
    PRIMARY KEY (entity, memory_id)
);
CREATE INDEX IF NOT EXISTS idx_entity ON mem_entities(entity);

-- 记忆间双向链接(topic 卷宗/supersede 谱系等统一在此)
CREATE TABLE IF NOT EXISTS mem_links (
    from_id TEXT NOT NULL,
    to_id   TEXT NOT NULL,
    rel     TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (from_id, to_id)
);
CREATE INDEX IF NOT EXISTS idx_links_to ON mem_links(to_id);
CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT);
`
	_, err := i.db.Exec(ddl)
	return err
}

func (i *Index) Close() error { return i.db.Close() }

// SetMeta / GetMeta 索引与事实源一致性控制
func (i *Index) SetMeta(ctx context.Context, key, value string) error {
	_, err := i.db.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (i *Index) GetMeta(ctx context.Context, key string) (string, error) {
	var v string
	err := i.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}
