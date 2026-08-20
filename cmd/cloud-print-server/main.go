package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cloud-print/server/internal/lifecycle"
)

func main() {
	configPath := flag.String("config", "/etc/cloud-print-server/config.yaml", "配置文件路径")
	flag.Parse()

	if err := lifecycle.Run(*configPath); err != nil {
		fmt.Fprintf(os.Stderr, "cloud-print-server 启动失败: %v\n", err)
		os.Exit(1)
	}
}