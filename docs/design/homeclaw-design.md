# HomeClaw 详细设计文档

## 一、项目结构

基于 PicoClaw fork，核心代码不改动，新增以下目录：

```
homeclaw/
├── cmd/
│   └── homeclaw/
│       └── main.go              # 主入口
├── pkg/
│   ├── homeclaw/
│   │   ├── data/
│   │   │   ├── space.go         # 空间数据访问层
│   │   │   ├── device.go        # 设备注册表数据访问层
│   │   │   └── member.go        # 家庭成员数据访问层
│   │   ├── workflow/
│   │   │   ├── engine.go        # 工作流执行引擎
│   │   │   ├── trigger.go       # 触发源管理（对话/事件/定时）
│   │   │   ├── loader.go        # 工作流定义加载器
│   │   │   └── types.go         # 工作流数据结构定义
│   │   └── router/
│   │       └── router.go        # 小模型意图路由
│   └── skills/
│       └── homeclaw/
│           ├── skill.go         # Skill 统一接口定义
│           ├── mijia/           # 米家 Skill
│           ├── tuya/            # 涂鸦 Skill
│           ├── homekit/         # HomeKit Skill
│           └── matter/          # Matter Skill
├── web/                         # Web UI（启动向导 + Chat）
│   ├── src/
│   └── dist/
└── workspace/
    └── data/
        ├── spaces.json          # 空间结构
        ├── devices.json         # 设备注册表
        ├── members.json         # 家庭成员
        └── workflows/           # 工作流定义文件

# 继承自 PicoClaw（不改动）
pkg/channels/    # 多渠道接入
pkg/providers/   # 大模型 Provider
pkg/cron/        # 定时任务
pkg/memory/      # 长期记忆
pkg/mcp/         # MCP 工具
pkg/auth/        # 认证
pkg/config/      # 配置（扩展字段，不改原有）
```

---

## 二、数据结构定义

### 2.1 空间结构（spaces.json）

```json
{
  "version": "1",
  "spaces": [
    {
      "id": "floor-1",
      "name": "一楼",
      "type": "floor",
      "children": [
        {
          "id": "living-room",
          "name": "客厅",
          "type": "room",
          "children": []
        },
        {
          "id": "kitchen",
          "name": "厨房",
          "type": "room",
          "children": []
        }
      ]
    },
    {
      "id": "floor-2",
      "name": "二楼",
      "type": "floor",
      "children": [
        {
          "id": "master-bedroom",
          "name": "主卧",
          "type": "room",
          "children": []
        },
        {
          "id": "kids-room",
          "name": "儿童房",
          "type": "room",
          "children": []
        }
      ]
    }
  ]
}
```

### 2.2 设备注册表（devices.json）

```json
{
  "version": "1",
  "devices": [
    {
      "id": "mijia-light-001",
      "name": "客厅吸顶灯",
      "brand": "mijia",
      "protocol": "miio",
      "model": "yeelink.light.ceiling1",
      "space_id": "living-room",
      "ip": "192.168.1.101",
      "token": "xxxxxxxx",
      "capabilities": ["on_off", "brightness", "color_temp"],
      "state": {
        "on": true,
        "brightness": 80,
        "color_temp": 4000
      },
      "last_seen": "2026-03-09T10:00:00Z",
      "added_at": "2026-03-01T08:00:00Z"
    }
  ]
}
```

### 2.3 家庭成员（members.json）

```json
{
  "version": "1",
  "members": [
    {
      "name": "爸爸",
      "role": "admin",
      "space_permissions": ["*"],
      "channels": {
        "telegram": {
          "user_id": "123456789",
          "bound_at": "2026-03-01T08:00:00Z"
        },
        "qq": null
      },
      "default_space_id": "master-bedroom",
      "created_at": "2026-03-01T08:00:00Z"
    },
    {
      "name": "小明",
      "role": "member",
      "space_permissions": ["kids-room"],
      "channels": {},
      "default_space_id": "kids-room",
      "created_at": "2026-03-01T08:00:00Z"
    }
  ]
}
```

### 2.4 工作流定义（workflows/sleep-scene.json）

```json
{
  "id": "sleep-scene",
  "name": "睡眠场景",
  "description": "准备睡觉时关闭房间的灯",
  "version": "1",
  "triggers": [
    {
      "type": "intent",
      "patterns": ["我要睡觉了", "睡觉", "晚安", "关灯睡觉"]
    }
  ],
  "context": {
    "space": "current",
    "member": "current"
  },
  "steps": [
    {
      "id": "step-1",
      "action": "device.control",
      "params": {
        "capability": "on_off",
        "value": false,
        "target": {
          "space": "{{context.space}}",
          "capability_filter": ["on_off"]
        },
        "exclude": []
      }
    }
  ],
  "variants": {
    "小明": {
      "description": "小明的睡眠场景（保留台灯）",
      "steps": [
        {
          "id": "step-1",
          "action": "device.control",
          "params": {
            "capability": "on_off",
            "value": false,
            "target": {
              "space": "{{context.space}}",
              "capability_filter": ["on_off"]
            },
            "exclude": ["bedside-lamp-001"]
          }
        },
        {
          "id": "step-2",
          "action": "device.control.delayed",
          "params": {
            "device_id": "bedside-lamp-001",
            "capability": "on_off",
            "value": false,
            "delay_seconds": 3600
          }
        }
      ]
    }
  },
  "created_by": "llm",
  "created_at": "2026-03-01T08:00:00Z",
  "updated_at": "2026-03-01T08:00:00Z"
}
```

---

## 三、核心接口定义

### 3.1 Skill 标准接口（Go）

```go
// pkg/skills/homeclaw/skill.go

type Device struct {
	ID           string
	Name         string
	Brand        string
	Protocol     string
	Model        string
	IP           string
	Capabilities []string
	State        map[string]interface{}
}

type ControlParams struct {
	DeviceID   string
	Capability string
	Value      interface{}
}

type StateChange struct {
	DeviceID  string
	State     map[string]interface{}
	Timestamp time.Time
}

type Skill interface {
	// 扫描局域网内该生态的所有设备
	Discover(ctx context.Context) ([]Device, error)

	// 读取设备当前状态
	GetState(ctx context.Context, deviceID string) (map[string]interface{}, error)

	// 执行控制指令
	Control(ctx context.Context, params ControlParams) error

	// 订阅设备状态变化（事件驱动）
	Subscribe(ctx context.Context, deviceID string, ch chan<- StateChange) error

	// 取消订阅
	Unsubscribe(ctx context.Context, deviceID string) error

	// 返回该 Skill 支持的设备能力元数据
	Metadata() SkillMetadata
}

type SkillMetadata struct {
	Brand        string
	Protocol     string
	Capabilities []CapabilityDef
}

type CapabilityDef struct {
	Name        string
	Type        string // bool / int / float / enum
	Writable    bool
	Description string
}
```

### 3.2 工作流引擎接口

```go
// pkg/homeclaw/workflow/engine.go

type WorkflowEngine interface {
	// 加载工作流定义
	Load(def WorkflowDef) error

	// 执行工作流
	Execute(ctx ExecutionContext) error

	// 列出已注册的工作流
	List() []WorkflowDef

	// 删除工作流
	Remove(id string) error
}

type ExecutionContext struct {
	MemberName string                 // 成员名（唯一标识），空表示无法确定
	SpaceID    string                 // 空间 ID，空表示无法确定
	TriggerBy  string                 // "intent" | "event" | "cron"
	Params     map[string]interface{}
}
```

### 3.3 意图路由接口

```go
// pkg/homeclaw/router/router.go

type IntentResult struct {
	WorkflowID string
	Confidence float64
	Params     map[string]interface{}
	Fallback   bool // true 表示需要上报大模型处理
}

type Router interface {
	// 输入自然语言，返回匹配的工作流和参数
	// space 参数：
	// - Web UI Chat: 空字符串（无空间感知）
	// - 音箱渠道: 音箱所在房间名
	// - Telegram/QQ: 成员绑定的 home_space_id
	Route(ctx context.Context, input string, memberName string, space string) (IntentResult, error)
}
```

### 3.4 数据访问层接口

```go
// pkg/homeclaw/data/space.go
type SpaceStore interface {
	GetAll() ([]Space, error)
	GetByID(id string) (*Space, error)
	Save(space Space) error
	Delete(id string) error
	FindByName(name string) (*Space, error)
}

// pkg/homeclaw/data/device.go
type DeviceStore interface {
	GetAll() ([]Device, error)
	GetByID(id string) (*Device, error)
	GetBySpace(spaceID string) ([]Device, error)
	GetByCapability(capability string) ([]Device, error)
	Save(device Device) error
	UpdateState(id string, state map[string]interface{}) error
	Delete(id string) error
}

// pkg/homeclaw/data/member.go
type MemberStore interface {
	GetAll() ([]Member, error)
	GetByName(name string) (*Member, error)
	GetByChannelID(channel string, channelUserID string) (*Member, error)
	Save(member Member) error
	Delete(name string) error
}
```

---

### 3.5 默认 Skill 定义

系统内置两个默认 Skill，不依赖任何硬件生态，供工作流步骤和大模型直接调用。

#### 3.5.1 保存工作流 Skill（`workflow.save`）

```go
// pkg/skills/homeclaw/builtin/workflow_save.go

// WorkflowIndex 是工作流索引条目，供小模型快速检索
type WorkflowIndex struct {
	ID              string   // 工作流唯一 ID
	Name            string   // 工作流名称
	Description     string   // 描述（用于语义相似度匹配）
	TriggerPatterns []string // 触发关键词/模板
	Tags            []string // 可选标签（场景分类）
	UpdatedAt       string   // 最后更新时间（ISO 8601）
}

// WorkflowSaveParams 保存工作流 Skill 的调用参数
type WorkflowSaveParams struct {
	Def       WorkflowDef // 完整工作流定义
	CreatedBy string      // "llm" | "user"
	Overwrite bool        // 是否覆盖已有同 ID 工作流
}

// WorkflowSaveSkill 保存工作流并同步更新索引
// action 名: "workflow.save"
type WorkflowSaveSkill interface {
	// Save 保存工作流定义到 workflows/ 目录，同步更新 workflow-index.json
	// - 若 ID 已存在且 Overwrite=false，返回 ErrWorkflowExists
	// - 保存成功后触发 WorkflowEngine.Load() 热加载
	Save(ctx context.Context, params WorkflowSaveParams) error

	// GetIndex 返回当前全量工作流索引（供小模型检索使用）
	GetIndex(ctx context.Context) ([]WorkflowIndex, error)
}
```

**工作流索引文件**（`workspace/data/workflow-index.json`）：

```json
{
  "version": "1",
  "updated_at": "2026-03-09T10:00:00Z",
  "workflows": [
    {
      "id": "sleep-scene",
      "name": "睡眠场景",
      "description": "准备睡觉时关闭房间的灯",
      "trigger_patterns": ["我要睡觉了", "睡觉", "晚安", "关灯睡觉"],
      "tags": ["灯光", "场景", "睡眠"],
      "updated_at": "2026-03-01T08:00:00Z"
    }
  ]
}
```

**设计说明：**
- 索引文件与工作流定义文件分离，索引只包含匹配所需的最小字段，不含完整步骤，保证小模型加载轻量
- 每次调用 `workflow.save` 均原子性地重写索引文件，保证一致性
- `WorkflowEngine` 监听 `workflows/` 目录变更，自动热加载新增/更新的工作流

---

#### 3.5.2 调用模型 Skill（`model.call`）

```go
// pkg/skills/homeclaw/builtin/model_call.go

// ModelRole 标识调用的模型角色
type ModelRole string

const (
	// ModelRoleSmall 小模型：本地 Ollama，用于轻量意图识别和简单语义判断
	ModelRoleSmall ModelRole = "small"
	// ModelRoleLarge 大模型：继承 PicoClaw providers，用于复杂推理和内容生成
	ModelRoleLarge ModelRole = "large"
)

// ModelCallParams 调用模型 Skill 的参数
type ModelCallParams struct {
	Role    ModelRole              // 调用哪类模型
	Prompt  string                 // 主提示词
	System  string                 // 可选系统提示词
	Context map[string]interface{} // 注入到提示词模板的上下文变量
	Schema  *json.RawMessage       // 可选：期望的 JSON 响应 Schema（用于结构化输出）
}

// ModelCallResult 模型响应结果
type ModelCallResult struct {
	Text   string      // 原始文本输出
	Parsed interface{} // 若 Schema 非空，反序列化后的结构体
	Model  string      // 实际使用的模型名称
	Tokens int         // 消耗的 token 数（估算）
}

// ModelCallSkill 统一的模型调用接口
// action 名: "model.call"
type ModelCallSkill interface {
	// Call 调用指定角色的模型并返回结果
	// 内部根据 Role 自动路由：
	// - small → Ollama HTTP API（配置的本地模型）
	// - large → PicoClaw Provider（config 中配置的大模型）
	Call(ctx context.Context, params ModelCallParams) (ModelCallResult, error)
}
```

**典型使用场景：**

| 场景 | Role | 说明 |
|---|---|---|
| Router 快速意图识别 | `small` | 输入用户语句 + 工作流索引摘要，判断是否有匹配工作流及其 ID |
| 大模型生成工作流 | `large` | 输入用户需求 + 现有设备列表，输出 `WorkflowDef` JSON |
| 工作流步骤中处理复杂语义 | `large` | 步骤 action 为 `model.call`，将上下文注入提示词，结果作为后续步骤参数 |
| 修正窗口判断 | `small` | 输入新指令 + 上次执行记录，判断是否为修正操作 |

**工作流步骤中的调用示例：**

```json
{
  "id": "step-infer-brightness",
  "action": "model.call",
  "params": {
    "role": "small",
    "prompt": "用户说：{{input}}。请从以下选项中选择合适的亮度值（0-100 整数）：{{brightness_options}}",
    "schema": { "type": "integer", "minimum": 0, "maximum": 100 },
    "output_as": "brightness_value"
  }
},
{
  "id": "step-set-brightness",
  "action": "device.control",
  "params": {
    "capability": "brightness",
    "value": "{{brightness_value}}",
    "target": { "space": "{{context.space}}" }
  }
}
```

**设计说明：**
- `model.call` 作为工作流的一等步骤类型，工作流引擎原生支持其输出绑定到 `output_as` 变量供后续步骤引用
- 小模型和大模型的配置（模型名/端点/超时）统一在 `config.json` 中维护，`ModelCallSkill` 不持有配置逻辑
- 大模型调用通过 PicoClaw 已有的 Provider 接口完成，复用鉴权和重试机制

---

## 四、请求处理流程

### 4.1 日常控制请求（主流程）

```
用户输入（任意渠道）
↓
渠道层识别身份（member_id）
↓
空间感知处理（各渠道独立实现）
- Web UI Chat: 无空间感知（space 为空），根据对话上下文推断，或操作时必须明确空间时追问用户
- 音箱渠道: 音箱所在房间名作为 space，成员无法识别（memberName 为空）
- Telegram/QQ: 根据渠道 user_id 查成员，空间为空（不自动使用成员房间）
↓
Router.Route(input, memberName, space)
├── 匹配成功（confidence > 0.8）
│ ↓
│ 检查成员是否有个性化变体（以 memberName 为 key）
│ ↓
│ WorkflowEngine.Execute(execCtx)
│ ↓
│ 调用对应 Skill.Control()
│ ↓
│ 多渠道反馈结果
│
└── 匹配失败（fallback=true）
↓
调用大模型处理
├── 简单控制指令 → 直接执行
└── 复杂场景 → 生成工作流 → 存储 → 执行
```

### 4.2 工作流修正检测

```
工作流执行完成
↓
开启修正窗口（30秒）
↓
窗口内收到新输入
↓
判断是否为修正指令（对上次操作的补充/撤销）
├── 是 → 执行修正操作
│ ↓
│ 询问是否保存为个性化变体
│
└── 否 → 作为新指令处理，关闭修正窗口
```

---

## 五、Web UI 页面结构

```
/setup              # 首次启动向导（未初始化时强制跳转）
  step-1: 设置管理员账号
  step-2: 配置 AI 模型
  step-3: Chat 引导（配置渠道 → 房间 → 设备 → 成员）

/                   # 主界面（初始化后）
/chat               # 对话控制（主要交互入口）
/spaces             # 空间管理页面
/devices            # 设备管理页面
/members            # 成员管理页面
/workflows          # 工作流列表/编辑
/settings           # 系统设置（模型/渠道/认证）
```

---

## 六、开发阶段计划

### Phase 1 — 基础骨架（2 周）

**目标：能跑起来，Web UI 能完成初始化引导**

| 任务 | 模块 | 说明 |
|---|---|---|
| P1-01 | `cmd/homeclaw/` | fork 并创建新主入口，去除 picoclaw CLI 相关 |
| P1-02 | `pkg/homeclaw/data/` | 实现 SpaceStore / DeviceStore / MemberStore（JSON 文件读写） |
| P1-03 | `web/` | 搭建 Web UI 框架（React + Vite），实现 Setup 向导页 step-1/2 |
| P1-04 | `web/` | 实现 Web UI Chat 页面（对接大模型 Provider） |
| P1-05 | `cmd/homeclaw/` | 内嵌 Web UI 静态资源，单二进制分发 |

**验收标准：** 启动后访问 Web UI，完成账号设置和模型配置，能在 Chat 页面与大模型对话。

---

### Phase 2 — 引导流程（2 周）

**目标：通过对话完成房间/设备/成员的初始化配置**

| 任务 | 模块 | 说明 |
|---|---|---|
| P2-01 | `pkg/homeclaw/data/` | 实现房间布局 AI 解析（自然语言 → spaces.json） |
| P2-02 | `pkg/skills/homeclaw/mijia/` | 实现米家 Skill（Discover / GetState / Control） |
| P2-03 | `pkg/homeclaw/data/` | 实现设备扫描引导流程（推断房间 → 询问 → 开关确认法） |
| P2-04 | `pkg/homeclaw/data/` | 实现成员创建和渠道绑定流程（生成验证码机制） |
| P2-05 | `web/` | Setup 向导 step-3（Chat 引导 UI） |

**验收标准：** 从零开始引导，完成房间设置 + 米家设备接入 + 一个成员绑定 Telegram。

---

### Phase 3 — 工作流引擎（2 周）

**目标：能执行工作流，实现基础设备控制**

| 任务 | 模块 | 说明 |
|---|---|---|
| P3-01 | `pkg/homeclaw/workflow/types.go` | 定义工作流 JSON Schema 和 Go 数据结构 |
| P3-02 | `pkg/homeclaw/workflow/engine.go` | 实现工作流执行引擎（串行步骤 + 延时步骤） |
| P3-03 | `pkg/homeclaw/workflow/trigger.go` | 实现三种触发源（对话/事件/cron）接入引擎 |
| P3-04 | `pkg/homeclaw/workflow/loader.go` | 实现工作流文件加载/存储/热更新 |
| P3-05 | 大模型集成 | 实现「无匹配 → 大模型生成工作流 → 存储」闭环 |

**验收标准：** 说「关掉客厅的灯」可以执行，说「我要睡觉了」大模型生成工作流并执行。

---

### Phase 4 — 意图路由（1 周）

**目标：接入小模型，降低大模型调用频率**

| 任务 | 模块 | 说明 |
|---|---|---|
| P4-01 | `pkg/homeclaw/router/` | 实现基于关键词/模板的轻量意图匹配（不依赖模型） |
| P4-02 | `pkg/homeclaw/router/` | 接入本地小模型（Ollama），实现模糊语义匹配 |
| P4-03 | 渠道层扩展 | 增强原有渠道，实现空间感知注入（音箱渠道需单独实现） |

**验收标准：** 高频指令（开/关灯）走本地匹配，不调大模型，响应 < 200ms。

---

### Phase 5 — 多渠道接入（1 周）

**目标：Telegram 和 QQ 渠道可正常使用**

| 任务 | 模块 | 说明 |
|---|---|---|
| P5-01 | 渠道适配 | 复用 PicoClaw channels 层，接入 HomeClaw 身份识别 |
| P5-02 | 身份绑定 | 实现渠道 user_id → member_id 的映射和验证码绑定流程 |
| P5-03 | 多渠道反馈 | 实现操作结果同时推送多个渠道，并标记已处理 |

**验收标准：** 成员通过 Telegram 发「关灯」，正确识别身份和空间，执行并回复结果。

---

### Phase 6 — 习惯学习与个性化（1 周）

**目标：工作流修正检测和个性化变体**

| 任务 | 模块 | 说明 |
|---|---|---|
| P6-01 | `pkg/homeclaw/workflow/engine.go` | 实现执行后修正窗口机制 |
| P6-02 | `pkg/homeclaw/workflow/` | 实现个性化变体创建和存储 |
| P6-03 | `pkg/homeclaw/router/` | 执行时优先匹配当前成员的个性化变体 |

**验收标准：** 睡眠场景后说「保留台灯」，HomeClaw 询问是否保存，确认后下次自动应用。

---

### Phase 7 — 更多生态 Skill（持续）

| Skill | 优先级 | 说明 |
|---|---|---|
| 涂鸦 Skill | P1 | Local API + WebSocket |
| HomeKit Skill | P2 | HAP 协议，Go 有开源库 |
| Matter Skill | P3 | 标准协议，Go SDK 尚在成熟中 |
| 语音平台接入 | P2 | 小爱同学 / 天猫精灵 IoT 平台对接 |

---

## 七、技术选型

| 组件 | 选型 | 理由 |
|---|---|---|
| 后端语言 | Go 1.25.7 | 继承 PicoClaw |
| Web UI | Vue 3 + Vite | 轻量，易于嵌入单二进制 |
| Web 框架 | `net/http` + `embed` | 静态资源内嵌，无外部依赖 |
| 数据存储 | JSON 文件 | 设备上限 100，无需数据库 |
| 小模型推理 | Ollama HTTP API | 本地推理，用户自选模型 |
| 大模型 | 继承 PicoClaw providers | 多 Provider 支持 |
| 米家协议 | `go-miio`（社区库） | miio 协议局域网直连 |
| 涂鸦协议 | Tuya Local API | 官方本地协议 |
| HomeKit | `hap`（Go 开源库） | HAP 协议实现 |

---

## 八、里程碑时间线

```
Week 1-2   Phase 1  基础骨架 + Web UI Setup 向导
Week 3-4   Phase 2  引导流程（房间/设备/成员）
Week 5-6   Phase 3  工作流引擎
Week 7     Phase 4  意图路由（小模型）
Week 8     Phase 5  多渠道接入
Week 9     Phase 6  习惯学习
Week 10+   Phase 7  更多生态 Skill（持续迭代）
```

**MVP 里程碑（第 6 周末）：**
- 完成初始化引导
- 米家设备接入
- 工作流执行
- Telegram 渠道控制
- 基础场景（开/关灯、睡眠场景）可用
