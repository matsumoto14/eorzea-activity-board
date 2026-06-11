// Package scheduler は毎日定時に処理を実行する最小スケジューラ。
package scheduler

import (
	"context"
	"time"
)

// RunDaily は loc のタイムゾーンで毎日 hour:minute に fn を呼ぶ。
// ctx がキャンセルされるまでブロックする。
func RunDaily(ctx context.Context, loc *time.Location, hour, minute int, fn func()) {
	for {
		now := time.Now().In(loc)
		next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, loc)
		if !next.After(now) {
			next = next.AddDate(0, 0, 1)
		}
		timer := time.NewTimer(next.Sub(now))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			fn()
		}
	}
}
