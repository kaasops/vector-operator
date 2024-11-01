package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/kaasops/vector-operator/internal/evcollector"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	"k8s.io/client-go/kubernetes"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	ctrl "sigs.k8s.io/controller-runtime"
	"syscall"
)

const (
	configPath = "/etc/event-collector/config.json"
	port       = "8080"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	log.Info("starting kubernetes-events-collector")

	// kubernetes client
	config, err := ctrl.GetConfig()
	if err != nil {
		log.Error("unable to load kubernetes config", "error", err)
		os.Exit(1)
	}
	log.Info("kubernetes config loaded")

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Error("unable to create clientset")
		os.Exit(1)
	}

	log.Info("kubernetes clientset created")

	// config
	v := viper.New()
	v.SetConfigFile(configPath)

	if err := v.ReadInConfig(); err != nil {
		log.Error("failed to read config file", "error", err)
		os.Exit(1)
	}

	var cfg evcollector.Config
	store := make(map[string]*evcollector.Collector)

	setupCollector := func(svcName, svcNamespace, port, watchedNamespace string) {
		host := fmt.Sprintf("%s.%s", svcName, svcNamespace)
		addr := net.JoinHostPort(host, port)

		if oldC, ok := store[host]; ok {
			if oldC.Addr == addr && oldC.Namespace == watchedNamespace {
				return
			}
			oldC.Stop()
		}

		c := evcollector.New(addr, watchedNamespace, cfg.MaxBatchSize, log, clientset.CoreV1().RESTClient())
		store[host] = c
		c.Start()
	}

	applyConfig := func() {
		if err := v.Unmarshal(&cfg); err != nil {
			log.Error("failed to unmarshal config", "error", err)
			os.Exit(1)
		}

		notModified := make(map[string]struct{}, len(store))
		for k := range store {
			notModified[k] = struct{}{}
		}

		for _, r := range cfg.Receivers {
			setupCollector(r.ServiceName, r.ServiceNamespace, r.Port, r.WatchedNamespace)
			delete(notModified, fmt.Sprintf("%s.%s", r.ServiceName, r.ServiceNamespace))
		}

		for k := range notModified {
			store[k].Stop()
			delete(store, k)
		}
	}

	applyConfig()

	log.Info("config loaded", "config", cfg)

	v.WatchConfig()
	v.OnConfigChange(func(_ fsnotify.Event) {
		log.Info("config file changed")
		applyConfig()
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	http.Handle("/metrics", promhttp.Handler())
	go func() {
		if err = http.ListenAndServe(net.JoinHostPort("", port), nil); err != nil && !errors.Is(http.ErrServerClosed, err) {
			log.Error("failed to start http server", "error", err)
			os.Exit(1)
		}
	}()
	log.Info("starting http server on port " + port)

	<-ctx.Done()
	log.Info("shutting down")

	for k := range store {
		store[k].Stop()
		delete(store, k)
	}
}
