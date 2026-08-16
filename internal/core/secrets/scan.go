package secrets

import (
	"fmt"
	"regexp"
	"strings"
)

// Hit 敏感信息命中
type Hit struct {
	Rule    string `json:"rule"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

var rules = []struct {
	name string
	re   *regexp.Regexp
}{
	{"aws-access-key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"aws-secret-assignment", regexp.MustCompile(`(?i)aws_secret_access_key\s*[:=]\s*["']?[A-Za-z0-9/+=]{30,}`)}, // generic 规则因 key 名含 _access_key 断裂而漏,单独成条
	{"openai-style-key", regexp.MustCompile(`sk-[a-zA-Z0-9]{20,}`)},
	{"openai-project-key", regexp.MustCompile(`sk-proj-[A-Za-z0-9_-]{20,}`)}, // OpenAI 项目级 key,sk- 旧规则因含连字符匹配不到
	{"anthropic-key", regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]{20,}`)},
	{"google-api-key", regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`)},
	{"slack-token", regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`)}, // xoxb/xoxp/xoxa/xoxr 用户与 bot token
	{"npm-token", regexp.MustCompile(`npm_[A-Za-z0-9]{30,}`)},
	{"pypi-token", regexp.MustCompile(`pypi-[A-Za-z0-9_-]{30,}`)},
	{"stripe-live-key", regexp.MustCompile(`(sk|rk)_live_[A-Za-z0-9]{16,}`)}, // pk_live 是公钥不算泄露,跳过
	{"telegram-bot-token", regexp.MustCompile(`\d{8,10}:[A-Za-z0-9_-]{35}`)},
	{"bearer-token", regexp.MustCompile(`(?i)bearer\s+[a-zA-Z0-9\-\._~+/]{16,}`)},
	{"private-key-block", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"github-token", regexp.MustCompile(`gh[pousr]_[a-zA-Z0-9]{30,}`)},
	{"github-fine-grained", regexp.MustCompile(`github_pat_[a-zA-Z0-9_]{30,}`)},
	{"gitlab-token", regexp.MustCompile(`glpat-[a-zA-Z0-9\-_]{15,}`)},
	{"jwt", regexp.MustCompile(`eyJ[a-zA-Z0-9_-]{10,}\.[a-zA-Z0-9_-]{10,}\.[a-zA-Z0-9_-]{8,}`)},
	{"password-assignment", regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*["']?\S{6,}`)},
	{"generic-secret-assignment", regexp.MustCompile(`(?i)(secret|api[_-]?key|token)\s*[:=]\s*["']?[A-Za-z0-9\-._~+/]{12,}["']?`)},
	{"mongo-conn-string", regexp.MustCompile(`mongodb(\+srv)?://\S+:\S+@`)},
	{"mysql-conn-string", regexp.MustCompile(`mysql://\S+:\S+@`)},
	{"postgres-conn-string", regexp.MustCompile(`postgres(ql)?://\S+:\S+@`)},
	{"redis-conn-string", regexp.MustCompile(`redis://\S+:\S+@`)},
	{"amqp-conn-string", regexp.MustCompile(`amqps?://\S+:\S+@`)},
	{"slack-webhook", regexp.MustCompile(`hooks\.slack\.com/services/T[a-zA-Z0-9]{5,}/B[a-zA-Z0-9]{5,}/[a-zA-Z0-9]+`)},
	{"discord-webhook", regexp.MustCompile(`discord(app)?\.com/api/webhooks/\d{10,}/[a-zA-Z0-9_-]+`)},
	{"feishu-webhook", regexp.MustCompile(`open\.feishu\.cn/open-apis/bot/v2/hook/[a-f0-9-]{20,}`)},
	{"generic-conn-with-creds", regexp.MustCompile(`[a-z][a-z0-9+.-]{2,}://[^\s/:@]{2,}:[^\s/@]{4,}@[a-zA-Z0-9.-]`)},
}

// ScanParts 扫描任意多个文本片段(标题/正文/tags/entities/anchors...),合并返回全部命中。
// 密钥可能藏在任何字段(锚点/实体名都可能是连接串),写入前所有用户提供字段必须全覆盖。
func ScanParts(parts ...string) []Hit {
	return Scan(strings.Join(parts, "\n"))
}

// Scan 扫描文本，返回全部命中（空 = 干净）。
// Snippet 中命中的密钥本体一律脱敏为 ***——ECC 硬化原则:ack 不回显敏感原文,
// 否则错误信息/日志反而变成新的泄露通道。
func Scan(text string) []Hit {
	var hits []Hit
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		for _, r := range rules {
			if m := r.re.FindString(line); m != "" {
				snippet := r.re.ReplaceAllString(line, "***")
				if len(snippet) > 100 {
					snippet = snippet[:100] + "..."
				}
				hits = append(hits, Hit{Rule: r.name, Line: i + 1, Snippet: strings.TrimSpace(snippet)})
			}
		}
	}
	return hits
}

// Err 便捷错误格式
func Err(hits []Hit) error {
	var sb strings.Builder
	sb.WriteString("检测到敏感信息,拒绝写入:\n")
	for _, h := range hits {
		fmt.Fprintf(&sb, "  - 第%d行 [%s]: %s\n", h.Line, h.Rule, h.Snippet)
	}
	sb.WriteString("请脱敏后重试。")
	return fmt.Errorf("%s", sb.String())
}
