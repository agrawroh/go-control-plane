package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/signal"
	"syscall"
	"testing"

	"github.com/envoyproxy/go-control-plane/cds/server"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthPb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestHealthCheckSuccess(t *testing.T) {
	defer signal.Reset(syscall.SIGTERM)

	recorder := httptest.NewRecorder()

	hc := server.NewHealthChecker(health.NewServer(), "route-discovery-service")

	r, _ := http.NewRequest("GET", "http://1.2.3.4/healthcheck", nil)
	hc.ServeHTTP(recorder, r)

	if 200 != recorder.Code {
		t.Errorf("expected code 200 actual %d", recorder.Code)
	}

	if "OK" != recorder.Body.String() {
		t.Errorf("expected body 'OK', got '%s'", recorder.Body.String())
	}
}

func TestHealthCheckFailure(t *testing.T) {
	defer signal.Reset(syscall.SIGTERM)

	recorder := httptest.NewRecorder()

	hc := server.NewHealthChecker(health.NewServer(), "route-discovery-service")
	hc.Fail()

	r, _ := http.NewRequest("GET", "http://1.2.3.4/healthcheck", nil)
	hc.ServeHTTP(recorder, r)

	if 500 != recorder.Code {
		t.Errorf("expected code 500 actual %d", recorder.Code)
	}
}

func TestGrpcHealthCheckSuccess(t *testing.T) {
	defer signal.Reset(syscall.SIGTERM)

	grpcHealthServer := health.NewServer()
	_ = server.NewHealthChecker(grpcHealthServer, "route-discovery-service")
	healthPb.RegisterHealthServer(grpc.NewServer(), grpcHealthServer)

	req := &healthPb.HealthCheckRequest{
		Service: "route-discovery-service",
	}

	res, _ := grpcHealthServer.Check(context.Background(), req)
	if healthPb.HealthCheckResponse_SERVING != res.Status {
		t.Errorf("expected status SERVING actual %v", res.Status)
	}
}

func TestGrpcHealthCheckFailure(t *testing.T) {
	defer signal.Reset(syscall.SIGTERM)

	grpcHealthServer := health.NewServer()
	hc := server.NewHealthChecker(grpcHealthServer, "route-discovery-service")
	hc.Fail()
	healthPb.RegisterHealthServer(grpc.NewServer(), grpcHealthServer)

	req := &healthPb.HealthCheckRequest{
		Service: "route-discovery-service",
	}

	res, _ := grpcHealthServer.Check(context.Background(), req)
	if healthPb.HealthCheckResponse_NOT_SERVING != res.Status {
		t.Errorf("expected status NOT_SERVING actual %v", res.Status)
	}
}
