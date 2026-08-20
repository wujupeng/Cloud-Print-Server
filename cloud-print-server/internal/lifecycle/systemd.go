package lifecycle

import (
	"context"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
	"go.uber.org/zap"
)

func NotifyReady() error {
	sent, err := daemon.SdNotify(false, daemon.SdNotifyReady)
	if err != nil {
		return err
	}
	if !sent {
		return nil
	}
	return nil
}

func NotifyStopping() error {
	_, err := daemon.SdNotify(false, daemon.SdNotifyStopping)
	return err
}

func NotifyReloading() error {
	_, err := daemon.SdNotify(false, daemon.SdNotifyReloading)
	return err
}

func StartWatchdog(ctx context.Context, logger *zap.Logger) {
	interval, err := daemon.SdWatchdogEnabled(false)
	if err != nil || interval == 0 {
		return
	}
	if interval > 0 {
		interval = interval / 2
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Info("systemd watchdog 启动", zap.Duration("interval", interval))

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := daemon.SdNotify(false, daemon.SdNotifyWatchdog); err != nil {
				logger.Warn("watchdog notify 失败", zap.Error(err))
			}
		}
	}
}