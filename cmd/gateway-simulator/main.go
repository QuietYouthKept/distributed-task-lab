package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	key, err := secretFromEnv("GATEWAY_SIMULATOR_API_KEY")
	if err != nil {
		logger.Error("invalid gateway simulator secret configuration", "error", err)
		os.Exit(2)
	}
	mode := os.Getenv("GATEWAY_SIMULATOR_MODE")
	delay := 0 * time.Millisecond
	if raw := os.Getenv("GATEWAY_SIMULATOR_DELAY"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			logger.Error("invalid gateway simulator delay")
			os.Exit(2)
		}
		delay = parsed
	}
	limit := 10000
	if raw := os.Getenv("GATEWAY_SIMULATOR_IDEMPOTENCY_LIMIT"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			logger.Error("invalid gateway simulator idempotency limit")
			os.Exit(2)
		}
		limit = parsed
	}
	simulator, err := newGatewaySimulator(key, mode, delay, limit)
	if err != nil {
		logger.Error("invalid gateway simulator configuration", "error", err)
		os.Exit(2)
	}
	addr := os.Getenv("GATEWAY_SIMULATOR_LISTEN_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8081"
	}
	server := &http.Server{Addr: addr, Handler: simulator.Handler(), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	logger.Info("local gateway simulator listening", "addr", addr, "mode", modeOrDefault(mode), "provider", "simulated")
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("gateway simulator stopped unexpectedly", "error", err)
		os.Exit(1)
	}
}

func secretFromEnv(name string) (string, error) {
	direct, file := os.Getenv(name), os.Getenv(name+"_FILE")
	if direct != "" && file != "" {
		return "", fmt.Errorf("%s and %s_FILE cannot both be set", name, name)
	}
	if file == "" {
		return direct, nil
	}
	contents, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("%s_FILE could not be read", name)
	}
	value := strings.TrimRight(string(contents), "\r\n")
	if value == "" {
		return "", fmt.Errorf("%s_FILE must not be empty", name)
	}
	return value, nil
}

func modeOrDefault(mode string) string {
	if mode == "" {
		return string(modeNormal)
	}
	return mode
}
