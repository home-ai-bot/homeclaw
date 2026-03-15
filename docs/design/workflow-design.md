# 工作流详细设计

## 1. 概述

工作流系统是 HomeClaw 的核心功能，用于定义和执行自动化任务。本文档详细描述工作流的存储、索引和执行机制。

## 2. 存储架构

### 2.1 文件结构

```
workspace/
└── homeclaw/
    └── data/                  # 所有数据存储在 data 目录
        ├── workflow-index.json      # 工作流索引文件
        └── workflows/               # 工作流定义文件目录
            ├── workflow-001.json
            ├── workflow-002.json
            └── ...
```

### 2.2 设计原则

- **索引与数据分离**：使用 `workflow-index.json` 存储所有工作流的元数据索引
- **独立文件存储**：每个工作流定义存储在单独的文件中，便于独立管理和版本控制
- **原子操作**：所有写操作使用 JSONStore 的备份机制，确保数据安全

## 3. 数据模型

### 3.1 工作流索引 (WorkflowIndex)

`workflow-index.json` 存储所有工作流的元数据：

```json
{
  "version": "1",
  "workflows": [
    {
      "id": "wf-001",
      "name": "晚安模式",
      "description": "关闭所有灯光，启动安防",
      "file_name": "workflow-wf-001.json",
      "created_by": "admin",
      "created_at": "2024-01-15T10:30:00Z",
      "updated_at": "2024-01-20T14:22:00Z",
      "enabled": true
    }
  ]
}
```

**字段说明：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | string | 是 | 工作流唯一标识，格式 `wf-{timestamp}` 或 `wf-{uuid}` |
| name | string | 是 | 工作流名称，用户可读 |
| description | string | 否 | 工作流描述 |
| file_name | string | 是 | 工作流定义文件名称 |
| created_by | string | 是 | 创建者标识 |
| created_at | string | 是 | 创建时间，ISO 8601 格式 |
| updated_at | string | 是 | 更新时间，ISO 8601 格式 |
| enabled | bool | 是 | 是否启用 |



### 3.3 工作流定义 (WorkflowDef)

所有工作流相关类型定义统一放在 `data/types.go`：

```go
// ==================== 工作流索引类型 ====================

// WorkflowMeta represents workflow metadata in the index
type WorkflowMeta struct {
    ID          string    `json:"id"`
    Name        string    `json:"name"`
    Description string    `json:"description"`
    FileName    string    `json:"file_name"`
    CreatedBy   string    `json:"created_by"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
    Enabled     bool      `json:"enabled"`
}

// WorkflowsData is the root structure for workflow-index.json
type WorkflowsData struct {
    Version   string         `json:"version"`
    Workflows []WorkflowMeta `json:"workflows"`
}

// ==================== 工作流定义类型 ====================

// WorkflowDef represents a workflow definition
type WorkflowDef struct {
    ID          string             `json:"id"`
    Name        string             `json:"name"`
    Description string             `json:"description"`
    Version     string             `json:"version"`
    Triggers    []Trigger          `json:"triggers"`
    Context     WorkflowContext    `json:"context"`
    Steps       []Step             `json:"steps"`
    Variants    map[string]Variant `json:"variants"`
    CreatedBy   string             `json:"created_by"`
    CreatedAt   time.Time          `json:"created_at"`
    UpdatedAt   time.Time          `json:"updated_at"`
}

type Trigger struct {
    Type     string   `json:"type"`     // "intent", "event", "cron"
    Patterns []string `json:"patterns"` // for intent triggers
}

type WorkflowContext struct {
    Space  string `json:"space"`  // "current" or specific space ID
    Member string `json:"member"` // "current" or specific member name
}
```

### 3.4 步骤定义 (Step)

Step 支持多种类型：工具调用、条件判断、循环控制。

```go
// StepType 定义步骤类型
type StepType string

const (
    StepTypeAction   StepType = "action"   // 工具调用
    StepTypeCondition StepType = "condition" // 条件判断
    StepTypeLoop     StepType = "loop"     // 循环控制
    StepTypeParallel StepType = "parallel" // 并行执行
)

// Step represents a single step in a workflow
type Step struct {
    ID       string                 `json:"id"`
    Type     StepType               `json:"type"`
    Name     string                 `json:"name,omitempty"`     // 步骤名称，便于调试
    
    // Action 类型字段
    Action   string                 `json:"action,omitempty"`   // 工具名称，如 "device.control", "model.call"
    Params   map[string]interface{} `json:"params,omitempty"`   // 工具参数，支持变量引用
    OutputAs string                 `json:"output_as,omitempty"` // 输出变量名，如 "light_result"
    
    // Condition 类型字段
    Condition *Condition            `json:"condition,omitempty"` // 条件判断配置
    
    // Loop 类型字段
    Loop     *LoopConfig           `json:"loop,omitempty"`     // 循环配置
    
    // Parallel 类型字段
    Branches []Step                `json:"branches,omitempty"` // 并行分支
}
```

### 3.5 条件判断 (Condition)

支持基于变量值的动态分支选择：

```go
// Condition defines conditional branching
type Condition struct {
    // 条件表达式，支持以下形式：
    // - "${varName}" - 判断变量是否存在/为真
    // - "${varName} == value" - 等于
    // - "${varName} != value" - 不等于
    // - "${varName} > 10" - 大于
    // - "${varName} contains 'xxx'" - 包含
    If   string `json:"if"`
    
    Then []Step `json:"then"` //条件为真时执行的步骤
    Else []Step `json:"else"` // 条件为假时执行的步骤
}
```

### 3.6 循环控制 (Loop)

支持基于集合或条件的循环：

```go
// LoopType 定义循环类型
type LoopType string

const (
    LoopTypeForEach LoopType = "foreach" // 遍历集合
    LoopTypeWhile   LoopType = "while"   // 条件循环
    LoopTypeRepeat  LoopType = "repeat"  // 固定次数
)

// LoopConfig defines loop configuration
type LoopConfig struct {
    Type LoopType `json:"type"`
    
    // ForEach 循环: "${devices}" 或 "${query_result.items}"
    // While 循环: "${status} != 'completed'"
    // Repeat 循环: "5"
    Expression string `json:"expression"`
    
    // 循环变量名，ForEach 时使用
    // 如 "device"，则循环内可通过 ${device.id} 访问
    Iterator string `json:"iterator,omitempty"`
    
    // 索引变量名（可选）
    IndexVar string `json:"index_var,omitempty"`
    
    // 循环体步骤
    Steps []Step `json:"steps"`
    
    // 最大迭代次数，防止死循环，默认 100
    MaxIterations int `json:"max_iterations,omitempty"`
}
```

### 3.7 变量引用语法

参数值支持变量引用，使用 `${}` 语法：

```go
// 变量引用示例：
{
    "action": "device.control",
    "params": {
        "device_id": "${device.id}",           // 引用循环变量
        "brightness": "${input.brightness}",   // 引用输入参数
        "previous_result": "${step1.output}"   // 引用前面步骤的输出
    }
}
```

**支持的引用类型：**

| 引用格式 | 说明 | 示例 |
|---------|------|------|
| `${input.xxx}` | 引用触发参数 | `${input.room}` |
| `${context.xxx}` | 引用上下文 | `${context.space_id}` |
| `${stepId.output}` | 引用步骤输出 | `${turn_on_light.result}` |
| `${varName}` | 引用命名变量 | `${device}` |
| `${varName.property}` | 引用对象属性 | `${device.id}` |
| `${varName[index]}` | 引用数组元素 | `${devices[0]}` |

### 3.8 完整示例

```json
{
  "id": "wf-evening-mode",
  "name": "傍晚模式",
  "description": "根据光线自动调节灯光",
  "version": "1",
  "triggers": [
    {
      "type": "intent",
      "patterns": ["打开傍晚模式", "执行傍晚模式"]
    }
  ],
  "context": {
    "space": "current"
  },
  "steps": [
    {
      "id": "get_light_sensor",
      "type": "action",
      "action": "device.query",
      "params": {
        "space_id": "${context.space_id}",
        "capability": "illuminance"
      },
      "output_as": "sensors"
    },
    {
      "id": "check_light",
      "type": "condition",
      "condition": {
        "if": "${sensors[0].value} < 100",
        "then": [
          {
            "id": "get_lights",
            "type": "action",
            "action": "device.query",
            "params": {
              "space_id": "${context.space_id}",
              "capability": "light"
            },
            "output_as": "lights"
          },
          {
            "id": "turn_on_all",
            "type": "loop",
            "loop": {
              "type": "foreach",
              "expression": "${lights}",
              "iterator": "light",
              "steps": [
                {
                  "id": "turn_on",
                  "type": "action",
                  "action": "device.control",
                  "params": {
                    "device_id": "${light.id}",
                    "power": "on",
                    "brightness": 80
                  }
                }
              ]
            }
          }
        ],
        "else": [
          {
            "id": "notify",
            "type": "action",
            "action": "notification.send",
            "params": {
              "message": "光线充足，无需开灯"
            }
          }
        ]
      }
    }
  ]
}
```

## 4. 存储接口设计

### 4.1 WorkflowStore 接口

```go
// WorkflowStore defines the interface for workflow data operations
type WorkflowStore interface {
    // 索引操作
    GetAllMeta() ([]WorkflowMeta, error)                    // 获取所有工作流元数据
    GetMetaByID(id string) (*WorkflowMeta, error)           // 根据 ID 获取元数据
    FindMetaByName(name string) (*WorkflowMeta, error)      // 根据名称查找元数据
    
    // 工作流定义操作
    GetByID(id string) (*workflow.WorkflowDef, error)       // 获取完整工作流定义
    Save(def *workflow.WorkflowDef, createdBy string) error // 保存工作流（新增或更新）
    Delete(id string) error                                 // 删除工作流
    
    // 状态管理
    Enable(id string) error                                 // 启用工作流
    Disable(id string) error                                // 禁用工作流
    IsEnabled(id string) (bool, error)                      // 检查是否启用
}
```

### 4.2 工作流程

#### 保存工作流 (Save)

```
1. 验证工作流定义
2. 生成/使用提供的 ID
3. 生成文件名: workflow-{id}.json
4. 更新索引 (workflow-index.json)
   - 如果是新建：添加新的 WorkflowMeta
   - 如果是更新：更新现有的 WorkflowMeta (updated_at)
5. 保存工作流定义到 workflows/workflow-{id}.json
```

#### 获取工作流 (GetByID)

```
1. 从索引中查找元数据
2. 如果未找到或已禁用，返回错误
3. 根据 file_name 读取工作流定义文件
4. 返回 WorkflowDef
```

#### 删除工作流 (Delete)

```
1. 从索引中移除元数据
2. 删除工作流定义文件
3. 更新索引文件
```

## 5. 与现有系统的集成

### 5.1 目录结构调整

删除 `loader.go`，workflow 包只保留执行相关功能：

```
pkg/homeclaw/
├── data/
│   ├── types.go             # 数据类型：WorkflowMeta, WorkflowsData, WorkflowDef, Step, Condition, Loop 等
│   ├── workflow.go          # 工作流存储实现 (WorkflowStore)
│   └── jsonstore.go         # JSON 存储基础
└── workflow/
    └── engine.go            # 执行引擎 - 执行工作流，调用 Skill
```

**删除文件：**
- `pkg/homeclaw/workflow/loader.go` - 功能由 `data.WorkflowStore` 替代
- `pkg/homeclaw/workflow/types.go` - 类型定义移至 `data/types.go`

### 5.2 工作流执行流程

```
用户触发 -> Intent解析 -> 工作流匹配 -> data.WorkflowStore.GetByID() -> workflow.Engine.Execute() -> 执行记录保存
```

**Engine 改造：**

```go
// Engine 工作流执行引擎
type Engine interface {
    // Execute 执行工作流
    // 参数:
    //   - ctx: 上下文
    //   - workflowDef: 工作流定义（从 WorkflowStore 读取）
    //   - execCtx: 执行上下文（包含 space, member, input 等）
    // 返回:
    //   - ExecutionRecord: 执行记录（已保存到 recorder）
    Execute(ctx context.Context, workflowDef *WorkflowDef, execCtx ExecutionContext) (*ExecutionRecord, error)
}

// NewEngine 创建执行引擎
// skillRegistry: Skill 注册表，用于执行 action 步骤（类似 toolloop 的 ToolRegistry）
func NewEngine(skillRegistry *skills.Registry) Engine
```

**Engine 职责：**
- 接收 WorkflowDef（已从 WorkflowStore 读取）
- 按顺序执行 Steps
- 调用 SkillRegistry 执行 action（类似 toolloop 调用 Tools.Execute）
- 执行记录通过 logger 写入日志

## 6. 命名规范

- **索引文件**：`workflow-index.json`
- **工作流文件**：`workflow-{id}.json`（例如：`workflow-wf-001.json`）
- **ID 格式**：`wf-{timestamp}` 或 `wf-{uuid}`
- **文件名生成规则**：`fmt.Sprintf("workflow-%s.json", workflowID)`

## 7. 错误处理

| 错误场景 | 返回错误 |
|---------|---------|
| 工作流不存在 | `ErrRecordNotFound` |
| 工作流已禁用 | `ErrWorkflowDisabled` |
| 索引文件损坏 | `ErrFileCorrupted` |
| 工作流文件丢失 | `ErrWorkflowFileMissing` |
| ID 冲突 | `ErrDuplicateID` |
| 名称冲突 | `ErrDuplicateName` |

## 8. 执行引擎设计

### 8.1 执行上下文 (ExecutionContext)

```go
type ExecutionContext struct {
    WorkflowID   string                 `json:"workflow_id"`
    ExecutionID  string                 `json:"execution_id"`
    MemberName   string                 `json:"member_name"`
    SpaceID      string                 `json:"space_id"`
    TriggerBy    string                 `json:"trigger_by"`
    Input        map[string]interface{} `json:"input"`        // 触发参数
    Variables    map[string]interface{} `json:"variables"`    // 变量存储
    StepResults  map[string]StepResult  `json:"step_results"` // 步骤执行结果
}
```

### 8.2 执行流程

Engine.Execute(ctx, workflowDef, execCtx) 执行逻辑：

```
1. 初始化执行上下文
   - 生成 ExecutionID
   - 创建变量空间 Variables = {input: execCtx.Input, context: execCtx}

2. 遍历执行 Steps（按顺序）
   
   StepTypeAction:
   - 解析参数变量（${xxx} -> 从 Variables 取值）
   - 调用 Skill 执行（类似 toolloop 的 Tools.Execute）
   - 结果存入 Variables[step.OutputAs]
   - 记录 StepExecution
   
   StepTypeCondition:
   - 计算条件表达式
   - 为真执行 Then 分支，为假执行 Else 分支
   
   StepTypeLoop:
   - 解析循环条件
   - 执行循环体，更新 iterator 变量
   - 检查 max_iterations

3. 记录执行结果到日志
   - 使用 logger 记录执行记录

4. 返回 ExecutionRecord（用于调用方展示或进一步处理）
```

### 8.3 变量解析器

```go
// VariableResolver handles variable substitution
type VariableResolver struct {
    ctx *ExecutionContext
}

// Resolve 解析字符串中的变量引用
// 输入: "Device ${device.name} is ${status}"
// 输出: "Device 客厅灯 is on"
func (r *VariableResolver) Resolve(input string) (interface{}, error)

// ResolveMap 递归解析 map 中的所有值
func (r *VariableResolver) ResolveMap(params map[string]interface{}) (map[string]interface{}, error)
```

## 9. 待讨论事项

1. **ID 生成策略**：使用时间戳还是 UUID？
2. **文件名格式**：是否需要包含工作流名称便于人工识别？
3. **软删除**：是否需要支持软删除（保留文件但标记为删除）？
4. **版本控制**：是否需要内置版本历史机制？
5. **并发控制**：是否需要处理并发编辑冲突？
6. **错误处理**：步骤失败时的重试、回滚策略？
7. **超时控制**：单个步骤和整个工作流的超时设置？
8. **日志查询**：执行记录只写入日志，是否需要通过日志查询历史执行？

---

*设计日期：2026-03-10*
*状态：待 Review*
