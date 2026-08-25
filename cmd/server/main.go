package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 10 * time.Second
	defaultWriteTimeout      = 15 * time.Second
	defaultIdleTimeout       = 60 * time.Second
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	cfg, err := parseConfig(args, os.Getenv)
	if err != nil {
		return err
	}
	if cfg.Selfcheck {
		temp, err := os.MkdirTemp("", "stage-rig-selfcheck-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(temp)
		cfg.DataDir = temp
	}
	app, err := buildApplication(cfg)
	if err != nil {
		return err
	}
	defer app.close()
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.Addr, err)
	}
	if cfg.Selfcheck {
		return runSelfcheck(app, listener)
	}
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- app.httpServer.Serve(listener) }()
	log.Printf("舞台吊挂安全放行工作台已监听 http://%s", cfg.Addr)
	signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-serveErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-signalContext.Done():
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return app.httpServer.Shutdown(ctx)
	}
}
