package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"time"

	"backend/config"
	"backend/processing"
	"backend/routes"
	"backend/utils"
	"github.com/gin-gonic/gin"
)

func main() {
	if err := utils.InitMySQL(); err != nil {
		log.Fatal("MySQL 连接失败，请检查 CWATCH_DSN: ", err)
	}
	if err := utils.Migrate(); err != nil {
		log.Fatal("模型迁移失败: ", err)
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
	if err != nil || workers < 0 || workers > 8 {
		log.Fatal("CWATCH_WORKERS 必须是 0 到 8 之间的整数")
	}

	router := gin.Default()
	router.SetTrustedProxies(nil)
	routes.SetupRoutes(router)
	webDir := config.Value("CWATCH_WEB_DIR", "../web")
	router.Static("/css", filepath.Join(webDir, "css"))
	router.Static("/js", filepath.Join(webDir, "js"))
	router.StaticFile("/", filepath.Join(webDir, "index.html"))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	rankingDone := make(chan struct{})
	go func() {
		defer close(rankingDone)
		utils.RunHotVideoRanking(ctx)
	}()
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		processing.Run(ctx, workers)
	}()

	addr := config.Value("CWATCH_ADDR", "127.0.0.1:5000")
	server := &http.Server{Addr: addr, Handler: router, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: time.Minute}
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.ListenAndServe() }()
	log.Printf("Cwatch 监听 %s", addr)
	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP 服务停止: %v", err)
		}
		stop()
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
		}
	}
	<-workerDone
	<-rankingDone
}
