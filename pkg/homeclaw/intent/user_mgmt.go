package intent

import (
	"context"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
)

// UserMgmtIntent handles user management intents (user.add, user.remove, user.query).
//
// Operations are executed directly against the MemberStore; the large-model
// agent loop is not involved.
type UserMgmtIntent struct {
	store data.MemberStore
}

// NewUserMgmtIntent creates a UserMgmtIntent backed by the given MemberStore.
// If store is nil the handler falls through to the large model for all intents.
func NewUserMgmtIntent(store data.MemberStore) *UserMgmtIntent {
	return &UserMgmtIntent{store: store}
}

// Types implements Intent.
func (u *UserMgmtIntent) Types() []IntentType {
	return []IntentType{
		IntentUserAdd,
		IntentUserRemove,
		IntentUserQuery,
	}
}

// Run executes the user management operation and returns a direct reply.
func (u *UserMgmtIntent) Run(_ context.Context, ictx IntentContext) IntentResponse {
	if u.store == nil {
		return IntentResponse{Handled: false}
	}

	switch ictx.Result.Type {
	case IntentUserAdd:
		return u.handleAdd(ictx)
	case IntentUserRemove:
		return u.handleRemove(ictx)
	case IntentUserQuery:
		return u.handleQuery(ictx)
	default:
		return IntentResponse{Handled: false}
	}
}

func (u *UserMgmtIntent) handleAdd(ictx IntentContext) IntentResponse {
	name := entityString(ictx.Result.Entities, "member_name")
	if name == "" {
		return IntentResponse{Handled: false}
	}

	member := data.Member{
		Name: name,
		Role: "member",
	}
	if err := u.store.Save(member); err != nil {
		return errResponse(fmt.Sprintf("添加成员「%s」失败：%s", name, err.Error()), err)
	}
	return IntentResponse{
		Handled:  true,
		Response: fmt.Sprintf("已添加家庭成员「%s」。", name),
	}
}

func (u *UserMgmtIntent) handleRemove(ictx IntentContext) IntentResponse {
	name := entityString(ictx.Result.Entities, "member_name")
	if name == "" {
		return IntentResponse{Handled: false}
	}

	if err := u.store.Delete(name); err != nil {
		if err == data.ErrRecordNotFound {
			return IntentResponse{Handled: true, Response: fmt.Sprintf("未找到成员「%s」。", name)}
		}
		return errResponse(fmt.Sprintf("删除成员「%s」失败：%s", name, err.Error()), err)
	}
	return IntentResponse{
		Handled:  true,
		Response: fmt.Sprintf("已删除家庭成员「%s」。", name),
	}
}

func (u *UserMgmtIntent) handleQuery(ictx IntentContext) IntentResponse {
	name := entityString(ictx.Result.Entities, "member_name")

	// Query a specific member.
	if name != "" {
		members, err := u.store.GetAll()
		if err != nil {
			return errResponse(fmt.Sprintf("查询成员失败：%s", err.Error()), err)
		}
		for _, m := range members {
			if strings.EqualFold(m.Name, name) {
				return IntentResponse{
					Handled:  true,
					Response: fmt.Sprintf("成员「%s」，角色：%s。", m.Name, m.Role),
				}
			}
		}
		return IntentResponse{Handled: true, Response: fmt.Sprintf("未找到成员「%s」。", name)}
	}

	// Query all members.
	members, err := u.store.GetAll()
	if err != nil {
		return errResponse(fmt.Sprintf("查询成员列表失败：%s", err.Error()), err)
	}
	if len(members) == 0 {
		return IntentResponse{Handled: true, Response: "当前没有任何家庭成员。"}
	}
	names := make([]string, 0, len(members))
	for _, m := range members {
		names = append(names, fmt.Sprintf("%s（%s）", m.Name, m.Role))
	}
	return IntentResponse{
		Handled:  true,
		Response: fmt.Sprintf("家庭成员共 %d 人：%s。", len(members), strings.Join(names, "、")),
	}
}
