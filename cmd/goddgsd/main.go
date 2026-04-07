package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/velariumai/goddgs"
)

var serveFn = func(s *http.Server) error { return s.ListenAndServe() }

func main() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	if err := run(stop); err != nil {
		log.Fatal(err)
	}
}

func run(stop <-chan os.Signal) error {
	cfg := goddgs.LoadConfigFromEnv()
	metrics := goddgs.NewPrometheusCollector(nil)
	engine, err := goddgs.NewDefaultEngineFromConfig(cfg, metrics.Hook)
	if err != nil {
		return fmt.Errorf("engine init: %w", err)
	}
	for _, p := range []string{"ddg", "brave", "tavily", "serpapi"} {
		en := false
		for _, ep := range engine.EnabledProviders() {
			if ep == p {
				en = true
				break
			}
		}
		metrics.SetProviderEnabled(p, en)
	}

	mux := goddgs.NewHTTPHandler(engine, cfg, promhttp.Handler())

	addr := getenv("GODDGS_ADDR", ":8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("goddgsd listening on %s", addr)
		if err := serveFn(srv); err != nil && err != http.ErrServerClosed {
			log.Printf("server error: %v", err)
		}
	}()
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

func getenv(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}
