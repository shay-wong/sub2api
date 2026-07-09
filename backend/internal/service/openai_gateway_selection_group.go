package service

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

func attachSelectionGroup(ctx context.Context, result *AccountSelectionResult) *AccountSelectionResult {
	if result == nil || result.Account == nil {
		return result
	}
	if group, ok := ctx.Value(ctxkey.Group).(*Group); ok && IsGroupContextValid(group) {
		groupID := group.ID
		result.GroupID = &groupID
		result.Group = group
	}
	return result
}

func (s *OpenAIGatewayService) resolveOpenAISelectionGroupContext(ctx context.Context, groupID *int64) (context.Context, *int64, *Group, error) {
	if groupID == nil || *groupID <= 0 {
		return ctx, groupID, nil, nil
	}
	currentID := *groupID
	visited := make(map[int64]struct{})
	forcePlatform, _ := ctx.Value(ctxkey.ForcePlatform).(string)
	skipClaudeCodeOnly := forcePlatform != ""

	for {
		if _, seen := visited[currentID]; seen {
			return ctx, nil, nil, fmt.Errorf("fallback group cycle detected")
		}
		visited[currentID] = struct{}{}

		group, err := s.openAISelectionGroupByID(ctx, currentID)
		if err != nil {
			return ctx, nil, nil, err
		}
		if group == nil {
			id := currentID
			return ctx, &id, nil, nil
		}

		if !skipClaudeCodeOnly && group.ClaudeCodeOnly && !IsClaudeCodeClient(ctx) {
			if group.FallbackGroupID == nil {
				return ctx, nil, nil, ErrClaudeCodeOnly
			}
			currentID = *group.FallbackGroupID
			continue
		}

		id := group.ID
		return withOpenAISelectionGroupContext(ctx, group), &id, group, nil
	}
}

func (s *OpenAIGatewayService) openAISelectionGroupByID(ctx context.Context, groupID int64) (*Group, error) {
	if group, ok := ctx.Value(ctxkey.Group).(*Group); ok && IsGroupContextValid(group) && group.ID == groupID {
		return group, nil
	}
	if s == nil || s.schedulerSnapshot == nil {
		return nil, nil
	}
	group, err := s.schedulerSnapshot.GetGroupByID(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("get group failed: %w", err)
	}
	if !IsGroupContextValid(group) {
		return nil, nil
	}
	return group, nil
}

func withOpenAISelectionGroupContext(ctx context.Context, group *Group) context.Context {
	if !IsGroupContextValid(group) {
		return ctx
	}
	if existing, ok := ctx.Value(ctxkey.Group).(*Group); ok && IsGroupContextValid(existing) && existing.ID == group.ID {
		return ctx
	}
	return context.WithValue(ctx, ctxkey.Group, group)
}
