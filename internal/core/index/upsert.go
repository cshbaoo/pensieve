package index

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"github.com/cshbaoo/pensieve/internal/core/memory"
)

// Upsert 写入/更新一条记忆的全部索引
func (i *Index) Upsert(ctx context.Context, m *memory.Memory, vec []float32, vecModel string) error {
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `INSERT INTO memories(id,type,title,project,status,confidence,votes,created,path,body)
		VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET type=excluded.type,title=excluded.title,project=excluded.project,
		status=excluded.status,confidence=excluded.confidence,votes=excluded.votes,created=excluded.created,
		path=excluded.path,body=excluded.body`,
		m.ID, m.Type, m.Title, m.Project, m.Status, m.Confidence, m.Votes, m.Created.Unix(), m.Path, m.Body); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM mem_fts WHERE id=?`, m.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO mem_fts(id,title,body,tags,entities) VALUES(?,?,?,?,?)`,
		m.ID, m.Title, m.Body, joinSpace(m.Tags), joinSpace(m.Entities)); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM mem_entities WHERE memory_id=?`, m.ID); err != nil {
		return err
	}
	for _, e := range m.Entities {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO mem_entities(entity,memory_id) VALUES(?,?)`, e, m.ID); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM mem_links WHERE from_id=?`, m.ID); err != nil {
		return err
	}
	for _, lk := range m.Links {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO mem_links(from_id,to_id,rel) VALUES(?,?,?)`, m.ID, lk.ID, lk.Rel); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM mem_vec WHERE id=?`, m.ID); err != nil {
		return err
	}
	if len(vec) > 0 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO mem_vec(id,dim,model,vec) VALUES(?,?,?,?)`,
			m.ID, len(vec), vecModel, encodeVec(vec)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (i *Index) Delete(ctx context.Context, id string) error {
	for _, q := range []string{
		`DELETE FROM memories WHERE id=?`,
		`DELETE FROM mem_fts WHERE id=?`,
		`DELETE FROM mem_entities WHERE memory_id=?`,
		`DELETE FROM mem_links WHERE from_id=?`,
		`DELETE FROM mem_vec WHERE id=?`,
	} {
		if _, err := i.db.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	return nil
}

// Row 检索返回的元数据行
type Row struct {
	ID, Type, Title, Project, Status, Path string
	Created                                int64
	Rel                                    string `json:",omitempty"` // 仅在 Children/Parents 查询时填充(链接的关系类型)
}

// FTSSearch 全文召回（BM25），返回 id→score
// 查询按空格拆词后 AND 组合各加引号词；trigram 分词下 <3 字的词天然不命中，由 Search 层 LIKE 兜底
func (i *Index) FTSSearch(ctx context.Context, query string, project, mtype string, limit int) (map[string]float64, error) {
	q := ftsQueryEscape(query)
	if q == "" {
		return map[string]float64{}, nil
	}
	sqlText := `SELECT m.id, bm25(mem_fts) FROM mem_fts
		JOIN memories m ON m.id = mem_fts.id
		WHERE mem_fts MATCH ? AND m.status != 'archived'`
	args := []any{q}
	if project != "" {
		sqlText += ` AND m.project = ?`
		args = append(args, project)
	}
	if mtype != "" {
		sqlText += ` AND m.type = ?`
		args = append(args, mtype)
	}
	sqlText += ` ORDER BY bm25(mem_fts) LIMIT ?`
	args = append(args, limit)

	rows, err := i.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var id string
		var score float64
		if err := rows.Scan(&id, &score); err != nil {
			return nil, err
		}
		out[id] = -score // bm25 越小越好，取负变为越大越好
	}
	return out, rows.Err()
}

// LikeSearch 兜底：短词/长中文串检索 FTS trigram 匹配不到时走 LIKE
// 长词（>12 字符、无空格的中文自然句）切成 4 字滑窗，命中 ≥ 半数窗口即算召回
func (i *Index) LikeSearch(ctx context.Context, query, project string, limit int) (map[string]float64, error) {
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return map[string]float64{}, nil
	}

	// 拆分长短词和长串滑窗
	var subClauses []string
	var args []any
	windowCounts := map[string]int{} // term -> 滑窗数
	for _, t := range terms {
		runes := []rune(t)
		if len(runes) <= 12 {
			subClauses = append(subClauses, "(title LIKE ? OR body LIKE ?)")
			args = append(args, "%"+t+"%", "%"+t+"%")
			windowCounts[t] = 1
			continue
		}
		// 长中文串: 4字滑窗,步长2
		var wins []string
		for j := 0; j+4 <= len(runes); j += 2 {
			wins = append(wins, string(runes[j:j+4]))
		}
		windowCounts[t] = len(wins)
		var ors []string
		for _, w := range wins {
			ors = append(ors, "(title LIKE ? OR body LIKE ?)")
			args = append(args, "%"+w+"%", "%"+w+"%")
		}
		subClauses = append(subClauses, "("+strings.Join(ors, " OR ")+")")
	}
	_ = windowCounts // 保留权重信息（当前实现为简单 OR，后续可升格为算分）

	sqlText := `SELECT id FROM memories WHERE status != 'archived' AND (` + strings.Join(subClauses, " AND ") + `)
		AND (?='' OR project=?) LIMIT ?`
	args = append(args, project, project, limit)

	rows, err := i.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = 0.5
	}
	return out, rows.Err()
}

// VecCandidates 返回（可能按 project 过滤后的）全部向量，Go 侧并发算余弦
func (i *Index) VecCandidates(ctx context.Context, project, mtype string) (map[string][]float32, error) {
	sqlText := `SELECT v.id, v.vec FROM mem_vec v JOIN memories m ON m.id=v.id WHERE m.status != 'archived'`
	args := []any{}
	if project != "" {
		sqlText += ` AND m.project = ?`
		args = append(args, project)
	}
	if mtype != "" {
		sqlText += ` AND m.type = ?`
		args = append(args, mtype)
	}
	rows, err := i.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]float32{}
	for rows.Next() {
		var id string
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			return nil, err
		}
		out[id] = decodeVec(blob)
	}
	return out, rows.Err()
}

// ListActive 列出活跃记忆（可按项目过滤；排除 topic 卷宗，避免卷宗套卷宗）
func (i *Index) ListActive(ctx context.Context, project string) ([]Row, error) {
	sqlText := `SELECT id,type,title,project,status,path,created FROM memories
		WHERE status='active' AND type != 'topic'`
	args := []any{}
	if project != "" {
		sqlText += ` AND project=?`
		args = append(args, project)
	}
	sqlText += ` ORDER BY created DESC LIMIT 500`
	rows, err := i.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.Type, &r.Title, &r.Project, &r.Status, &r.Path, &r.Created); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetRow 元数据读取
func (i *Index) GetRow(ctx context.Context, id string) (*Row, error) {
	r := &Row{}
	err := i.db.QueryRowContext(ctx, `SELECT id,type,title,project,status,path,created FROM memories WHERE id=?`, id).
		Scan(&r.ID, &r.Type, &r.Title, &r.Project, &r.Status, &r.Path, &r.Created)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return r, err
}

// GetBody 读取记忆正文（rerank/展示用）
func (i *Index) GetBody(ctx context.Context, id string) (string, error) {
	var b string
	err := i.db.QueryRowContext(ctx, `SELECT body FROM memories WHERE id=?`, id).Scan(&b)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return b, err
}

// ---------- 增量索引支持（文件 mtime 指纹） ----------

// ChangedSinceIndex 判断该文件相对已索引指纹是否有变化（增/改/新增）
func (i *Index) FileChanged(ctx context.Context, id string, size int64, modUnix int64) (bool, error) {
	var fp string
	err := i.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, "fp:"+id).Scan(&fp)
	if err == sql.ErrNoRows || fp != fingerprint(size, modUnix) {
		return true, nil
	}
	if err != nil {
		return true, err
	}
	return false, nil
}

func (i *Index) MarkFile(ctx context.Context, id string, size int64, modUnix int64) error {
	return i.SetMeta(ctx, "fp:"+id, fingerprint(size, modUnix))
}

// AllIDs 返回已索引的全部记忆 id（用于检测"文件已删但索引还在"）
func (i *Index) AllIDs(ctx context.Context) ([]string, error) {
	rows, err := i.db.QueryContext(ctx, `SELECT id FROM memories`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func fingerprint(size, modUnix int64) string {
	return fmt.Sprintf("%d:%d", size, modUnix)
}

// Children 返回 topic 记忆管理的子记忆（按 links 出向；按创建时间倒序）
func (i *Index) Children(ctx context.Context, id string) ([]Row, error) {
	rows, err := i.db.QueryContext(ctx, `SELECT m.id,m.type,m.title,m.project,m.status,m.path,m.created,l.rel
		FROM mem_links l JOIN memories m ON m.id = l.to_id
		WHERE l.from_id=? AND m.status != 'archived'
		ORDER BY m.created DESC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.Type, &r.Title, &r.Project, &r.Status, &r.Path, &r.Created, &r.Rel); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Parents 反向链：谁把这条记忆当作子记忆/关联（反向链接面板）
func (i *Index) Parents(ctx context.Context, id string) ([]Row, error) {
	rows, err := i.db.QueryContext(ctx, `SELECT m.id,m.type,m.title,m.project,m.status,m.path,m.created,l.rel
		FROM mem_links l JOIN memories m ON m.id = l.from_id
		WHERE l.to_id=? AND m.status != 'archived'`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.Type, &r.Title, &r.Project, &r.Status, &r.Path, &r.Created, &r.Rel); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Topics 项目下所有 topic 卷宗（按活跃优先 + 时间倒序）
func (i *Index) Topics(ctx context.Context, project string) ([]Row, error) {
	rows, err := i.db.QueryContext(ctx, `SELECT id,type,title,project,status,path,created FROM memories
		WHERE type='topic' AND status != 'archived' AND (?='' OR project=?)
		ORDER BY created DESC`, project, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.Type, &r.Title, &r.Project, &r.Status, &r.Path, &r.Created); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (i *Index) EntityBoost(ctx context.Context, query string) (map[string]float64, error) {
	rows, err := i.db.QueryContext(ctx, `SELECT memory_id, COUNT(*) c FROM mem_entities
		WHERE instr(?, entity) > 0 GROUP BY memory_id`, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]float64{}
	maxC := 1.0
	type pair struct {
		id string
		c  float64
	}
	var ps []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.id, &p.c); err != nil {
			return nil, err
		}
		if p.c > maxC {
			maxC = p.c
		}
		ps = append(ps, p)
	}
	for _, p := range ps {
		out[p.id] = p.c / maxC
	}
	return out, rows.Err()
}

// AllIDs 返回全部记忆 id（reindex 用）
func (i *Index) Count(ctx context.Context) (int, error) {
	var n int
	err := i.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memories`).Scan(&n)
	return n, err
}

func joinSpace(ss []string) string {
	out := ""
	for _, s := range ss {
		out += " " + s
	}
	return out
}

// ftsQueryEscape 查询拆词后逐词加引号（隐式 AND），防 FTS5 语法注入
func ftsQueryEscape(query string) string {
	terms := strings.Fields(query)
	quoted := make([]string, 0, len(terms))
	for _, t := range terms {
		t = strings.ReplaceAll(t, `"`, ``)
		if t != "" {
			quoted = append(quoted, `"`+t+`"`)
		}
	}
	return strings.Join(quoted, " ")
}

func encodeVec(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

func decodeVec(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}
