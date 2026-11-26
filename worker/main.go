package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"

	"backend/config"
	"backend/processing"
	"backend/utils"
)

func main() {
	if err := utils.InitMySQL(); err != nil {
		log.Fatal("MySQL 连接失败，请检查 CWATCH_DSN: ", err)
	}
	if err := utils.InitMedia(); err != nil {
		log.Fatal("视频目录初始化失败: ", err)
	}
	closeCache, err := utils.InitRedisVideoCache()
	if err != nil {
		log.Fatal(err)
	}
	defer closeCache()

	workers, err := strconv.Atoi(config.Value("CWATCH_WORKERS", "1"))
	if err != nil || workers < 1 || workers > 8 {
		log.Fatal("CWATCH_WORKERS 必须是 1 到 8 之间的整数")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	processing.Run(ctx, workers)
}
