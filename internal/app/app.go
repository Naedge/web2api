package app

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"web2api/internal/config"
	"web2api/internal/handler"
	"web2api/internal/middleware"
	"web2api/internal/repository"
	"web2api/internal/router"
	"web2api/internal/service"
	"web2api/internal/storage"
)

func Run() error {
	cfg, err := config.Load("config.json")
	if err != nil {
		return err
	}

	db, err := storage.Open(cfg)
	if err != nil {
		return err
	}

	adminRepo := repository.NewAdminUserRepository(db)
	accountRepo := repository.NewAccountRepository(db)
	cpaRepo := repository.NewCPAPoolRepository(db)
	proxyRepo := repository.NewProxySettingRepository(db)

	authService := service.NewAuthService(adminRepo, cfg.SessionSecret)
	proxyService := service.NewProxyService(proxyRepo)
	accountService := service.NewAccountService(accountRepo, proxyService, cfg.TLSVerify)
	imageUpstream := service.NewImageUpstreamService(accountService, proxyService, cfg.TLSVerify)
	chatService := service.NewChatService(accountService, imageUpstream)
	cpaService := service.NewCPAService(cpaRepo, accountService, cfg.TLSVerify)

	authMiddleware := middleware.NewAuthMiddleware(cfg.APIKey, authService)
	webHandler := handler.NewWebHandler()
	authHandler := handler.NewAuthHandler(authService)
	systemHandler := handler.NewSystemHandler(chatService)
	proxyHandler := handler.NewProxyHandler(proxyService)
	accountHandler := handler.NewAccountHandler(accountService)
	cpaHandler := handler.NewCPAHandler(cpaService)
	imageHandler := handler.NewImageHandler(chatService)

	engine := router.New(router.Dependencies{
		Auth:    authMiddleware,
		Web:     webHandler,
		AuthUI:  authHandler,
		System:  systemHandler,
		Proxy:   proxyHandler,
		Account: accountHandler,
		CPA:     cpaHandler,
		Image:   imageHandler,
	})

	server := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           engine,
		ReadHeaderTimeout: 15 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go startAccountRefreshLoop(ctx, accountService, time.Duration(cfg.RefreshAccountIntervalMinute)*time.Minute)

	errCh := make(chan error, 1)
	go func() {
		log.Printf("web2api listening on %s", cfg.Addr())
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

func startAccountRefreshLoop(
	ctx context.Context,
	accountService *service.AccountService,
	interval time.Duration,
) {
	if interval <= 0 {
		interval = 30 * time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = accountService.RefreshLimitedAccounts(context.Background())
		}
	}
}
