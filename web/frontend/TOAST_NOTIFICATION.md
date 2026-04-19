# 设备操作 Toast 提示功能

## 功能概述

现在当你在设备控制页面点击 turn-on、turn-off 等按钮时，操作完成后会在页面右上角显示一个绿色的成功提示。

## 效果展示

### 操作成功
```
┌─────────────────────────────────┐
│  ✓  客厅灯 turn_on 成功         │  ← 右上角绿色提示，3秒后自动消失
└─────────────────────────────────┘
```

### 操作失败
```
┌─────────────────────────────────┐
│  ✗  设备控制失败: 连接超时      │  ← 右上角红色提示
└─────────────────────────────────┘
```

## 实现细节

### 1. 修改文件

**`device-control-page.tsx`**
```typescript
// 使用 callTool 替代 executeDeviceOperation
const result = await callTool(
  {
    toolName: "hc_cli",
    method: "exe",
    brand: from,
    params: {
      from_id: fromId,
      from: from,
      ops: opsName,
    },
  },
  {
    timeout: 60000,
    successMessage: `${deviceName} ${opsName} 成功`,  // ← 这里设置提示消息
  }
)
```

**`device-command-executor.ts`**
```typescript
// 集成 sonner toast 库
import { toast } from "sonner"

function showTooltip(message: string): void {
  toast.success(message, {
    duration: 3000,        // 3秒后自动消失
    position: "top-right", // 右上角显示
  })
}
```

### 2. Toast 配置

使用项目已有的 `sonner` 库，配置如下：

| 属性 | 值 | 说明 |
|------|-----|------|
| duration | 3000ms | 提示持续时间 |
| position | top-right | 显示位置 |
| type | success | 成功提示（绿色） |
| type | error | 错误提示（红色） |

### 3. 工作流程

```
用户点击按钮
    ↓
显示"正在执行..."日志
    ↓
发送 WebSocket 命令
    ↓
等待响应（最长60秒）
    ↓
成功？
├─ 是 → 显示 Toast: "设备名 操作名 成功" ✓
└─ 否 → 显示 Toast: "错误信息" ✗
    ↓
更新操作日志面板
```

## 使用示例

### 示例 1：设备控制（turn-on/turn-off）

```typescript
// 点击 turn_on 按钮
await callTool(
  {
    toolName: "hc_cli",
    method: "exe",
    brand: "xiaomi",
    params: {
      from_id: "device_123",
      from: "xiaomi",
      ops: "turn_on",
    },
  },
  {
    successMessage: "客厅灯 turn_on 成功",
  }
)
// 显示: ✓ 客厅灯 turn_on 成功
```

### 示例 2：标记不可操作

```typescript
await markDeviceAsNoAction({ from_id: "123", from: "xiaomi" })
// 操作日志显示: "已标记为不可操作"
```

### 示例 3：生成操作（Fire-and-Forget）

```typescript
await callTool(
  {
    toolName: "hc_cli",
    method: "generate_ops",
    brand: "tuya",
    params: {
      from_id: "device_456",
      from: "tuya",
    },
  },
  {
    waitForResponse: false,  // 不等待响应
    successMessage: "已发送生成请求",
  }
)
// 立即显示: ✓ 已发送生成请求
```

## 与其他日志的关系

### 三个不同的反馈机制

1. **Toast 提示**（右上角）
   - ✅ 即时反馈
   - ⏱️ 3秒后自动消失
   - 📍 显示操作结果（成功/失败）

2. **操作日志面板**（页面底部）
   - 📋 持久记录
   - 📝 显示详细执行过程
   - 🔄 可以清空

3. **通信记录面板**（右侧边栏）
   - 🔍 调试用途
   - ⚠️ 只显示失败情况
   - 📊 显示完整通信内容

### 实际效果

点击 turn-on 后：
```
1. [Toast] ✓ 客厅灯 turn_on 成功  ← 右上角，3秒消失
2. [操作日志] 客厅灯 · turn_on · 执行成功  ← 页面底部，持久显示
3. [通信记录] （无记录，因为成功了）  ← 右侧边栏，失败才显示
```

## 自定义 Toast

如果需要自定义 Toast 样式或行为，可以修改 `device-command-executor.ts` 中的 `showTooltip` 函数：

```typescript
function showTooltip(message: string): void {
  toast.success(message, {
    duration: 5000,           // 改为5秒
    position: "top-center",   // 改为顶部居中
    description: "详细描述",  // 添加描述
    action: {                 // 添加操作按钮
      label: "撤销",
      onClick: () => console.log("撤销"),
    },
  })
}
```

## 错误处理

### 网络错误
```typescript
// WebSocket 连接失败
toast.error("WebSocket 连接失败", {
  description: "请检查网络连接",
})
```

### 超时错误
```typescript
// 操作超时（>60秒）
toast.error("操作超时", {
  description: "设备响应时间过长，请稍后重试",
})
```

### Agent 错误
```typescript
// Agent 返回错误
toast.error(result.error, {
  description: "工具调用失败",
})
```

## 测试验证

```bash
# 1. 编译检查
cd g:\code\homeclaw\web\frontend
npx tsc -b

# 2. 完整构建
cd g:\code\homeclaw
.\scripts\build-windows.ps1

# 3. 运行测试
# - 打开设备控制页面
# - 点击任意设备的 turn_on 按钮
# - 观察右上角是否显示绿色提示
```

## 技术栈

- **Toast 库**: [sonner](https://sonner.emilkowal.ski/)
- **位置**: 已集成到项目中（`web/frontend/package.json`）
- **样式**: 自动适配项目主题（light/dark mode）

## 注意事项

1. **Toast 只在操作完成后显示**
   - 发送命令时不显示
   - 等待响应时不显示
   - 只有收到结果后才显示

2. **Fire-and-Forget 模式**
   - 设置 `waitForResponse: false`
   - 立即显示成功提示
   - 不等待实际执行结果

3. **国际化支持**
   - 提示消息可以使用 `t()` 函数国际化
   - 例如：`successMessage: t("device.operation.success")`

## 后续优化建议

1. **添加音效**
   ```typescript
   toast.success(message, {
     icon: "🔔",
   })
   ```

2. **批量操作汇总**
   ```typescript
   // 多个设备同时操作时，显示汇总提示
   toast.success("3个设备操作成功")
   ```

3. **操作撤销**
   ```typescript
   toast.success("设备已关闭", {
     action: {
       label: "撤销",
       onClick: () => turnOn(deviceId),
     },
   })
   ```
