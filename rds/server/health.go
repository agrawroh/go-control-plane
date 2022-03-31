package server

import (
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/envoyproxy/go-control-plane/rds/utils"

	"google.golang.org/grpc/health"
	healthPb "google.golang.org/grpc/health/grpc_health_v1"
)

type HealthChecker struct {
	grpc *health.Server
	ok   uint32
	name string
}

var (
	logger utils.Logger
)

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

func (hc *HealthChecker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ok := atomic.LoadUint32(&hc.ok)
	logger.Debugf("HealthCheck request from host: %s", r.Host)
	if ok == 1 {
		_, err := w.Write([]byte("OK"))
		if err != nil {
			return
		}
	} else {
		w.WriteHeader(500)
	}
}

func (hc *HealthChecker) Fail() {
	atomic.StoreUint32(&hc.ok, 0)
	hc.grpc.SetServingStatus(hc.name, healthPb.HealthCheckResponse_NOT_SERVING)
}

func (hc *HealthChecker) Server() *health.Server {
	return hc.grpc
}
