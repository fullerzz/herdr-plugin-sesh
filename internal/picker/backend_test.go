package picker

import (
	"context"

	"github.com/fullerzz/herdr-plugin-sesh/internal/settings"
)

type fakeBackend struct {
	closeWorkspace       func(context.Context, string) error
	reloadPicker         func(context.Context) (ReloadResult, error)
	refreshAgentStatuses func() (map[string]string, error)
	openSettings         func() (settings.Model, error)
	reloadSettings       func(context.Context, settings.Result) (DisplayOptions, ReloadResult, error)
}

func (b *fakeBackend) CloseWorkspace(ctx context.Context, id string) error {
	if b.closeWorkspace != nil {
		return b.closeWorkspace(ctx, id)
	}
	return nil
}
func (b *fakeBackend) ReloadPicker(ctx context.Context) (ReloadResult, error) {
	if b.reloadPicker != nil {
		return b.reloadPicker(ctx)
	}
	return ReloadResult{}, nil
}
func (b *fakeBackend) RefreshAgentStatuses() (map[string]string, error) {
	if b.refreshAgentStatuses != nil {
		return b.refreshAgentStatuses()
	}
	return nil, nil
}
func (b *fakeBackend) OpenSettings() (settings.Model, error) {
	if b.openSettings != nil {
		return b.openSettings()
	}
	return settings.Model{}, nil
}
func (b *fakeBackend) ReloadSettings(ctx context.Context, result settings.Result) (DisplayOptions, ReloadResult, error) {
	if b.reloadSettings != nil {
		return b.reloadSettings(ctx, result)
	}
	return DisplayOptions{}, ReloadResult{}, nil
}
