package server

import (
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"google.golang.org/grpc/health"
	healthPb "google.golang.org/grpc/health/grpc_health_v1"
)

type HealthChecker struct {
	grpc *health.Server
	ok   uint32
	name string
}

// NewHealthChecker returns the new health checker instance.
func NewHealthChecker(grpcHealthServer *health.Server, name string) *HealthChecker {
	ret := &HealthChecker{}
	ret.ok = 1
	ret.name = name

	ret.grpc = grpcHealthServer
	ret.grpc.SetServingStatus(ret.name, healthPb.HealthCheckResponse_SERVING)

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGTERM)

	go func() {
		<-sigterm
		atomic.StoreUint32(&ret.ok, 0)
		ret.grpc.SetServingStatus(ret.name, healthPb.HealthCheckResponse_NOT_SERVING)
	}()

	return ret
}

// ServeHTTP serve the HTTP traffic for health checks.
func (hc *HealthChecker) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {
	if ok := atomic.LoadUint32(&hc.ok); ok == 1 {
		if _, err := writer.Write([]byte("OK")); err != nil {
			return
		}
	} else {
		writer.WriteHeader(500)
	}
}

// Fail set the ServingStatus to `NOT_SERVING` which would make the health checks fail.
func (hc *HealthChecker) Fail() {
	atomic.StoreUint32(&hc.ok, 0)
	hc.grpc.SetServingStatus(hc.name, healthPb.HealthCheckResponse_NOT_SERVING)
}

// Server return the gRPC health check server instance.
func (hc *HealthChecker) Server() *health.Server {
	return hc.grpc
}
