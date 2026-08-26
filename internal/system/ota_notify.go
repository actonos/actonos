package system

import "context"

// UpdateNotifier is the subset of NotificationManager used by the 24h ticker.
type UpdateNotifier interface {
	Create(ctx context.Context, notif Notification) (*Notification, error)
}

// MaybeNotify creates at most one system notification per new latest_version.
// No notify on error_code, and no notify when update_available is false.
func (o *OTAEngine) MaybeNotify(ctx context.Context, result *CheckResult, notifier UpdateNotifier) error {
	if notifier == nil || result == nil {
		return nil
	}
	if result.ErrorCode != "" || !result.UpdateAvailable {
		return nil
	}
	if result.LatestVersion == "" || result.LatestVersion == o.LastNotifiedVersion() {
		return nil
	}
	_, err := notifier.Create(ctx, Notification{
		Title:    "ActonOS update available",
		Message:  "Version " + result.LatestVersion + " is available (running " + result.CurrentVersion + ").",
		Type:     "info",
		Category: "system",
		Link:     "#/settings?view=maintenance",
	})
	if err != nil {
		return err
	}
	o.SetLastNotifiedVersion(result.LatestVersion)
	return nil
}
