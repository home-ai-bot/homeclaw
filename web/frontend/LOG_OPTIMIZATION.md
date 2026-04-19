# 通信日志优化总结

## 问题

1. **日志过多**：点击 turn-on/turn-off 按钮时产生 3 条日志
   - 第 1 条：工具调用日志（紫色）
   - 第 2 条：成功日志（绿色）
   - 第 3 条：Agent 回复日志（绿色）

2. **颜色误导**：所有 error 类型的日志都用红色，但有些只是警告

## 优化方案

### 1. 简化日志记录逻辑

**优化前**：
```typescript
// executeToolCall 会记录 2 条日志
addLog({ type: "tool", title: "调用: hc_cli.exe" })  // 调用时
addLog({ type: "receive", title: "成功: hc_cli.exe" })  // 成功时
// 或者
addLog({ type: "error", title: "失败: hc_cli.exe" })  // 失败时
```

**优化后**：
```typescript
// 只在失败时记录到通信面板
if (!result.success) {
  addLog({
    type: "error",
    title: "工具调用失败: hc_cli.exe",
    content: result.error,
  })
}
```

**原因**：
- 设备控制页面有自己的操作日志面板（页面底部）
- 成功操作已在操作日志中显示
- 通信面板只记录异常情况，避免重复

### 2. 优化日志颜色逻辑

**优化前**：
```typescript
const typeColor =
  log.type === "error" ? "text-destructive"  // 所有 error 都是红色
  : ...
```

**优化后**：
```typescript
const typeColor =
  log.type === "send" ? "text-blue-600"
  : log.type === "receive" ? "text-green-600"
  : log.type === "tool" ? "text-purple-600"
  : log.title.includes("失败") || log.title.includes("错误")
    ? "text-destructive"  // 只有真正的错误才用红色
    : "text-yellow-600"   // 其他 warning 用黄色
```

**颜色方案**：
- 🔵 **蓝色** - 发送的消息
- 🟢 **绿色** - 接收的响应/成功
- 🟣 **紫色** - 工具调用
- 🔴 **红色** - 真正的错误（标题包含"失败"或"错误"）
- 🟡 **黄色** - 其他警告/提示信息

## 优化效果

### 设备控制页面（turn-on/turn-off）

**优化前**（3 条日志）：
```
🟣 调用: hc_cli.exe
🟢 成功: hc_cli.exe
🟢 Agent 回复: 设备已开启
```

**优化后**（0-1 条日志）：
```
（成功时不记录到通信面板，只在操作日志面板显示）
或
🔴 工具调用失败: hc_cli.exe（仅在失败时显示）
```

### 其他页面（小米、涂鸦等）

**登录操作**：
```
（成功时不记录）
🔴 工具调用失败: xiaomi.login（失败时显示）
```

**长时间操作（fire-and-forget）**：
```
（不等待响应，不记录）
```

## 使用建议

### 场景 1：快速操作（turn-on/turn-off）
```typescript
// 使用 executeToolCall，成功时不记录日志
await executeToolCall({
  toolName: "hc_cli",
  method: "exe",
  brand: "xiaomi",
  params: { from_id: "123", from: "xiaomi", ops: "turn_on" }
})
// 页面自己的操作日志面板会显示结果
```

### 场景 2：需要详细日志的调试场景
```typescript
// 使用 sendWebSocketMessage 手动记录
sendWebSocketMessage({
  type: "message.send",
  payload: { content: "tool:hc_cli {...}" }
})
```

### 场景 3：长时间操作
```typescript
// 使用 fire-and-forget 模式
await executeToolCall({
  toolName: "hc_cli",
  method: "generate_ops",
  brand: "tuya",
  params: { from_id: "456", from: "tuya" }
}, {
  waitForResponse: false,
  successMessage: "已发送请求"
})
```

## 文件修改清单

1. ✅ `use-smart-home-websocket.ts` - 简化 executeToolCall 日志逻辑
2. ✅ `smart-home-layout.tsx` - 优化日志颜色判断逻辑

## 测试验证

```bash
cd g:\code\homeclaw
.\scripts\build-windows.ps1
```

编译成功，无 TypeScript 错误。
