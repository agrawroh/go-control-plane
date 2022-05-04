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
	// health check gRPC server instance
	grpcServer *health.Server
	// state of the health check response. See: `HealthCheckResponse_ServingStatus` in `grpc/health/grpc_health_v1`
	// { 0 = HealthCheckResponse_NOT_SERVING, 1 = HealthCheckResponse_SERVING }
	// If we don't want to accept any new workloads from the main server then we can explicitly fail the checks so that
	// kubernetes marks the pod as unhealthy and do not forward new requests to it.
	ok uint32
	// name of the health check server
	name string
}

// NewHealthChecker returns the new health checker instance.
func NewHealthChecker(grpcHealthServer *health.Server, name string) *HealthChecker {
	ret := &HealthChecker{
		ok:         1,
		name:       name,
		grpcServer: grpcHealthServer,
	}
	ret.grpcServer.SetServingStatus(ret.name, healthPb.HealthCheckResponse_SERVING)

	// We want to fail the health checks on any SIGTERM signal from the main server. To do this we are creating a new
	// channel and listening to any SIGTERM notifications on this new channel. Upon getting notified, we change the
	// serving status to `NOT_SERVING` to explicitly fail the health checks.
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGTERM)

	go func() {
		<-sigterm
		atomic.StoreUint32(&ret.ok, 0)
		ret.grpcServer.SetServingStatus(ret.name, healthPb.HealthCheckResponse_NOT_SERVING)
	}()

	return ret
}

// ServeHTTP serve the HTTP traffic for health checks.
func (hc *HealthChecker) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {
	// Kubernetes would expect 2XX/3XX status codes as successes and 4XX/5XX status codes as failures.
	if ok := atomic.LoadUint32(&hc.ok); ok == 1 {
		// Here we are returning a 200 status with body as OK to indicate success.
		if _, err := writer.Write([]byte("OK")); err != nil {
			return
		}
	} else {
		// Here we are returning a 500 status code indicate a failure. We don't need any content in the body here.
		writer.WriteHeader(500)
	}
}

// Fail set the ServingStatus to `NOT_SERVING` which would make the health checks fail.
func (hc *HealthChecker) Fail() {
	atomic.StoreUint32(&hc.ok, 0)
	hc.grpcServer.SetServingStatus(hc.name, healthPb.HealthCheckResponse_NOT_SERVING)
}

// Server return the gRPC health check server instance.
func (hc *HealthChecker) Server() *health.Server {
	return hc.grpcServer
}
