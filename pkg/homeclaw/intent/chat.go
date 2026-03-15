package intent

import "context"

// ChatIntent handles conversational intents (chat.*).
//
// These intents are handled directly by the small model without involving the
// large-model agent loop:
//   - chat.greeting  → friendly greeting reply
//   - chat.help      → usage help text
//   - chat.confirm   → acknowledgement for multi-turn flows
//   - chat.cancel    → cancellation for multi-turn flows
type ChatIntent struct{}

// Type implements Intent.
// ChatIntent handles all conversational subtypes.
func (c *ChatIntent) Types() []IntentType {
	return []IntentType{
		IntentChatGreeting,
		IntentChatHelp,
		IntentChatConfirm,
		IntentChatCancel,
	}
}

// Run handles a conversational intent and returns a direct reply.
func (c *ChatIntent) Run(_ context.Context, ictx IntentContext) IntentResponse {
	switch ictx.Result.Type {
	case IntentChatGreeting:
		return IntentResponse{
			Handled:  true,
			Response: "你好！我是 HomeClaw 智能家居助手，有什么可以帮你的吗？",
		}

	case IntentChatHelp:
		return IntentResponse{
			Handled: true,
			Response: "我可以帮你：\n" +
				"• 控制家里的智能设备（开灯、调温度、场景模式等）\n" +
				"• 管理设备、房间、家庭成员\n" +
				"• 创建和管理自动化工作流\n" +
				"直接告诉我你想做什么就好！",
		}

	case IntentChatConfirm:
		// Confirmation is typically handled by the calling multi-turn flow;
		// here we provide a safe fallback reply.
		return IntentResponse{
			Handled:  true,
			Response: "好的，已确认。",
		}

	case IntentChatCancel:
		return IntentResponse{
			Handled:  true,
			Response: "已取消，有需要随时告诉我。",
		}

	default:
		return IntentResponse{Handled: false}
	}
}
