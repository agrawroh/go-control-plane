package server_test

import (
	"context"
	"testing"

	"github.com/envoyproxy/go-control-plane/cds/env"
	rdsServer "github.com/envoyproxy/go-control-plane/cds/server"
	"github.com/envoyproxy/go-control-plane/cds/utils"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
)

func TestNewServer(t *testing.T) {
	ctx := context.Background()
	logger := utils.Logger{
		Debug: false,
	}
	settings := env.NewSettings()
	snapshotCache := cache.NewSnapshotCache(false, cache.IDHash{}, logger)
	gRPCServer := server.NewServer(ctx, snapshotCache, nil)
	rdsServer.RunServer(&settings, gRPCServer, logger)
}
