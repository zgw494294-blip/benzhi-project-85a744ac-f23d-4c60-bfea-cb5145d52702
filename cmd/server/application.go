package main

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"

	"stage-rig-clearance/internal/audit"
	"stage-rig-clearance/internal/httpapi"
	"stage-rig-clearance/internal/rigging"
	"stage-rig-clearance/internal/store"
)

type application struct {
	repository *store.Repository
	auditor    *audit.Store
	httpServer *http.Server
}

func buildApplication(cfg config) (*application, error) {
	repository, err := store.Open(filepath.Join(cfg.DataDir, "domain"))
	if err != nil {
		return nil, fmt.Errorf("open domain store: %w", err)
	}
	auditor, err := audit.Open(filepath.Join(cfg.DataDir, "audit"))
	if err != nil {
		_ = repository.Close()
		return nil, fmt.Errorf("open audit store: %w", err)
	}
	service := rigging.NewService(repository, auditor)
	api := httpapi.New(service)
	server := &http.Server{
		Addr: cfg.Addr, Handler: api.Handler(), ReadHeaderTimeout: defaultReadHeaderTimeout,
		ReadTimeout: defaultReadTimeout, WriteTimeout: defaultWriteTimeout, IdleTimeout: defaultIdleTimeout,
		MaxHeaderBytes: 32 << 10,
	}
	return &application{repository: repository, auditor: auditor, httpServer: server}, nil
}

func (a *application) close() error {
	return errors.Join(a.repository.Close(), a.auditor.Close())
}
