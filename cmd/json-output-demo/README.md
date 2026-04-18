# `json-output-demo`

演示 Agora 的 provider-neutral JSON Output：`llm.ResponseFormat{Type: llm.ResponseFormatJSONObject}`。

## DeepSeek reasoning JSON mode

```bash
go run ./cmd/json-output-demo \
  -provider deepseek \
  -model deepseek-reasoner \
  -thinking-mode on \
  -reasoning-mode native \
  -response-format json_object
```

## DeepSeek chat JSON mode

```bash
go run ./cmd/json-output-demo \
  -provider deepseek \
  -model deepseek-chat \
  -thinking-mode off \
  -response-format json_object
```

## Notes

- Prompt 中应包含 `json` 字样和期望 JSON 示例，这是 DeepSeek JSON Output 的官方要求。
- JSON Output 返回的是普通文本，demo 会尝试解析并 pretty-print。
- 真实工具执行仍建议使用 tool-calling；planning / extraction / schema-only 阶段更适合 JSON Output。

