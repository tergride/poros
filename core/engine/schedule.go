package engine

import (
	"context"
	"log"
	"time"
)

const (
	// defaultUpdateInterval is for providers that advise none, as in the Qt client.
	defaultUpdateInterval = 6 * time.Hour
	// retryInterval is the wait after a failed update, unless the interval is shorter.
	retryInterval = time.Hour
	scheduleTick  = time.Minute
)

// StartSchedule updates the subscriptions that are due and looks for a new Poros, checking once a minute until Close.
func (e *Engine) StartSchedule() {
	ctx, cancel := context.WithCancel(context.Background())
	e.stop = cancel
	e.stopped.Add(1)
	go func() {
		defer e.stopped.Done()
		ticker := time.NewTicker(scheduleTick)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				e.updateDue(ctx)
				e.checkUpdatesIfDue()
			}
		}
	}()
}

// updateDue updates, one at a time, each subscription whose time has come; groups of the user's have no updates.
func (e *Engine) updateDue(ctx context.Context) {
	settings, err := e.Settings()
	if err != nil {
		return
	}
	list, err := e.groups()
	if err != nil {
		return
	}
	now := e.now()
	for _, rec := range list {
		if ctx.Err() != nil {
			return
		}
		if rec.manual() || !updatesItself(rec, settings) || now.Before(dueAt(rec, settings)) {
			continue
		}
		// A failure is kept on the subscription for the interface to show.
		if _, err := e.UpdateSubscription(ctx, rec.ID); err != nil && ctx.Err() == nil {
			log.Printf("subscription %s: update failed: %v", rec.ID, err)
		}
	}
}

// updatesItself tells whether a subscription updates on a schedule: as its group says, or as the settings of all
// subscriptions do.
func updatesItself(rec groupRecord, settings Settings) bool {
	switch rec.Settings.Update {
	case GroupUpdateNever:
		return false
	case GroupUpdateProvider, GroupUpdateHours:
		return true
	}
	return settings.SubscriptionAutoUpdate
}

// dueAt is when a subscription is updated next: an interval after the last update, or sooner after a failure.
func dueAt(rec groupRecord, settings Settings) time.Time {
	interval := defaultUpdateInterval
	hours := settings.SubscriptionUpdateHours
	switch rec.Settings.Update {
	case GroupUpdateProvider:
		hours = 0
	case GroupUpdateHours:
		hours = rec.Settings.UpdateHours
	}
	switch {
	case hours > 0:
		interval = time.Duration(hours) * time.Hour
	case rec.Info.UpdateInterval > 0:
		interval = rec.Info.UpdateInterval
	}
	due := time.UnixMilli(rec.UpdatedAt).Add(interval)
	if rec.CheckedAt > rec.UpdatedAt {
		due = time.UnixMilli(rec.CheckedAt).Add(min(interval, retryInterval))
	}
	return due
}
