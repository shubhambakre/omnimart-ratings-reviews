package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shubhambakre/omnimart-ratings-reviews/internal/config"
	"github.com/shubhambakre/omnimart-ratings-reviews/internal/moderation"
	"github.com/shubhambakre/omnimart-ratings-reviews/internal/repository"
	"github.com/shubhambakre/omnimart-ratings-reviews/internal/repository/memory"
	sqliterepo "github.com/shubhambakre/omnimart-ratings-reviews/internal/repository/sqlite"
	"github.com/shubhambakre/omnimart-ratings-reviews/internal/service"
	"github.com/shubhambakre/omnimart-ratings-reviews/internal/transport/http/middleware"
	"github.com/shubhambakre/omnimart-ratings-reviews/internal/transport/http/nonsitefacing"
	"github.com/shubhambakre/omnimart-ratings-reviews/internal/transport/http/sitefacing"
)

func main() {
	cfg := config.Load()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	reviewRepo, ratingRepo := mustBuildRepos(cfg, log)
	mod := moderation.NewSimpleModerator()

	ratingSvc := service.NewRatingService(reviewRepo, ratingRepo, log)
	reviewSvc := service.NewReviewService(reviewRepo, ratingRepo, mod, ratingSvc, log)

	if cfg.Seed {
		seed(reviewSvc)
	}

	siteSrv := buildSiteServer(cfg, reviewSvc, ratingSvc, log)
	intSrv := buildInternalServer(cfg, reviewSvc, ratingSvc, log)

	go listen("site-facing", siteSrv, log)
	go listen("internal", intSrv, log)
	log.Info("servers up", "site", cfg.SiteAddr, "internal", cfg.InternalAddr)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = siteSrv.Shutdown(ctx)
	_ = intSrv.Shutdown(ctx)
}

func buildSiteServer(cfg config.Config, rs *service.ReviewService, rt *service.RatingService, log *slog.Logger) *http.Server {
	mux := http.NewServeMux()
	(&sitefacing.Handler{Reviews: rs, Ratings: rt}).Mount(mux)

	h := middleware.Chain(mux,
		middleware.RequestID,
		middleware.Logger(log),
		middleware.Recover(log),
	)
	return &http.Server{
		Addr:              cfg.SiteAddr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func buildInternalServer(cfg config.Config, rs *service.ReviewService, rt *service.RatingService, log *slog.Logger) *http.Server {
	mux := http.NewServeMux()
	(&nonsitefacing.Handler{Reviews: rs, Ratings: rt}).Mount(mux)

	// Health bypasses API key so liveness probes work without secrets.
	gated := middleware.Chain(mux,
		middleware.RequestID,
		middleware.Logger(log),
		middleware.Recover(log),
		gatedExceptHealth(cfg.InternalKey),
	)
	return &http.Server{
		Addr:              cfg.InternalAddr,
		Handler:           gated,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func gatedExceptHealth(key string) func(http.Handler) http.Handler {
	apiKey := middleware.APIKeyAuth(key)
	return func(next http.Handler) http.Handler {
		gated := apiKey(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}
			gated.ServeHTTP(w, r)
		})
	}
}

func mustBuildRepos(cfg config.Config, log *slog.Logger) (repository.ReviewRepository, repository.RatingRepository) {
	switch cfg.StorageDriver {
	case "sqlite":
		log.Info("storage driver", "driver", "sqlite", "path", cfg.StoragePath)
		rr, rt, err := sqliterepo.NewRepos(cfg.StoragePath)
		if err != nil {
			log.Error("sqlite init failed", "err", err)
			os.Exit(1)
		}
		return rr, rt
	case "memory", "":
		log.Info("storage driver", "driver", "memory")
		return memory.NewReviewRepo(), memory.NewRatingRepo()
	default:
		log.Error("unknown storage driver", "driver", cfg.StorageDriver, "valid", "memory, sqlite")
		os.Exit(1)
		panic("unreachable") // satisfies compiler; os.Exit above terminates the process
	}
}

func listen(name string, s *http.Server, log *slog.Logger) {
	if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server error", "name", name, "err", err)
		os.Exit(1)
	}
}

func seed(rs *service.ReviewService) {
	ctx := context.Background()
	samples := []service.SubmitReviewInput{
		{ProductID: "PROD-1", CustomerID: "C-1", Rating: 5, Title: "Great", Body: "Loved the product, holds up well.", VerifiedPurchase: true},
		{ProductID: "PROD-1", CustomerID: "C-2", Rating: 4, Title: "Solid", Body: "Good quality for the price.", VerifiedPurchase: true},
		{ProductID: "PROD-1", CustomerID: "C-3", Rating: 2, Title: "Meh", Body: "Smaller than expected, but works fine.", VerifiedPurchase: false},
		{ProductID: "PROD-2", CustomerID: "C-4", Rating: 5, Title: "Excellent", Body: "Five stars, would buy again.", VerifiedPurchase: true},
	}
	for _, s := range samples {
		_, _ = rs.Submit(ctx, s)
	}
}
