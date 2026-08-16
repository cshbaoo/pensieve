package extract

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/cshbaoo/pensieve/internal/llm"
)

// Draft LLM 提炼出的记忆草稿
type Draft struct {
	Title    string    `json:"title"`
	Type     string    `json:"type"`
	Tags     []string  `json:"tags"`
	Entities []string  `json:"entities"`
	Anchors  []string  `json:"anchors"`
	Links    []DrfLink `json:"links"`
	Body     string    `json:"body"`
}

// DrfLink 提炼草稿中的关联链接
type DrfLink struct {
	ID  string `json:"id"`
	Rel string `json:"rel,omitempty"`
}

// coding pack 的提炼提示词(未来迁移到 packs/coding/extract.prompt)
// typeProfiles:每种类型的 body 骨架——类型策略表第一块(检索权重/保鲜策略随后)
// 设计意图:同一份存储,不同的"形态要求",让每种知识以最适合被复用的结构入库。
const typeProfiles = `各类型 body 骨架(选定 type 后按对应结构组织 body,缺失维度就省略,不造内容):
- api:        定位(文件/接口在哪) → 行为与参数(含单位/边界值) → 注意点与坑
- bugfix:     症状(用户侧的报错/现象) → 根因(因果链,不许直接跳结论) → 修法 → 防复发做法
- gotcha:     坑是什么(一句话场景) → 为什么容易被踩(认知差在哪) → 正确姿势
- decision:   背景与约束 → 决策内容 → 推论与代价(为什么不是别的选项)
- snippet:    使用条件(何时该用它) → 代码块(完整可粘贴) → 常见误用
- pattern:    约定内容 → 适用场景 → 反例(什么时候不该这么做)
- requirement: 目标(一句话) → 验收标准(可验证) → 范围外(明确不做什么)
- topic:      卷宗范围说明 + 收录标准(什么记忆该进本卷宗)
`

const codingPrompt = `你是编程团队的知识管理员。从下面提供的会话/文本内容中，提取最值得长期保留的工程经验（最多 1 条，宁缺毋滥）。

只允许这些类型:
- requirement: 产品需求/PRD 事实(需求目标、重要背景、验收标准、交付范围)
- decision: 技术决策及理由
- bugfix: 已定位/修复 bug 的根因(必须讲清因果链)
- gotcha: 容易再踩的坑
- api: 外部系统/接口的准确细节(参数/单位/行为)
- pattern: 项目约定/模式/协作纪律
- snippet: 可复用代码片段
- topic: 汇总多条已有记忆的"卷宗/索引页"(输入若是在整理归类多条已知记忆时使用)

` + typeProfiles + `

links 字段: 若该记忆与其他记忆有明确关联,输出带 rel 的对象,
rel 取值为: contains(卷宗收编子记忆) / implements(实现/落地某需求或方案) /
related(同主题平行关联) / superseded-by / caused-by / fixes。
如果不确定就先用 related。

规则:
1. 只在内容确实包含可复用的工程价值时提取;纯寒暄/一次性操作不值得记,此时输出 {"skip": true}
2. 事实必须来自原文,禁止推测编造
3. body 用 Markdown,结构清晰,保留关键文件路径与行号(形如 path/to/file.go:123)
4. 【重要】必须完整保留原文中的所有外部引用:飞书/文档 token、wiki/docx 链接、commit hash、MR/issue 编号——可溯源性是记忆可信度的根
5. anchors: 相关代码文件路径(可带行号)
6. entities: 涉及的接口名/函数名/文件名/系统名等可被精确匹配的标识符
7. tags: 2-5 个中文或英文关键词

输出严格 JSON(不要输出任何其他内容,不要用 markdown 代码块包裹):
{"title":"一句话标题","type":"...","tags":[...],"entities":[...],"anchors":[...],"links":[{"id":"mem_xx","rel":"related"}],"body":"..."}
(可不输出 links 字段;若不确定能不能链,宁缺)`

var jsonBlock = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\})\\s*```")

// Extract 从输入文本提炼记忆草稿
func Extract(ctx context.Context, client *llm.Client, model, input string) (*Draft, bool, error) {
	out, err := client.Chat(ctx, model, codingPrompt, input)
	if err != nil {
		return nil, false, err
	}
	out = strings.TrimSpace(out)
	if m := jsonBlock.FindStringSubmatch(out); m != nil {
		out = m[1]
	}
	// 容错:截取第一个 { 到最后一个 }
	lo := strings.Index(out, "{")
	hi := strings.LastIndex(out, "}")
	if lo < 0 || hi <= lo {
		return nil, false, fmt.Errorf("LLM 未返回 JSON: %s", trunc(out, 200))
	}
	out = out[lo : hi+1]

	var d struct {
		Draft
		Skip bool `json:"skip"`
	}
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		return nil, false, fmt.Errorf("LLM JSON 解析失败: %w", err)
	}
	if d.Skip {
		return nil, true, nil
	}
	if d.Draft.Title == "" || d.Draft.Body == "" {
		return nil, false, fmt.Errorf("LLM 返回缺 title/body")
	}
	if d.Draft.Type == "" {
		d.Draft.Type = "pattern"
	}
	return &d.Draft, false, nil
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
