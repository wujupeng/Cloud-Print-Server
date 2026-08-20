package lifecycle

import (
	"os"
	"os/signal"
	"syscall"
)

func WaitForSignal() chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	return ch
}