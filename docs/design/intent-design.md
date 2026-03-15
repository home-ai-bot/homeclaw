# HomeClaw 意图识别设计

## 一、架构概述

HomeClaw 采用**双模型分层架构**处理用户输入：

```
用户输入
    ↓
┌─────────────────────────────────────┐
│  小模型：意图分类 (Intent Classifier)  │
│  - 使用轻量模型 (phi/gemma/qwen 2B)   │
│  - 输出：intent + confidence         │
└─────────────────────────────────────┘
    ↓
分支处理：
    │
    ├──→ 【设备控制类】device.control.*
    │       ↓
    │       小模型：工作流匹配 (Workflow Matcher)
    │       ├── 匹配成功 → 直接执行工作流
    │       └── 匹配失败 → 调用大模型生成工作流 → 存储 → 执行
    │
    ├──→ 【工作流创建/修改】workflow.create.* / workflow.modify
    │       ↓
    │       调用大模型生成/修改工作流 → 用户确认 → 存储 → 执行
    │
    ├──→ 【配置类】config.* / user.* / space.* / device.add
    │       ↓
    │       大模型引导式对话 → 收集必要信息 → 执行操作
    │
    └──→ 【对话类】chat.*
            ↓
            小模型直接回复或调用大模型回复
```

---

## 二、意图分类体系

### 2.1 设备控制类意图（核心操作）

| 意图 | 示例 | 处理流程 |
|------|------|---------|
| `device.control.single` | "开灯"、"把温度调到26度" | 小模型匹配工作流 → 执行 |
| `device.control.scene` | "我要睡觉了"、"我出门了" | 小模型匹配场景工作流 → 执行 |
| `device.control.global` | "关掉所有灯"、"打开全屋空调" | 小模型匹配工作流 → 执行 |
| `device.control.correct` | "保留台灯"、"别开那盏灯" | 修正上一工作流 → 小模型调整执行 |

### 2.2 设备管理类意图

| 意图 | 示例 | 处理流程 |
|------|------|---------|
| `device.add` | "我买了个新的小米摄像头" | 小模型识别 → 触发 Skill 扫描 → 大模型辅助归属 |
| `device.scan` | "扫描一下家里的设备" | 触发局域网扫描流程 |
| `device.remove` | "删除客厅的旧灯" | 小模型确认 → 执行删除 |
| `device.rename` | "把书房的灯改名为台灯" | 小模型确认 → 更新名称 |
| `device.move` | "把走廊的灯移到客厅" | 小模型确认 → 修改归属 |
| `device.query.status` | "客厅现在多少度？" | 小模型匹配查询工作流 → 执行 |

### 2.3 空间/房间管理类意图

| 意图 | 示例 | 处理流程 |
|------|------|---------|
| `space.define` | "一楼有客厅、餐厅、厨房" | 大模型解析结构 → 存储空间树 |
| `space.rename` | "把书房改名为工作室" | 小模型确认 → 重命名 |
| `space.query` | "我家有哪些房间？" | 查询空间结构 |

### 2.4 用户管理类意图

| 意图 | 示例 | 处理流程 |
|------|------|---------|
| `user.add` | "添加我老婆为成员" | 大模型引导 → 收集信息 → 创建用户 |
| `user.remove` | "删除小孩的账号" | 小模型确认 → 删除 |
| `user.bind_channel` | "绑定我的 Telegram" | 生成验证码 → 完成绑定 |
| `user.unbind_channel` | "解绑微信" | 小模型确认 → 解绑 |
| `user.set_permission` | "给老婆开通摄像头权限" | 大模型解析 → 更新权限 |
| `user.query` | "我家有哪些成员？" | 查询用户列表 |

### 2.5 系统配置类意图

| 意图 | 示例 | 处理流程 |
|------|------|---------|
| `config.channel.add` | "配置 Telegram Bot" | 大模型引导 → 验证 → 保存 |
| `config.channel.remove` | "删除 QQ 渠道" | 小模型确认 → 删除 |
| `config.model.set` | "切换到大模型 GPT-4" | 小模型确认 → 切换配置 |
| `config.skill.enable` | "启用米家 Skill" | 触发 Skill 加载流程 |
| `config.skill.disable` | "禁用涂鸦 Skill" | 小模型确认 → 禁用 |

### 2.6 工作流管理类意图

| 意图 | 示例 | 处理流程 |
|------|------|---------|
| `workflow.create.scene` | "帮我设置睡眠场景" | 大模型生成 → 存储工作流 |
| `workflow.create.automation` | "每天晚上10点关灯" | 大模型生成定时工作流 |
| `workflow.modify` | "睡眠场景保留台灯" | 小模型匹配 → 大模型修改变体 |
| `workflow.delete` | "删除睡眠场景" | 小模型确认 → 删除 |
| `workflow.list` | "我有哪些自动化？" | 查询工作流列表 |
| `workflow.save_preference` | "记住这个偏好" | 保存个性化变体 |

### 2.7 对话/交互类意图

| 意图 | 示例 | 处理流程 |
|------|------|---------|
| `chat.greeting` | "你好"、"在吗" | 小模型直接回复 |
| `chat.help` | "你能做什么？" | 返回帮助信息 |
| `chat.confirm` | "是的"、"确认" | 继续上一流程 |
| `chat.cancel` | "算了"、"取消" | 中断当前流程 |
| `chat.clarify` | "开哪个灯？" | 追问澄清（系统主动）|

---

## 三、关键设计决策

| 问题 | 决策 |
|------|------|
| **小模型如何匹配工作流？** | 使用向量相似度：用户输入 embedding ↔ 工作流描述/别名 embedding |
| **工作流存储结构？** | 每个工作流包含：`id`, `description`, `aliases[]`, `trigger_patterns[]`, `nodes[]` |
| **如何区分"创建场景"和"执行场景"？** | 关键词触发："设置/创建/帮我" → 创建模式；"我要/开始" → 执行模式 |
| **个性化变体如何存储？** | 关联到 `(user_id, base_workflow_id)`，执行时优先匹配用户变体 |

---

## 四、处理流程详解

### 4.1 设备控制类处理流程

```
用户输入: "我要睡觉了"
    ↓
小模型意图识别 → device.control.scene (confidence: 0.95)
    ↓
小模型工作流匹配:
    - 查询工作流索引中 description/aliases 与 "睡觉" 相关的条目
    - 计算向量相似度
    - 匹配到 "睡眠场景" 工作流 (similarity: 0.92)
    ↓
存在匹配工作流？
    ├── 是 → 执行工作流 → 关闭当前房间灯
    └── 否 → 调用大模型生成工作流 → 用户确认 → 保存 → 执行
```

### 4.2 工作流创建类处理流程

```
用户输入: "帮我设置睡眠场景"
    ↓
小模型意图识别 → workflow.create.scene (confidence: 0.98)
    ↓
调用大模型生成工作流:
    - 上下文：当前房间、设备列表、用户偏好
    - 输出：工作流 JSON (关闭灯 + 保留夜灯 + 延时关闭)
    ↓
展示给用户确认 → 保存到工作流存储 → 建立索引
```

### 4.3 配置类处理流程

```
用户输入: "添加我老婆为成员"
    ↓
小模型意图识别 → user.add (confidence: 0.96)
    ↓
大模型引导式对话:
    - "请问成员的名字是？"
    - "需要绑定 Telegram/QQ 账号吗？"
    - "可以控制哪些房间？"
    ↓
收集完整信息 → 创建用户 → 设置权限
```

---

## 五、小模型 Prompt 模板

### 5.1 意图分类 Prompt

```
你是一个智能家居助手的意图识别器。请分析用户输入，从以下类别中选择最匹配的意图：

## 设备控制类
- device.control.single: 控制单个设备（开灯、调温度）
- device.control.scene: 触发场景（睡觉、出门、回家）
- device.control.global: 全局控制（所有灯、全屋空调）
- device.control.correct: 修正上一操作（保留台灯、别开那盏）

## 设备管理类
- device.add: 添加新设备
- device.scan: 扫描设备
- device.remove: 删除设备
- device.rename: 重命名设备
- device.move: 移动设备到其他房间
- device.query.status: 查询设备状态

## 空间管理类
- space.define: 定义空间结构
- space.rename: 重命名空间
- space.query: 查询空间

## 用户管理类
- user.add: 添加成员
- user.remove: 删除成员
- user.bind_channel: 绑定渠道账号
- user.unbind_channel: 解绑渠道账号
- user.set_permission: 设置权限
- user.query: 查询成员

## 配置类
- config.channel.add: 添加渠道
- config.channel.remove: 删除渠道
- config.model.set: 切换模型
- config.skill.enable: 启用 Skill
- config.skill.disable: 禁用 Skill

## 工作流管理类
- workflow.create.scene: 创建场景
- workflow.create.automation: 创建自动化
- workflow.modify: 修改工作流
- workflow.delete: 删除工作流
- workflow.list: 列出自定义
- workflow.save_preference: 保存偏好

## 对话类
- chat.greeting: 问候
- chat.help: 寻求帮助
- chat.confirm: 确认
- chat.cancel: 取消

请输出 JSON 格式：
{
    "intent": "device.control.scene",
    "confidence": 0.95,
    "entities": {
        "scene_name": "睡眠"
    }
}

用户输入: {user_input}
```

### 5.2 工作流匹配 Prompt

```
你是一个工作流匹配器。根据用户输入，从以下工作流列表中选择最匹配的一个。

可用工作流：
{workflow_list}

用户输入: {user_input}

请输出 JSON 格式：
{
    "matched": true,
    "workflow_id": "sleep_scene",
    "similarity": 0.92,
    "reason": "用户说"睡觉"与"睡眠场景"语义匹配"
}

如果没有匹配的工作流：
{
    "matched": false,
    "reason": "没有找到相关工作流"
}
```

---

## 六、与大模型交互的边界

| 场景 | 小模型处理 | 大模型处理 |
|------|-----------|-----------|
| 意图识别 | ✓ | ✗ |
| 工作流匹配 | ✓ | ✗ |
| 简单确认/取消 | ✓ | ✗ |
| 工作流生成 | ✗ | ✓ |
| 复杂对话引导 | ✗ | ✓ |
| 自然语言转结构化数据 | ✗ | ✓ |
| 设备归属推断 | 辅助 | 主导 |
| 权限解析 | 辅助 | 主导 |

---

## 七、错误处理与降级策略

1. **意图识别置信度低 (< 0.7)**：
   - 调用大模型进行二次确认
   - 或询问用户："你是想控制设备，还是管理配置？"

2. **工作流匹配置信度低 (< 0.8)**：
   - 调用大模型理解意图
   - 大模型决定是执行现有工作流还是创建新工作流

3. **小模型不可用**：
   - 直接路由到大模型处理
   - 大模型完成完整的意图识别和处理

4. **大模型不可用**：
   - 小模型继续处理已知意图
   - 对于需要大模型的意图，返回友好提示："当前无法处理复杂请求，请稍后再试"
