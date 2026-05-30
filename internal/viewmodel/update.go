package viewmodel

import "github.com/valueforvalue/DixieData/internal/update"

func UpdateSettingsFromDomain(state update.SettingsState) UpdateSettings {
	var lastApply *UpdateApplyStatus
	if state.LastApply != nil {
		lastApply = &UpdateApplyStatus{
			Status:    state.LastApply.Status,
			Version:   state.LastApply.Version,
			Message:   state.LastApply.Message,
			AppliedAt: state.LastApply.AppliedAt,
		}
	}
	return UpdateSettings{
		CurrentVersion:     state.CurrentVersion,
		BuildIdentity:      state.BuildIdentity,
		SourceURL:          state.SourceURL,
		EffectiveSourceURL: state.EffectiveSourceURL,
		UsingDefaultSource: state.UsingDefaultSource,
		CanApply:           state.CanApply,
		DisabledReason:     state.DisabledReason,
		LastApply:          lastApply,
		NoticeMessage:      state.NoticeMessage,
		NoticeKind:         state.NoticeKind,
	}
}

func UpdateCheckResultFromDomain(result update.CheckResult) UpdateCheckResult {
	return UpdateCheckResult{
		CurrentVersion:   result.CurrentVersion,
		AvailableVersion: result.AvailableVersion,
		UpdateAvailable:  result.UpdateAvailable,
		DownloadURL:      result.DownloadURL,
		NotesURL:         result.NotesURL,
		ReleaseNotes:     result.ReleaseNotes,
		PublishedAt:      result.PublishedAt,
		SourceLabel:      result.SourceLabel,
		CanApply:         result.CanApply,
		DisabledReason:   result.DisabledReason,
	}
}
