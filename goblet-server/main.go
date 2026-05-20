// Copyright 2021 Canva Inc
// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	_ "net/http/pprof"
	"net/url"
	"os"
	"time"

	"github.com/GetStream/goblet"
	"github.com/GetStream/goblet/github"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

var (
	config      = flag.String("config", "", "Path to Goblet's configuration file")
	checkConfig = flag.Bool("check", false, "Only checking if the config is valid, then exit")

	// latencyBoundaries (ms) used for goblet.*_processing_time / fetch_waiting_time histograms.
	latencyBoundaries = []float64{
		100, 200, 400, 800,
		1000, 2000, 4000, 8000,
		10000, 20000, 40000, 80000,
		100000, 200000, 400000, 800000,
		1000000, 2000000, 4000000, 8000000,
	}
)

// initMeterProvider wires the OTLP gRPC metrics exporter (defaults to the
// local Datadog Agent's OTLP endpoint) and installs histogram buckets that
// match the previous OpenCensus configuration. The returned shutdown
// function must be called before the program exits.
func initMeterProvider(ctx context.Context) (func(context.Context) error, error) {
	exp, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("create OTLP metric exporter: %w", err)
	}

	histogramView := sdkmetric.NewView(
		sdkmetric.Instrument{Kind: sdkmetric.InstrumentKindHistogram},
		sdkmetric.Stream{
			Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
				Boundaries: latencyBoundaries,
			},
		},
	)

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp)),
		sdkmetric.WithView(histogramView),
	)
	otel.SetMeterProvider(mp)
	return mp.Shutdown, nil
}

func FetchRepositories(config *goblet.ServerConfig, repositories []string, mustFetch bool) []error {
	errorChans := make([]chan error, 0, len(repositories))

	for _, repository := range repositories {
		errorChan := make(chan error, 1)
		errorChans = append(errorChans, errorChan)
		u, err := url.Parse(repository)
		if err != nil {
			errorChan <- err
		} else {
			goblet.FetchManagedRepositoryAsync(config, u, mustFetch, errorChan)
		}
	}

	errors := make([]error, 0)
	for _, errorChan := range errorChans {
		err := <-errorChan
		if err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}

func main() {
	flag.Parse()

	if *config == "" {
		log.Fatal("The '-config' argument is mandatory")
	}

	configFile, err := goblet.LoadConfigFile(*config)
	if err != nil {
		log.Fatalf("Couldn't load the configuration file: %v\n", err)
	}

	if *checkConfig {
		fmt.Println("Config is valid")
		return
	}

	var er = func(r *http.Request, err error) {
		log.Printf("Error while processing a request: %v", err)
	}

	var rl = func(r *http.Request, status int, requestSize, responseSize int64, latency time.Duration) {
		dump, err := httputil.DumpRequest(r, false)
		if err != nil {
			return
		}
		log.Printf("%q %d reqsize: %d, respsize %d, latency: %v", dump, status, requestSize, responseSize, latency)
	}

	var lrol = func(action string, u *url.URL) goblet.RunningOperation {
		log.Printf("Starting %s for %s", action, u.String())
		return &logBasedOperation{action, u}
	}

	ts, err := github.NewTokenSource(
		os.Getenv("GH_APP_ID"),
		os.Getenv("GH_APP_INSTALLATION_ID"),
		os.Getenv("GH_APP_PRIVATE_KEY"),
		time.Duration(configFile.TokenExpiryDeltaSeconds)*time.Second,
	)

	if err != nil {
		log.Fatal(err)
	}

	authorizer := github.NewAuthorizer(true, goblet.StatsdClient)
	defer authorizer.Close()

	config := &goblet.ServerConfig{
		LocalDiskCacheRoot:         configFile.CacheRoot,
		URLCanonicalizer:           github.URLCanonicalizer,
		RequestAuthorizer:          authorizer.RequestAuthorizer,
		TokenSource:                ts,
		ErrorReporter:              er,
		RequestLogger:              rl,
		LongRunningOperationLogger: lrol,
		PackObjectsHook:            configFile.PackObjectsHook,
		PackObjectsCache:           configFile.PackObjectsCache,
	}

	if configFile.EnableMetrics {
		log.Println("Initializing OTLP metrics exporter...")
		shutdown, err := initMeterProvider(context.Background())
		if err != nil {
			log.Fatalf("Failed to initialize the OTLP metrics exporter: %v", err)
		}
		defer func() {
			if err := shutdown(context.Background()); err != nil {
				log.Printf("OTLP metrics shutdown: %v", err)
			}
		}()
	}

	if configFile.PackObjectsHook != "" {
		if configFile.PackObjectsCache == "" {
			log.Fatalf("pack_objects_cache must be set in config, if pack_objects_hook is set.")
		}
	}

	log.Println("Initializing repositories...")
	for _, repository := range configFile.Repositories {
		u, err := url.Parse(repository)
		if err != nil {
			log.Fatalf("Failed to initialize repository '%s': %v", repository, err)
		}

		_, err = goblet.OpenManagedRepository(config, u)
		if err != nil {
			log.Fatalf("Failed to initialize repository '%s': %v", repository, err)
		}
	}

	// Pre-fetch repositories before serving any traffic. This prevents initial
	// requests from being blocked a long time until the repositories cache is
	// ready.
	log.Println("Pre-fetching repositories...")
	if errs := FetchRepositories(config, configFile.Repositories, true); len(errs) > 0 {
		for _, err := range errs {
			log.Println(err)
		}
		os.Exit(1)
	}

	// Schedule periodic upstream fetches every 15 minutes.
	log.Println("Starting background fetches...")
	cancel := goblet.RunEvery(5*time.Minute, func(t time.Time) {
		for _, err := range FetchRepositories(config, configFile.Repositories, false) {
			log.Println(err)
		}
	})
	defer cancel()

	log.Println("Registering HTTP routes...")
	http.Handle("/", goblet.HTTPHandler(config))

	http.HandleFunc("/healthz", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "ok\n")
	})

	http.HandleFunc("/authcache", authorizer.CacheMetricsHandler)

	log.Printf("Starting HTTP server on port %d...\n", configFile.Port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", configFile.Port), nil))
}

type logBasedOperation struct {
	action string
	u      *url.URL
}

func (op *logBasedOperation) Printf(format string, a ...interface{}) {
	log.Printf("Progress %s (%s): %s", op.action, op.u.String(), fmt.Sprintf(format, a...))
}

func (op *logBasedOperation) Done(err error) {
	log.Printf("Finished %s for %s: %v", op.action, op.u.String(), err)
}
