package memory

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Anchor 通用锚点：内核不理解 Kind 的语义，由 DomainPack 解释
type Anchor struct {
	Kind   string `yaml:"kind"`
	Target string `yaml:"target"`
}

// Link 带类型的记忆间关系。
// rel 推荐词表: contains(卷宗→子记忆) / part-of(子→卷宗) / related / supersedes(新替旧) / superseded-by / caused-by / fixes
type Link struct {
	ID  string `yaml:"id"`
	Rel string `yaml:"rel,omitempty"`
}

// Links 支持两种 YAML 形态,向后兼容旧的纯 id 列表：
//
//	links: [mem_a, mem_b]                （旧,rel=""）
//	links: [{id: mem_a, rel: contains}]  （新）
type Links []Link

func (l *Links) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("links 必须是数组")
	}
	var out Links
	for _, node := range value.Content {
		switch node.Kind {
		case yaml.ScalarNode:
			out = append(out, Link{ID: node.Value})
		case yaml.MappingNode:
			var lk Link
			if err := node.Decode(&lk); err != nil {
				return err
			}
			out = append(out, lk)
		default:
			return fmt.Errorf("links 元素只能是 id 字符串或 {id,rel} 对象")
		}
	}
	*l = out
	return nil
}

// MarshalYAML rel 为空时仍输出标量形态,保持文件干净
func (l Links) MarshalYAML() (any, error) {
	seq := &yaml.Node{Kind: yaml.SequenceNode}
	for _, lk := range l {
		if lk.Rel == "" {
			seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: lk.ID})
		} else {
			m := &yaml.Node{Kind: yaml.MappingNode}
			m.Content = append(m.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "id"}, &yaml.Node{Kind: yaml.ScalarNode, Value: lk.ID},
				&yaml.Node{Kind: yaml.ScalarNode, Value: "rel"}, &yaml.Node{Kind: yaml.ScalarNode, Value: lk.Rel})
			seq.Content = append(seq.Content, m)
		}
	}
	return seq, nil
}

// IDs 便捷取纯 id 列表
func (l Links) IDs() []string {
	out := make([]string, 0, len(l))
	for _, lk := range l {
		out = append(out, lk.ID)
	}
	return out
}

// SuccessorID 若本记忆已被取代,返回继任者 id(superseded-by link);否则空串
func SuccessorID(m *Memory) string {
	for _, lk := range m.Links {
		if lk.Rel == "superseded-by" && lk.ID != "" {
			return lk.ID
		}
	}
	return ""
}

// FreshnessBanner 读取侧兜底横幅:命中过时记忆时先于正文呈现,防止漂移结论被采信。
// succ 为继任者(可为 nil)。stale/非 superseded 的也给出轻提示。
func FreshnessBanner(m *Memory, succTitle, succCreated string) string {
	switch m.Status {
	case "superseded":
		b := "> ⚠️ 本记忆已被取代,结论可能已过期——请以继任者为准。\n"
		if succTitle != "" {
			b += "> 继任者: " + succTitle
			if succCreated != "" {
				b += " (" + succCreated + ")"
			}
			b += "\n"
		}
		return b + "> 以下为历史内容,仅供溯源,勿直接采信。\n\n"
	case "stale":
		return "> ⚠️ 本记忆已标记 stale(锚点失活/可能过期),请复核后再采信。\n\n"
	case "archived":
		return "> 🗄 本记忆已归档,不参与检索召回。\n\n"
	}
	return ""
}

// Memory 一条记忆（Markdown frontmatter + 正文）
type Memory struct {
	ID          string    `yaml:"id"`
	Type        string    `yaml:"type"`
	Title       string    `yaml:"title"`
	Project     string    `yaml:"project"`
	Tags        []string  `yaml:"tags,omitempty"`
	Status      string    `yaml:"status"`
	Confidence  string    `yaml:"confidence"`
	Entities    []string  `yaml:"entities,omitempty"`
	Anchors     []Anchor  `yaml:"anchors,omitempty"`
	Source      string    `yaml:"source"`
	Links       Links     `yaml:"links,omitempty"`
	Votes       int       `yaml:"votes"`
	Sensitivity string    `yaml:"sensitivity"`
	Created     time.Time `yaml:"created"`

	Body string `yaml:"-"` // frontmatter 之后的 Markdown 正文

	// Path 为文件在仓库内的相对路径（仅内存中使用）
	Path string `yaml:"-"`
}

// NewID 生成全局唯一记忆 ID：mem_YYYYMMDD_<4位hash>
func NewID(title string, t time.Time) string {
	h := md5.Sum([]byte(fmt.Sprintf("%s|%d", title, t.UnixNano())))
	return fmt.Sprintf("mem_%s_%s", t.Format("20060102"), hex.EncodeToString(h[:])[:4])
}
