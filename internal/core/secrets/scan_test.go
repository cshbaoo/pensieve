package secrets

import (
	"strings"
	"testing"
)

func TestScanHits(t *testing.T) {
	cases := []struct {
		name, text, expectRule string
	}{
		{"openai key", "用这个 key: sk-abc123def456ghi789jkl0mnop", "openai-style-key"},
		{"openai project key", "OPENAI key: sk-proj-AbCd1234efgh5678ijkl9012mnop", "openai-project-key"},
		{"anthropic key", "claude key: sk-ant-api03-AbCd1234efgh5678ijkl9012mnop", "anthropic-key"},
		{"google api key", "gcp key: AIzaSyAbCdEfGhIjKlMnOpQrStUvWxYz0123456", "google-api-key"},
		{"slack bot token", "SLACK_TOKEN=xoxb-1234567890-abcdefghijkl", "slack-token"},
		{"npm token", "token: npm_1234567890abcdefghijklmnopqrstu", "npm-token"},
		{"pypi token", "token: pypi-AgEIcHlwaS5vcmcCJGEyZmFiY2RlZg1234", "pypi-token"},
		{"stripe live", "STRIPE_KEY=sk_live_4eC39HqLyjWDarjtT1zdp7dc", "stripe-live-key"},
		{"telegram bot", "bot = 110201543:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaws", "telegram-bot-token"},
		{"aws secret assign", `AWS_SECRET_ACCESS_KEY = "wJalrXUtnFEMIK7MDENGbPxRfiCYEXAMPLEKEY"`, "aws-secret-assignment"},
		{"aws key", "AKIAIOSFODNN7EXAMPLE", "aws-access-key"},
		{"bearer", "Authorization: Bearer abcdefgh12345678abcd", "bearer-token"},
		{"private key", "-----BEGIN RSA PRIVATE KEY-----", "private-key-block"},
		{"github pat", "ghp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "github-token"},
		{"gitlab pat", "glpat-abcdefghijklmnop12", "gitlab-token"},
		{"password assign", `db password = "supersecret123"`, "password-assignment"},
		{"mongo conn", "mongodb://user:pass123@host:27017/db", "mongo-conn-string"},
		{"jwt token", "签名结果: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U", "jwt"},
		{"github fine-grained", "token=github_pat_11ABCDEFG0xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", "github-fine-grained"},
		{"postgres conn", "postgresql://app:SeCrEt9876@db.internal:5432/app", "postgres-conn-string"},
		{"postgres short scheme", "postgres://u:p@sss4567890@host/db", "postgres-conn-string"},
		{"redis conn", "redis://default:weakpass999@redis:6379", "redis-conn-string"},
		{"amqp conn", "amqps://svc:mqpass12345@mq.example.com/vhost", "amqp-conn-string"},
		{"slack webhook", "回调地址 https://hooks.slack.com/services/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", "slack-webhook"},
		{"discord webhook", "url=https://discord.com/api/webhooks/1234567890123456789/AbCdEfGhIjKlMnOpQrStUvWxYz", "discord-webhook"},
		{"feishu webhook", "https://open.feishu.cn/open-apis/bot/v2/hook/27be29e4-bb33-4fcf-9514-19e2fc408655", "feishu-webhook"},
		{"generic conn creds", "kafka://user:Sup3rLongPwd1@broker:9092/topic", "generic-conn-with-creds"},
		{"unquoted k8s style", `api_token: dGhpcy1pcy1hLWJhc2U2NC1zZWNyZXR2YWx1ZQ==`, "generic-secret-assignment"},
	}
	for _, c := range cases {
		hits := Scan(c.text)
		found := false
		for _, h := range hits {
			if h.Rule == c.expectRule {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: 应命中 %s, 实际 %+v", c.name, c.expectRule, hits)
		}
	}
}

func TestScanClean(t *testing.T) {
	clean := []string{
		"接口返回的 cpu 单位是核,不是毫核",
		"BuildQueueName(ns, group) 用于生成队列名",
		"token 过期后需要重新登录",
		"password 字段在文档里讨论,不含实际值", // 没值不应命中
		"my tokens are short",
		"pk_live_4eC39HqLyjWDarjtT1zdp7dc 是 Stripe 公钥,可公开", // 公钥不算泄露
		`api_key = "${MY_ENV_VAR}"`,                        // 环境变量占位符不算泄露(env 展开写法)
		"2026-08-14 10:00 会议时长 35 分钟",                      // 时间戳冒号不应触发 telegram 规则
		"版本号 2602.11988 与 2504.13171 是 arXiv 论文号",          // 纯数字串不触发
	}
	for _, s := range clean {
		if hits := Scan(s); len(hits) > 0 {
			t.Errorf("误报: %q -> %+v", s, hits)
		}
	}
}

// 脱敏:命中行的 snippet 不得回显密钥本体(错误信息会进 agent 上下文/日志,本身就是泄露通道)
func TestSnippetRedacted(t *testing.T) {
	hits := Scan("key: sk-proj-SECRETVALUE1234567890abcd")
	if len(hits) == 0 {
		t.Fatal("应命中")
	}
	for _, h := range hits {
		if strings.Contains(h.Snippet, "sk-proj-SECRETVALUE") {
			t.Errorf("snippet 泄露密钥本体: %q", h.Snippet)
		}
		if !strings.Contains(h.Snippet, "***") {
			t.Errorf("snippet 应有脱敏标记: %q", h.Snippet)
		}
	}
	// Err 文案同样不得含原文
	if e := Err(hits); strings.Contains(e.Error(), "SECRETVALUE") {
		t.Errorf("错误文案泄露密钥: %v", e)
	}
}

// ScanParts:密钥藏在非正文字段(tags/entities/锚点)也必须命中
func TestScanPartsCoversAllFields(t *testing.T) {
	hits := ScanParts("正常标题", "正常正文", "normal-tag", "锚点 glpat-secret1234567890abcdef 在某处")
	found := false
	for _, h := range hits {
		if h.Rule == "gitlab-token" {
			found = true
		}
	}
	if !found {
		t.Fatalf("锚点里的密钥应被命中: %+v", hits)
	}
}

func TestScanMultiLine(t *testing.T) {
	text := "第一行干净\n第二行 sk-0123456789abcdefghijklmn 有密钥\n第三行干净"
	hits := Scan(text)
	if len(hits) == 0 {
		t.Fatal("应命中")
	}
	if hits[0].Line != 2 {
		t.Fatalf("行号不对: %d", hits[0].Line)
	}
	if e := Err(hits); !strings.Contains(e.Error(), "第2行") {
		t.Fatalf("错误文案应含行号: %v", e)
	}
}

func BenchmarkScan(b *testing.B) {
	text := strings.Repeat("这是一段正常的工程记忆正文,讨论接口行为与调用方式。\n", 100) +
		"api_key = \"abcdef1234567890xyz\"\n" +
		strings.Repeat("normal line with urls like https://example.com/docs and no creds\n", 100)
	b.ReportAllocs()
	for b.Loop() {
		_ = Scan(text)
	}
}
