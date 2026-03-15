package intent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	homeclawcfg "github.com/sipeed/picoclaw/pkg/homeclaw/config"
	"github.com/sipeed/picoclaw/pkg/providers"
)

// intentClassifyPrompt is the system prompt used for intent classification.
// The intent types here correspond 1-to-1 with the IntentType constants in intent.go.
const intentClassifyPrompt = `你是一个智能家居助手的意图识别器。请分析用户输入，从以下类别中选择最匹配的意图。

## 设备控制类
- device.control.single:  控制单个具体设备（开灯、关空调、调温度到26度）
- device.control.scene:   触发场景或情景模式（睡觉模式、出门、回家、电影模式）
- device.control.global:  对所有或某类设备执行同一操作（关掉所有灯、全屋空调调低）
- device.control.correct: 修正刚才的操作（不对，保留台灯、把那盏灯关掉）

## 设备管理类
- device.add:          添加新设备到系统
- device.scan:         扫描/发现局域网设备
- device.remove:       删除/移除设备
- device.rename:       重命名设备
- device.move:         将设备移动到其他房间
- device.query.status: 查询设备当前状态（灯是否开着、空调温度是多少）

## 空间管理类
- space.define:  定义/创建家的空间结构（我有客厅、卧室、书房）
- space.rename:  重命名房间或空间
- space.query:   查询空间结构或某个房间里有哪些设备

## 用户管理类
- user.add:             添加家庭成员
- user.remove:          删除家庭成员
- user.query:           查询成员列表或某成员信息

## 系统配置类
- config.skill.enable:  启用某个 Skill 插件
- config.skill.disable: 禁用某个 Skill 插件

## 对话类
- chat.greeting: 问候、打招呼
- chat.help:     询问能做什么、使用帮助
- chat.confirm:  确认操作（好的、确认、是的、没问题）
- chat.cancel:   取消操作（不了、算了、取消）

## 输出规则
- 只输出 JSON，不要有任何额外内容。
- confidence 表示你对该意图判断的置信度，范围 0.0–1.0。
- entities 提取与意图相关的关键实体，常见字段：
  - device_name: 设备名称（如"台灯"、"客厅空调"）
  - action: 动作（如"on"、"off"、"set"）
  - value: 目标值（如温度"26"、亮度"50%"）
  - space_name: 空间/房间名称
  - member_name: 家庭成员名称
  - workflow_name: 工作流名称
  - skill_name: Skill 名称
- 若无相关实体则 entities 为空对象 {}。
- 若无法判断意图或置信度极低，intent 填 "unknown"。

输出格式：
{
    "intent": "<intent_type>",
    "confidence": <0.0-1.0>,
    "entities": {}
}`

// llmClassifier implements IntentClassifier by calling a small language model.
type llmClassifier struct {
	provider  providers.LLMProvider
	cfg       *homeclawcfg.HomeclawConfig
	modelName string // resolved model identifier sent to the provider
}

// NewLLMClassifier creates an IntentClassifier that uses the given LLMProvider
// (expected to be a small / lightweight model) for intent recognition.
// modelName is the model identifier passed to provider.Chat().
func NewLLMClassifier(
	provider providers.LLMProvider,
	cfg *homeclawcfg.HomeclawConfig,
	modelName string,
) IntentClassifier {
	return &llmClassifier{
		provider:  provider,
		cfg:       cfg,
		modelName: modelName,
	}
}

// Classify sends the userInput to the small model and parses the JSON response.
// On error or low confidence it returns IntentUnknown so the agent loop can
// fall through to the large-model handler.
func (c *llmClassifier) Classify(ctx context.Context, userInput string) (IntentResult, error) {
	unknown := IntentResult{Type: IntentUnknown, Confidence: 0}

	if c.provider == nil {
		return unknown, nil
	}

	userMsg := fmt.Sprintf("用户输入: %s", userInput)

	messages := []providers.Message{
		{Role: "system", Content: intentClassifyPrompt},
		{Role: "user", Content: userMsg},
	}

	resp, err := c.provider.Chat(ctx, messages, nil, c.modelName, map[string]any{
		"max_tokens":  256,
		"temperature": 0.0,
	})
	if err != nil {
		// Degrade gracefully: classification failure falls through to large model.
		return unknown, fmt.Errorf("intent classifier: %w", err)
	}

	if resp == nil || len(resp.Content) == 0 {
		return unknown, nil
	}

	raw := extractJSON(resp.Content)
	if raw == "" {
		return unknown, fmt.Errorf("intent classifier: no JSON in response: %q", resp.Content)
	}

	var result IntentResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return unknown, fmt.Errorf("intent classifier: parse response: %w", err)
	}

	// Apply confidence threshold.
	threshold := c.cfg.ConfidenceThreshold
	if threshold <= 0 {
		threshold = homeclawcfg.DefaultConfidenceThreshold
	}
	if result.Confidence < threshold {
		return unknown, nil
	}

	if result.Type == "" {
		return unknown, nil
	}

	return result, nil
}

// extractJSON attempts to extract a JSON object from a larger string.
// The model sometimes wraps the JSON in markdown code fences.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	// Strip markdown code fences if present.
	if idx := strings.Index(s, "```"); idx >= 0 {
		s = s[idx+3:]
		if strings.HasPrefix(s, "json") {
			s = s[4:]
		}
		if end := strings.Index(s, "```"); end >= 0 {
			s = s[:end]
		}
	}

	// Find the first '{' ... '}' block.
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < start {
		return ""
	}
	return strings.TrimSpace(s[start : end+1])
}
