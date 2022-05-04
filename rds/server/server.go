package server

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	"github.com/envoyproxy/go-control-plane/rds/utils"

	"github.com/pkg/errors"

	"github.com/envoyproxy/go-control-plane/rds/env"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"github.com/gorilla/mux"
	healthPb "google.golang.org/grpc/health/grpc_health_v1"

	routeService "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
)

/**
 * checkError check and panic in case of an error.
 */
func checkError(err error) {
	if err != nil {
		panic(err)
	}
}

/**
 * stringToTLSVersion utility method to convert the version string to TLS version enum.
 */
func stringToTLSVersion(version string) (uint16, error) {
	var res uint16
	switch version {
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
		return 0, fmt.Errorf("invalid TLS version provided: '%s'", version)
	}

	return res, nil
}

/**
 * getCertificatePool create a new certificate pool by reading all the CA certificates.
 */
func getCertificatePool(cAPath string) (*x509.CertPool, error) {
	if cAPath == "" {
		return nil, nil
	}
	certs, err := ioutil.ReadFile(cAPath)
	if err != nil {
		return nil, err
	}
	rootCAs := x509.NewCertPool()
	if ok := rootCAs.AppendCertsFromPEM(certs); !ok {
		return nil, errors.New("could not append CA certificates to the certificate pool")
	}
	return rootCAs, nil
}

/**
 * createTLSConfig create TLS configuration for HTTPS server.
 */
func createTLSConfig(s *env.Settings) *tls.Config {
	var err error
	minTLS, err := stringToTLSVersion(s.MinTLSVersion)
	checkError(err)
	maxTLS, err := stringToTLSVersion(s.MaxTLSVersion)
	checkError(err)

	if s.ServerTLS {
		return nil
	}

	var certs []tls.Certificate
	ca, err := getCertificatePool(s.ServerCaPath)
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

	return &tls.Config{
		MinVersion:   minTLS,
		MaxVersion:   maxTLS,
		Certificates: certs,
		ClientAuth:   clientAuth,
		ClientCAs:    ca,
	}
}

/**
 * setupHealthCheck register the health check service on the grpcServer.
 */
func setupHealthCheck(grpcServer *grpc.Server, router *mux.Router) {
	healthChecker := NewHealthChecker(health.NewServer(), "route-discovery-service")
	router.Path("/healthz").Handler(healthChecker)
	healthPb.RegisterHealthServer(grpcServer, healthChecker.Server())
}

/**
 * startHTTPServer start a new HTTP server on HTTPPort with the given handler.
 */
func startHTTPServer(HTTPPort int, handler http.Handler, logger utils.Logger) {
	addr := fmt.Sprintf(":%d", HTTPPort)
	logger.Infof("Listening for HTTP on port: '%s'", addr)
	list, err := net.Listen("tcp", addr)
	checkError(err)
	httpServer := &http.Server{Handler: handler}
	if err = httpServer.Serve(list); err != nil {
		panic(err)
	}
}

/**
 * checkStatsDServerStatus check whether StatsD container is up and running.
 */
func checkStatsDServerStatus(statsDHost, statsDPort string, logger utils.Logger) bool {
	timeout := 60 * time.Second
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(statsDHost, statsDPort), timeout)
	if err != nil {
		logger.Errorf("error occurred while checking StatsD container liveliness", err.Error())
		return false
	}
	err = conn.Close()
	return err != nil
}

// RunServer bootstrap the Route Discovery Service server.
func RunServer(settings *env.Settings, server server.Server, logger utils.Logger) {
	// Make sure that StatsD server is up and running
	if checkStatsDServerStatus(settings.StatsDHost, settings.StatsDPort, logger) {
		panic("StatsD was not ready in 60 seconds.")
	}

	// Setup gRPC server
	grpcServer := grpc.NewServer([]grpc.ServerOption{
		grpc.MaxConcurrentStreams(settings.GrpcMaxConcurrentStreams),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    time.Duration(settings.GrpcKeepaliveTimeSeconds) * time.Second,
			Timeout: time.Duration(settings.GrpcKeepaliveTimeoutSeconds) * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             time.Duration(settings.GrpcKeepaliveMinTimeSeconds) * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.Creds(credentials.NewTLS(createTLSConfig(settings))),
	}...)
	port := settings.Port
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	checkError(err)

	// Setup health check server
	healthCheckRouter := mux.NewRouter()
	setupHealthCheck(grpcServer, healthCheckRouter)
	go startHTTPServer(settings.HTTPPort, healthCheckRouter, logger)
	// We only add the debug server if its enabled
	if settings.EnableDebugServer {
		go startHTTPServer(settings.DebugPort, http.DefaultServeMux, logger)
	}

	// Register RDS service
	routeService.RegisterRouteDiscoveryServiceServer(grpcServer, server)
	logger.Infof("RDS Management Server Started. Port: %d\n", port)
	if err = grpcServer.Serve(listener); err != nil {
		panic(err)
	}
}
