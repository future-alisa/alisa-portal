package main

import (
	"alisa/portal/internal/portal"
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	store, err := portal.Open(os.Getenv("DATA_FILE"))
	if err != nil {
		log.Fatal(err)
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	auth, err := portal.OAuthFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		log.Fatal("invalid LISTEN_ADDR")
	}
	ip := net.ParseIP(host)
	if (ip == nil || !ip.IsLoopback()) && os.Getenv("API_TOKEN") == "" && auth == nil {
		log.Fatal("API_TOKEN is required when listening outside loopback")
	}
	srv := &http.Server{Addr: addr, Handler: portal.HandlerWithAuth(store, auth), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		log.Printf("portal API listening on %s", addr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
}
