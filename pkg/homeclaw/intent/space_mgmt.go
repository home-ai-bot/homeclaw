package intent

import (
	"context"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
)

// SpaceMgmtIntent handles space management intents (space.define, space.rename,
// space.query).
type SpaceMgmtIntent struct {
	store data.SpaceStore
}

// NewSpaceMgmtIntent creates a SpaceMgmtIntent backed by the given SpaceStore.
// If store is nil the handler falls through to the large model for all intents.
func NewSpaceMgmtIntent(store data.SpaceStore) *SpaceMgmtIntent {
	return &SpaceMgmtIntent{store: store}
}

// Types implements Intent.
func (s *SpaceMgmtIntent) Types() []IntentType {
	return []IntentType{
		IntentSpaceDefine,
		IntentSpaceRename,
		IntentSpaceQuery,
	}
}

// Run executes the space management operation and returns a direct reply.
func (s *SpaceMgmtIntent) Run(_ context.Context, ictx IntentContext) IntentResponse {
	if s.store == nil {
		return IntentResponse{Handled: false}
	}

	switch ictx.Result.Type {
	case IntentSpaceDefine:
		return s.handleDefine(ictx)
	case IntentSpaceRename:
		return s.handleRename(ictx)
	case IntentSpaceQuery:
		return s.handleQuery(ictx)
	default:
		return IntentResponse{Handled: false}
	}
}

func (s *SpaceMgmtIntent) handleDefine(ictx IntentContext) IntentResponse {
	name := entityString(ictx.Result.Entities, "space_name")
	if name == "" {
		return IntentResponse{Handled: false}
	}

	space := data.Space{
		Name: name,
		From: map[string]string{"name": "manual"},
	}
	if err := s.store.Save(space); err != nil {
		return errResponse(fmt.Sprintf("创建空间「%s」失败：%s", name, err.Error()), err)
	}
	return IntentResponse{
		Handled:  true,
		Response: fmt.Sprintf("已创建空间「%s」。", name),
	}
}

func (s *SpaceMgmtIntent) handleRename(ictx IntentContext) IntentResponse {
	oldName := entityString(ictx.Result.Entities, "space_name")
	newName := entityString(ictx.Result.Entities, "new_name")
	if oldName == "" || newName == "" {
		return IntentResponse{Handled: false}
	}

	spaces, err := s.store.GetAll()
	if err != nil {
		return errResponse(fmt.Sprintf("查找空间失败：%s", err.Error()), err)
	}
	for _, space := range spaces {
		if strings.EqualFold(space.Name, oldName) {
			// Delete old and save with new name
			if err := s.store.Delete(space.Name); err != nil {
				return errResponse(fmt.Sprintf("重命名空间失败：%s", err.Error()), err)
			}
			space.Name = newName
			if err := s.store.Save(space); err != nil {
				return errResponse(fmt.Sprintf("重命名空间失败：%s", err.Error()), err)
			}
			return IntentResponse{
				Handled:  true,
				Response: fmt.Sprintf("已将「%s」重命名为「%s」。", oldName, newName),
			}
		}
	}
	return IntentResponse{Handled: true, Response: fmt.Sprintf("未找到空间「%s」。", oldName)}
}

func (s *SpaceMgmtIntent) handleQuery(ictx IntentContext) IntentResponse {
	name := entityString(ictx.Result.Entities, "space_name")

	// Query a specific space.
	if name != "" {
		spaces, err := s.store.GetAll()
		if err != nil {
			return errResponse(fmt.Sprintf("查询空间失败：%s", err.Error()), err)
		}
		for _, space := range spaces {
			if strings.EqualFold(space.Name, name) {
				return IntentResponse{Handled: true, Response: fmt.Sprintf("空间「%s」已定义。", space.Name)}
			}
		}
		return IntentResponse{Handled: true, Response: fmt.Sprintf("未找到空间「%s」。", name)}
	}

	// Query all spaces.
	spaces, err := s.store.GetAll()
	if err != nil {
		return errResponse(fmt.Sprintf("查询空间列表失败：%s", err.Error()), err)
	}
	if len(spaces) == 0 {
		return IntentResponse{Handled: true, Response: "当前没有定义任何空间。"}
	}
	names := make([]string, 0, len(spaces))
	for _, sp := range spaces {
		names = append(names, sp.Name)
	}
	return IntentResponse{
		Handled:  true,
		Response: fmt.Sprintf("共有 %d 个空间：%s。", len(spaces), strings.Join(names, "、")),
	}
}
