package server

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/envoyproxy/go-control-plane/rds/env"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"github.com/gorilla/mux"
	healthPb "google.golang.org/grpc/health/grpc_health_v1"

	discoveryGrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	endpointService "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	routeService "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	runtimeService "github.com/envoyproxy/go-control-plane/envoy/service/runtime/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
)

func checkError(err error) {
	if err != nil {
		panic(err)
	}
}

func stringToTlsVersion(ver string) (uint16, error) {
	var res uint16
	switch ver {
	case "TLSv1.0":
		res = tls.VersionTLS10
	case "TLSv1.1":
		res = tls.VersionTLS11
	case "TLSv1.2":
		res = tls.VersionTLS12
	case "TLSv1.3":
		res = tls.VersionTLS13
	case "":
		res = 0
	default:
		return 0, errors.New("invalid TLS version provided")
	}

	return res, nil
}

func getCertPool(CAPath string) (*x509.CertPool, error) {
	if CAPath == "" {
		return nil, nil
	}
	rootCAs := x509.NewCertPool()
	certs, err := ioutil.ReadFile(CAPath)
	if err != nil {
		return nil, err
	}
	if ok := rootCAs.AppendCertsFromPEM(certs); !ok {
		return nil, errors.New("could not append CA certs to cert pool")
	}
	return rootCAs, nil
}

func CreateTlsConfig(s *env.Settings) {
	var err error
	minTls, err := stringToTlsVersion(s.MinTlsVersion)
	checkError(err)
	maxTls, err := stringToTlsVersion(s.MaxTlsVersion)
	checkError(err)

	if s.ServerTls {
		var certs []tls.Certificate
		ca, err := getCertPool(s.ServerCaPath)
		checkError(err)

		if s.ServerCertificatePath != "" && s.ServerKeyPath != "" {
			serverKeyPair, err := tls.LoadX509KeyPair(s.ServerCertificatePath, s.ServerKeyPath)
			checkError(err)
			certs = append(certs, serverKeyPair)
		}

		var clientAuth tls.ClientAuthType
		if s.RequireClientCert {
			clientAuth = tls.RequireAndVerifyClientCert
		} else {
			clientAuth = tls.NoClientCert
		}

		s.ServerTlsConfig = &tls.Config{
			MinVersion:   minTls,
			MaxVersion:   maxTls,
			Certificates: certs,
			ClientAuth:   clientAuth,
			ClientCAs:    ca,
		}
	}
}

func registerServer(grpcServer *grpc.Server, server server.Server) {
	// Register Services (Discovery, RDS, EDS, Runtime)
	discoveryGrpc.RegisterAggregatedDiscoveryServiceServer(grpcServer, server)
	routeService.RegisterRouteDiscoveryServiceServer(grpcServer, server)
	endpointService.RegisterEndpointDiscoveryServiceServer(grpcServer, server)
	runtimeService.RegisterRuntimeDiscoveryServiceServer(grpcServer, server)
}

func setupHealthCheck(grpcServer *grpc.Server, router *mux.Router) {
	// Register Health Checker Service
	healthChecker := NewHealthChecker(health.NewServer(), "route-discovery-service")
	router.Path("/healthz").Handler(healthChecker)
	healthPb.RegisterHealthServer(grpcServer, healthChecker.Server())
}

func startHttpServer(httpPort int, router *mux.Router) {
	addr := fmt.Sprintf(":%d", httpPort)
	logger.Infof("Listening for HTTP on '%s'", addr)
	list, err := net.Listen("tcp", addr)
	srv := &http.Server{Handler: router}
	err = srv.Serve(list)
	if err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func startDebugServer(httpPort int) {
	addr := fmt.Sprintf(":%d", httpPort)
	logger.Infof("Listening for Debug on '%s'", addr)
	list, err := net.Listen("tcp", addr)
	srv := &http.Server{Handler: http.DefaultServeMux}
	err = srv.Serve(list)
	if err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func checkStatsDServerStatus(statsDHost, statsDPort string) bool {
	timeout := 60 * time.Second
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(statsDHost, statsDPort), timeout)
	if err != nil {
		return false
	}
	err = conn.Close()
	if err != nil {
		return false
	}
	return true
}

// RunServer starts the RDS server at the given port
func RunServer(settings *env.Settings, srv server.Server) {
	// gRPC golang library sets a very small upper bound for the number gRPC/h2
	// streams over a single TCP connection. If a proxy multiplexes requests over
	// a single connection to the management server, then it might lead to
	// availability problems. Keepalive timeouts based on connection_keepalive parameter https://www.envoyproxy.io/docs/envoy/latest/configuration/overview/examples#dynamic
	if checkStatsDServerStatus(settings.StatsDHost, settings.StatsDPort) {
		CreateTlsConfig(settings)
		var grpcOptions []grpc.ServerOption
		tlsCredentials := credentials.NewTLS(settings.ServerTlsConfig)
		grpcOptions = append(grpcOptions,
			grpc.MaxConcurrentStreams(settings.GrpcMaxConcurrentStreams),
			grpc.KeepaliveParams(keepalive.ServerParameters{
				Time:    time.Duration(settings.GrpcKeepaliveTimeSeconds) * time.Second,
				Timeout: time.Duration(settings.GrpcKeepaliveTimeoutSeconds) * time.Second,
			}),
			grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
				MinTime:             time.Duration(settings.GrpcKeepaliveMinTimeSeconds) * time.Second,
				PermitWithoutStream: true,
			}),
			grpc.Creds(tlsCredentials),
		)
		grpcServer := grpc.NewServer(grpcOptions...)
		port := settings.Port
		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			log.Fatal(err)
		}

		// Setup Health Checker & HTTP Server
		healthCheckRouter := mux.NewRouter()
		setupHealthCheck(grpcServer, healthCheckRouter)
		go startHttpServer(settings.HttpPort, healthCheckRouter)
		go startDebugServer(8081)

		// Register Discovery Service(s)
		registerServer(grpcServer, srv)

		logger.Infof("RDS Management Server Started. Port: %d\n", port)
		if err = grpcServer.Serve(lis); err != nil {
			log.Fatal(err)
		}
	} else {
		log.Fatal("StatsD was not ready in 60 seconds.")
	}
}
