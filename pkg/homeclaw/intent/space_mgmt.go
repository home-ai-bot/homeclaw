package intent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
)

// spaceID generates a simple time-based ID for a new space.
func spaceID() string {
	return fmt.Sprintf("space-%d", time.Now().UnixNano())
}

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

	spaceType := entityString(ictx.Result.Entities, "space_type")
	if spaceType == "" {
		spaceType = "room"
	}

	space := data.Space{
		ID:   spaceID(),
		Name: name,
		Type: spaceType,
	}
	if err := s.store.Save(space); err != nil {
		return errResponse(fmt.Sprintf("创建空间「%s」失败：%s", name, err.Error()), err)
	}
	return IntentResponse{
		Handled:  true,
		Response: fmt.Sprintf("已创建%s「%s」。", spaceType, name),
	}
}

func (s *SpaceMgmtIntent) handleRename(ictx IntentContext) IntentResponse {
	oldName := entityString(ictx.Result.Entities, "space_name")
	newName := entityString(ictx.Result.Entities, "new_name")
	if oldName == "" || newName == "" {
		return IntentResponse{Handled: false}
	}

	space, err := s.store.FindByName(oldName)
	if err != nil {
		if err == data.ErrRecordNotFound {
			return IntentResponse{Handled: true, Response: fmt.Sprintf("未找到空间「%s」。", oldName)}
		}
		return errResponse(fmt.Sprintf("查找空间失败：%s", err.Error()), err)
	}

	space.Name = newName
	if err := s.store.Save(*space); err != nil {
		return errResponse(fmt.Sprintf("重命名空间失败：%s", err.Error()), err)
	}
	return IntentResponse{
		Handled:  true,
		Response: fmt.Sprintf("已将「%s」重命名为「%s」。", oldName, newName),
	}
}

func (s *SpaceMgmtIntent) handleQuery(ictx IntentContext) IntentResponse {
	name := entityString(ictx.Result.Entities, "space_name")

	// Query a specific space.
	if name != "" {
		space, err := s.store.FindByName(name)
		if err != nil {
			if err == data.ErrRecordNotFound {
				return IntentResponse{Handled: true, Response: fmt.Sprintf("未找到空间「%s」。", name)}
			}
			return errResponse(fmt.Sprintf("查询空间失败：%s", err.Error()), err)
		}
		childNames := make([]string, 0, len(space.Children))
		for _, c := range space.Children {
			childNames = append(childNames, c.Name)
		}
		detail := fmt.Sprintf("「%s」（类型：%s）", space.Name, space.Type)
		if len(childNames) > 0 {
			detail += fmt.Sprintf("，包含：%s", strings.Join(childNames, "、"))
		}
		return IntentResponse{Handled: true, Response: detail + "。"}
	}

	// Query all top-level spaces.
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
