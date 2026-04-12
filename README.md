# 🐾 homeclaw

[![License](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](LICENSE)
[![Status](https://img.shields.io/badge/Status-Alpha-orange.svg)](https://github.com/yourname/homeclaw)
[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8.svg)](https://go.dev/)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows%20%7C%20ARM-lightgrey.svg)](https://github.com/yourname/homeclaw)

> **AI 时代的智能家居大脑。**  
> 基于 AI 决策让自然语言成为控制家居的唯一接口。零门槛、全开放、本地优先。

---

## 🌟 产品定位

`homeclaw` 是专为 AI 时代重构的智能家居中枢。我们摒弃传统配置型系统的复杂逻辑，以 **大模型理解 + 小模型路由 + 本地工作流引擎** 为核心，打造真正零门槛、高隐私、全开放的智能家庭操作系统。

---

## ✨ 核心特性

| 特性 | 说明 |
|:---|:---|
| 🌐 **极简接入，自然对话** | 原生支持米家、涂鸦、HomeKit、Matter 等主流生态，自然对话实现设备发现、设备控制，无需复杂的学习成本。 |
| 🧠 **智能平权** | 设备只需提供控制接口，思考、调度、联动全由 `homeclaw` 大脑接管。智能门锁、喂食器、扫地机等‘智能（障）设备’彻底“去脑化”，专注执行。 |
| 💰 **极低部署成本** | 完美适配树莓派、NAS、旁路由、旧手机等闲置硬件。无大模型仅需 50MB 内存，有本地模型 8GB 即可流畅运行。 |
| 🔒 **本地化安全** | 内置本地大模型支持，所有数据、推理、指令均在局域网内完成。支持云端模型按需调用，隐私绝对可控。 |
| 🚀 **无限场景扩展** | 从监督写作业、防火防盗，到自动喂食、智能烹饪。支持空间感知控制、习惯学习与动态设备添加，场景无边界。 |

---
## 🚀 快速开始

### 📦 如何编译
#### linux Mac 编译

- make build 生成 picoclaw 可执行程序
- make build-launcher 生成 picoclaw-launcher 可执行程序
- make build-all 生成 picoclaw 各个平台的可执行程序

#### windows编译

- 缺少make命令 执行 .\scripts\build-windows.ps1 生成picoclaw、picoclaw-launcher 2个可执行程序

### 📦 部署方式

详细安装步骤请参考 [安装指南](./homeclaw-doc/install-step.md)。

### 📋 配置说明

- **首次启动**：将自动进入 Web UI 向导，引导完成 AI 模型配置、渠道绑定与设备扫描

### 💻 硬件要求

| 模式 | 内存 | 硬盘 | 适用场景 |
|:---|:---|:---|:---|
| 纯本地控制 | ≥ 50 MB | ≥ 100 MB | 基础设备联动、IM/音箱控制 |
| 本地大模型 | ≥ 8 GB | ≥ 20 GB | 完整 AI 决策、复杂场景生成、隐私敏感环境 |

---

## 🏗️ 架构设计

![HomeClaw 架构图](./homeclaw-doc/images/homeclaw.drawio.png)

### 🧠 核心能力

| 分类 | 模块 | 状态 | 说明 |
|:---|:---|:---|:---|
| **意图识别**|  | 🚧 开发中 | 基于本地小模型实现意图识别与路由 |
| **工作流引擎**|  | 🚧 开发中 | 大模型生成工作流、小模型调用执行、加速执行速度 |
| **基础能力** | jsonStore | ✅ 已完成 | JSON 数据存储 |
| | eventCenter | ✅ 已完成 | 事件中心 |
|  | ffmpeg封装 | ✅ 已完成 | 音视频处理 |
| | yolo能力 | ✅ 已完成 | 目标检测 |
| **基础对象** | home | ✅ 已完成 | 家庭 |
|  | room | ✅ 已完成 | 房间 |
|  | device | ✅ 已完成 | 设备 |
| **llm执行**|  | ✅ 已完成 | 大模型调用执行 |

## 🔌 设备接入

### 📊 接入进度

| 品牌 | 设备添加 | 云端控制 | 本地控制 | 本地视频 |
|:---|:---|:---|:---|:---|
| 小米 | ✅ 测试完成 | ✅ 测试完成 | ⏳ 未开始 |  ✅ 测试完成  |
| 涂鸦 | 开发完成  |   开发完成  | ⏳ 未开始 | ⏳ 未开始 |
| HomeKit | 开发完成 | 无 |  开发完成 | ⏳ 未开始 |
| Matter协议 | ⏳ 未开始 | ⏳ 未开始 | ⏳ 未开始 | ⏳ 未开始 |
| HomeAssistant | ⏳ 未开始 | ⏳ 未开始 | ⏳ 未开始 | ⏳ 未开始 |
| 海尔 | ⏳ 未开始 | ⏳ 未开始 | ⏳ 未开始 | ⏳ 未开始 |
| 美的 | ⏳ 未开始 | ⏳ 未开始 | ⏳ 未开始 | ⏳ 未开始 |
| 格力 | ⏳ 未开始 | ⏳ 未开始 | ⏳ 未开始 | ⏳ 未开始 |
| Wyze | ⏳ 未开始 | ⏳ 未开始 | ⏳ 未开始 | ⏳ 未开始 |
| Roborock | ⏳ 未开始 | ⏳ 未开始 | ⏳ 未开始 | ⏳ 未开始 |


### 🔐 授权

参考 [go2rtc](https://github.com/AlexxIT/go2rtc) 实现图形化设备授权管理。

### ⚡ 操作

#### 🔌 Client 接口实现

为保证实现跨平台，尽量使用实现client接口的方式来实现接入，参考 pkg\homeclaw\third\tuya\tuya_client.go、pkg\homeclaw\third\miio\mi_client.go

```go
// 身份识别
Brand() string  // 返回品牌名称

// 查询方法
GetHomes() ([]*HomeInfo, error)              // 获取家庭列表
GetRooms(homeID string) ([]*data.Space, error)   // 获取房间列表
GetDevices(homeID string) ([]*data.Device, error) // 获取设备列表
GetSpec(deviceID string) (*SpecInfo, error)   // 获取设备规格

// 设备控制
Execute(params map[string]any) (map[string]any, error)  // 执行设备动作
GetProps(params map[string]any) (any, error)            // 获取设备属性
SetProps(params map[string]any) (any, error)            // 设置设备属性

// 事件管理
EnableEvent(params map[string]any) error   // 启用事件订阅
DisableEvent(params map[string]any) error  // 禁用事件订阅

// 视频流
GetRtspStr(deviceID string) (string, error)  // 获取RTSP视频流URL
```

**本地控制特性**：支持本地控制，可接受设备事件推送。

#### 🛠️ Skill 方式

为保证多平台，目前暂时不支持skill方式加入，待讨论确定
---


## 🎯 典型场景

### ⚡ 极速安装 · 一句话同步，立享高端智能

- **极速安装**：APP 一键安装，首次向导自动完成模型配置与账号授权，自动同步全部设备
- **增量同步**：新设备入网后，一句"扫描新设备"触发 AI 推断归属，房间与用途自动识别
- **全平台通达**：微信、钉钉、Telegram、音箱，任意渠道发指令，统一响应

---

### 🌫️ 实时感知 · 无感决策，如空气般融入生活

- **人来灯开，人走灯关，天黑开灯，天亮关灯**：无需指示，顺其自然
- **下雨出门，温馨提醒，每天回家，亲切问候**：贴心感知生活节奏，该说的它来说
- **忘关火、忘关水，自动发现，及时提醒**：异常不过夜，安全不缺席
- **猫猫狗狗，定时喂食，宠物区温控，贴心照料**：细心如家人，从不遗漏

---

### 🧠 自我进化 · 真正懂你的高端智能，每日每月反思自己

- **说过的话，全部记住，下次触发，自动适配**：偏好沉淀，无需重复交代
- **每日复盘，每月反思，归纳偏差，修正规则**：越用越准，悄悄进化
- **多人共住，权限隔离，各自习惯，互不干扰**：全家共用，体验各异
- **第一天是助手，第一月是管家，第一年是伙伴**：真正懂你，与你同行

---

## 🤝 参与贡献

`homeclaw` 目前处于 Alpha 阶段，核心架构与 Skill 契约已稳定，欢迎早期体验者与开发者加入：

- 🐛 **提交 Issue**：功能建议、Bug 反馈、生态兼容性问题
- 🔀 **提交 PR**：Skill 插件开发、协议适配、UI/交互优化
- 📖 **完善文档**：部署教程、场景模板、硬件兼容列表

详见 [CONTRIBUTING.md](../CONTRIBUTING.md)（筹备中）

---

## 💬 社区与反馈

- 📮 **问题反馈**：[GitHub Issues](https://github.com/home-ai-bot/homeclaw/issues)
- 💬 **交流群**：<img src="./homeclaw-doc/images/group1.jpg" alt="HomeClaw开源1群" width="400">
- 🌐 **项目主页**：暂无
- 🗺️ **RoadMap**：[ROADMAP.md](./homeclaw-doc/roadmap.md)

---

## 📜 开源协议

本项目采用 [GPL-3.0 License](../LICENSE) 开源。

---

## 🙏 致谢

HomeClaw 站在众多优秀开源项目的肩膀上，向以下项目致以诚挚感谢：

- [**picoclaw**](https://github.com/qingconglaixueit/picoclaw) — 核心 AI Agent 引擎，提供多渠道对话与工具调度能力
- [**go2rtc**](https://github.com/AlexxIT/go2rtc) — 高性能实时流媒体框架，支持摄像头接入与音视频转发
- [**FFmpeg**](https://ffmpeg.org) — 多媒体处理基础设施，音视频编解码与流处理的行业标准


---

> **"让每一台设备专注执行，让每一次交互充满智慧。"**  
> 🐾 **homeclaw** —— AI 时代的智能家居大脑。