// Copyright Splunk Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package example instruments a simple HTTP server-client application.
//
// The application is configured to send spans to a local instance
// of the OpenTelemetry Collector, which propagates them to both
// Splunk Observability Cloud and to a local Jaeger instance.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/signalfx/splunk-otel-go/distro"
	"github.com/signalfx/splunk-otel-go/instrumentation/net/http/splunkhttp"
)

func main() {
	exitCode := 0
	defer func() {
		os.Exit(exitCode)
	}()

	// handle CTRL+C gracefully
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// initialize Splunk OTel distro
	sdk, err := distro.Run()
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := sdk.Shutdown(context.Background()); err != nil {
			log.Println(err)
			exitCode = 1
		}
	}()

	// instrument http.Handler
	var handler http.Handler = http.HandlerFunc(handle)
	handler = splunkhttp.NewHandler(handler)
	handler = otelhttp.NewHandler(handler, "handle")

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           handler,
		WriteTimeout:      time.Second,
		ReadTimeout:       time.Second,
		ReadHeaderTimeout: time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		} else {
			errCh <- nil
		}
	}()

	// instrument http.Client
	client := &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

	call(ctx, client)

	if err := srv.Shutdown(context.Background()); err != nil {
		log.Println(err)
		exitCode = 1
		return
	}
	if err := <-errCh; err != nil {
		log.Println(err)
		exitCode = 1
		return
	}
}

func handle(w http.ResponseWriter, req *http.Request) {
	fmt.Println("HTTP request:")
	dump, err := httputil.DumpRequest(req, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Print(string(dump))
}

func call(ctx context.Context, client *http.Client) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:8080/hello", http.NoBody)
	if err != nil {
		panic(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
		return
	}
	defer resp.Body.Close()

	fmt.Println("HTTP response:")
	dump, err := httputil.DumpResponse(resp, false)
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Print(string(dump))
}
