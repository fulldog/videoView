package app

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"videoview/internal/config"
	"videoview/internal/handler"
	"videoview/internal/logger"
	"videoview/internal/middleware"
	"videoview/internal/repository"
	"videoview/internal/service"
	"videoview/internal/transcode"
	"videoview/internal/upload"
)

func Run() error {
	cfgPath := flag.String("config", "configs/config.yaml", "config file path")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}

	log, err := logger.New(cfg.Log, cfg.IsDev())
	if err != nil {
		return err
	}
	defer func() { _ = log.Sync() }()

	if !cfg.IsDev() {
		gin.SetMode(gin.ReleaseMode)
		gin.DisableConsoleColor()
	}

	db, err := repository.OpenMySQL(cfg.MySQL.DSN, log, cfg.IsDev())
	if err != nil {
		return err
	}
	if err := repository.AutoMigrate(db); err != nil {
		return err
	}
	if err := repository.Seed(context.Background(), db); err != nil {
		return err
	}

	store, err := upload.NewStore(cfg.Storage.Root)
	if err != nil {
		return err
	}

	userRepo := repository.NewUserRepo(db)
	roleRepo := repository.NewRoleRepo(db)
	permRepo := repository.NewPermRepo(db)
	resetRepo := repository.NewResetRepo(db)
	uploadRepo := repository.NewUploadRepo(db)
	videoRepo := repository.NewVideoRepo(db)

	worker := transcode.NewWorker(cfg.Transcode, store, videoRepo, log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker.Start(ctx)

	authSvc := service.NewAuthService(userRepo, roleRepo, resetRepo, cfg)
	rbacSvc := service.NewRBACService(userRepo, roleRepo, permRepo)
	uploadSvc := service.NewUploadService(uploadRepo, videoRepo, store, worker)
	videoSvc := service.NewVideoService(videoRepo)

	engine := gin.New()
	handler.RegisterRoutes(engine, handler.Deps{
		Auth:    handler.NewAuthHandler(authSvc),
		RBAC:    handler.NewRBACHandler(rbacSvc),
		Uploads: handler.NewUploadHandler(uploadSvc),
		Videos:  handler.NewVideoHandler(videoSvc),
		Users:   userRepo,
		Secret:  cfg.JWT.Secret,
		IsDev:   cfg.IsDev(),
	}, middleware.RequestLog(log), middleware.Recovery(log))

	engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	srv := &http.Server{
		Addr:              cfg.App.Addr,
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("server start", zap.String("addr", cfg.App.Addr), zap.String("mode", cfg.App.Mode))
		errCh <- srv.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case sig := <-sigCh:
		log.Info("shutdown", zap.String("signal", sig.String()))
	}

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	return srv.Shutdown(shutdownCtx)
}
