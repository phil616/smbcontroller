package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"smb-controller/internal/config"
	"smb-controller/internal/database"
	"smb-controller/internal/handler"
	"smb-controller/internal/repository"
	"smb-controller/internal/service"
	"smb-controller/internal/session"
	"smb-controller/internal/smb"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:embed web
var embeddedWeb embed.FS

func main() {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		log.Fatalf("failed to load Asia/Shanghai timezone: %v", err)
	}
	time.Local = loc

	configPath := flag.String("config", "/etc/smb-controller/config.yaml", "config file path")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	db, err := database.New(cfg.Database.Path)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	detector := smb.NewDetector()
	detectResult := detector.Detect(cfg.Smb.ConfPath)
	log.Printf("SMB detection: smbd_installed=%v smbd_running=%v conf=%s", detectResult.SmbdInstalled, detectResult.SmbdRunning, detectResult.ConfPath)

	repos := repository.NewRepositories(db)
	executor := smb.NewExecutor(cfg.Smb)
	generator := smb.NewGenerator(cfg.Smb.ConfPath, cfg.Smb.BackupDir, cfg.Smb.BackupMaxCount)
	sessionStore := session.NewStore(time.Duration(cfg.Session.TTLHours) * time.Hour)
	services := service.NewServices(repos, executor, generator, detector, detectResult, cfg.Smb.AllowedShareRoots)

	stopSessions := make(chan struct{})
	go sessionStore.Cleanup(stopSessions)
	defer close(stopSessions)
	stopTemporaryUsers := make(chan struct{})
	go cleanupTemporaryUsers(services, stopTemporaryUsers)
	defer close(stopTemporaryUsers)

	webSubFS, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		log.Fatalf("failed to load embedded web fs: %v", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	handler.RegisterRoutes(r, services, sessionStore, webSubFS, cfg.Server.Domain)

	srv := &http.Server{
		Addr:         cfg.Server.Listen,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("SMB Controller listening on %s timezone=%s", cfg.Server.Listen, time.Local.String())
		var err error
		if cfg.Server.TLSCert != "" && cfg.Server.TLSKey != "" {
			err = srv.ListenAndServeTLS(cfg.Server.TLSCert, cfg.Server.TLSKey)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
}

func cleanupTemporaryUsers(services *service.Services, stop <-chan struct{}) {
	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if count, err := services.SMB.CleanupExpiredTemporaryUsers(ctx); err != nil {
			log.Printf("temporary user cleanup failed: %v", err)
		} else if count > 0 {
			log.Printf("cleaned up %d expired temporary SMB users", count)
		}
	}
	run()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			run()
		case <-stop:
			return
		}
	}
}
