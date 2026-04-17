# `toolcall-demo`

一个独立的 structured tool-call 演示入口，不再往 `cmd/stream` 里塞逻辑。

## 适合验证什么

- `ToolCallPolicy` 的 `auto / required / tool / none`
- `StrictSchema` 对各家模型的约束效果
- malformed / drifted arguments 触发后的 repair loop
- 不同 provider / model / base URL 下的实际表现

## 快速运行

```bash
go run ./cmd/toolcall-demo \
  -provider claude \
  -model claude-sonnet-4-6-n \
  -base-url https://api.ccodezh.com \
  -tool-choice tool \
  -tool-name lookup_weather \
  -strict-schema=true \
  -disable-parallel=true \
  -max-repair-attempts 1
```

## 常见测试方式

### 1) 强制某个工具

```bash
go run ./cmd/toolcall-demo \
  -provider deepseek \
  -tool-choice tool \
  -tool-name create_support_ticket \
  -task "请创建一个支持工单：标题是“Claude structured output 偶发失败”，severity=high，owner=agora-core，notify=true，并补上 tags=structured,tool-call。你必须先调用工具。"
```

### 2) 仅要求必须调用工具，但不限定具体工具

```bash
go run ./cmd/toolcall-demo \
  -provider minimax \
  -tool-choice required
```

### 3) 对比关闭工具

```bash
go run ./cmd/toolcall-demo \
  -provider gpt \
  -tool-choice none
```

## 输出里重点看什么

- `[llm call N] phase=repair`：说明框架触发了隐藏修复轮次
- `tool[...] raw=...`：模型原始 tool arguments
- `=== Stored History ===`：写回 memory 的最终结构化结果
- `=== Tool Results ===`：真正执行过的工具结果

## VS Code

已经补了一个新的 launch 配置：`toolcall demo (select provider)`。

