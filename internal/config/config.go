// Package config は環境変数からの設定読み込み。
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Token      string // DISCORD_TOKEN(必須)
	DBPath     string // EAB_DB_PATH(既定 data/eab.db)
	PostHour   int    // EAB_POST_HOUR(既定 20)
	PostMinute int    // EAB_POST_MINUTE(既定 0)
}

func Load() (*Config, error) {
	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		return nil, errors.New("環境変数 DISCORD_TOKEN を設定してください")
	}

	cfg := &Config{
		Token:      token,
		DBPath:     envOr("EAB_DB_PATH", "data/eab.db"),
		PostHour:   20,
		PostMinute: 0,
	}

	var err error
	if cfg.PostHour, err = envIntOr("EAB_POST_HOUR", 20); err != nil {
		return nil, err
	}
	if cfg.PostMinute, err = envIntOr("EAB_POST_MINUTE", 0); err != nil {
		return nil, err
	}
	// time.Date は範囲外を正規化して(25時→翌1時)黙って時刻がずれるため、ここで弾く
	if cfg.PostHour < 0 || cfg.PostHour > 23 {
		return nil, fmt.Errorf("EAB_POST_HOUR は 0〜23 で指定してください: %d", cfg.PostHour)
	}
	if cfg.PostMinute < 0 || cfg.PostMinute > 59 {
		return nil, fmt.Errorf("EAB_POST_MINUTE は 0〜59 で指定してください: %d", cfg.PostMinute)
	}
	return cfg, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntOr(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s が数値ではありません: %q", key, v)
	}
	return n, nil
}
