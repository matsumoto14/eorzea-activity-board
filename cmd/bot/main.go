// eorzea-activity-board: 身内 FF14 グループ向けのゆるマッチング Discord Bot。
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // distroless 等 tzdata の無い環境向けに埋め込む

	"eorzea-activity-board/internal/bot"
	"eorzea-activity-board/internal/config"
	"eorzea-activity-board/internal/scheduler"
	"eorzea-activity-board/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("起動失敗", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		return err
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	b, err := bot.New(cfg.Token, st, loc)
	if err != nil {
		return err
	}
	if err := b.Open(); err != nil {
		return err
	}
	defer b.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	slog.Info("起動完了", "post_time", time.Date(0, 1, 1, cfg.PostHour, cfg.PostMinute, 0, 0, loc).Format("15:04"))
	scheduler.RunDaily(ctx, loc, cfg.PostHour, cfg.PostMinute, b.PostDailyProposals)
	slog.Info("シャットダウンします")
	return nil
}
