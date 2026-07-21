package main

import (
	"encoding/json"
	"net/http"
	"time"
)

const listenAddr = ":8080"

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"service": "opencloud-phase0-spike",
			"status":  "ok",
		})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><title>OpenCloud Phase 0</title><h1>Docker + Caddy provisioning spike</h1>"))
	})

	server := &http.Server{
		Addr:              listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		panic(err)
	}
}
