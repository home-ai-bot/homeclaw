package intent

import "context"

// SystemConfigIntent handles system configuration intents
// (config.skill.enable, config.skill.disable).
//
// Skill configuration changes require the large model to confirm the skill
// name, validate availability, and apply changes.  This handler always
// returns Handled=false so the agent loop falls through to runAgentLoop().
type SystemConfigIntent struct{}

// Types implements Intent.
func (s *SystemConfigIntent) Types() []IntentType {
	return []IntentType{
		IntentConfigSkillEnable,
		IntentConfigSkillDisable,
	}
}

// Run delegates all system configuration operations to the large-model agent loop.
func (s *SystemConfigIntent) Run(_ context.Context, _ IntentContext) IntentResponse {
	return IntentResponse{Handled: false}
}
