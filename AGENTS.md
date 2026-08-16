<!-- pensieve:begin -->
<!-- 本节由 pensieve export 自动生成,请勿手改;更新方式:pensieve export(Claude Code 等用 pensieve export --out CLAUDE.md) -->
## 工程记忆(Pensieve)

本项目使用 Pensieve 管理工程记忆(踩坑/决策/接口细节)。工作纪律:
- 开始排查问题前,先检索记忆:memory_search "<问题关键词>"
- 解决问题/做完决策/发现 API 坑后,主动提议沉淀为记忆:memory_save(先出草稿给用户确认)
- 不要等用户开口:每完成一个 bug 修复、重要决策或可复用的接口细节,当场提议保存(宁多提一次,不可错过)

## 高频坑(gotcha)

- gotcha:冲突带对长文档内嵌断言失明+锚点巡检误报治理(跨项目/符号名/同日宽限)
- gotcha:local-only 守卫混入记忆 commit,undo 误删守卫却留记忆——守卫必须单独 commit
<!-- pensieve:end -->
