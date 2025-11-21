package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/kaasops/vector-operator/internal/buildinfo"
	"github.com/kaasops/vector-operator/internal/evcollector"
)

func main() {
	configPath := flag.String("config", "/etc/event-collector/config.json", "path to config file") // data is taken from a ConfigMap created by the Vector operator
	port := flag.String("port", "8080", "port to listen on")
	logLevel := flag.String("log-level", "info", "log level (debug, info, warn, error)")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLogLevel(*logLevel)}))
	log.Info("build info", "version", buildinfo.Version)
	log.Info("starting kubernetes-events-collector")

	// kubernetes client
	k8sCfg, err := ctrl.GetConfig()
	if err != nil {
		log.Error("unable to load kubernetes config", "error", err)
		os.Exit(1)
	}
	log.Info("kubernetes config loaded")

	clientset, err := kubernetes.NewForConfig(k8sCfg)
	if err != nil {
		log.Error("unable to create clientset")
		os.Exit(1)
	}

	log.Info("kubernetes clientset created")

	// config
	v := viper.New()
	v.SetConfigFile(*configPath)

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
		if err = http.ListenAndServe(net.JoinHostPort("", *port), nil); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("failed to start http server", "error", err)
			os.Exit(1)
		}
	}()
	log.Info("starting http server on port " + *port)

	<-ctx.Done()
	log.Info("shutting down")

	for k := range store {
		store[k].Stop()
		delete(store, k)
	}
}

func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
