package main

import (
	"context"

	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/stream/v3"
)

type MockSnapshotCache struct {
	StatusKeys []string
}

var (
	snapshotsMap = make(map[string]cache.ResourceSnapshot)
)

func (c MockSnapshotCache) CreateWatch(request *cache.Request, state stream.StreamState, responses chan cache.Response) (cancel func()) {
	logger.Infof("Request: %s | State: %s | Responses: %s", request, state, responses)
	panic("Not Implemented!")
}

func (c MockSnapshotCache) CreateDeltaWatch(request *cache.DeltaRequest, state stream.StreamState, responses chan cache.DeltaResponse) (cancel func()) {
	logger.Infof("Request: %s | State: %s | Responses: %s", request, state, responses)
	panic("Not Implemented!")
}

func (c MockSnapshotCache) Fetch(ctx context.Context, request *cache.Request) (cache.Response, error) {
	logger.Infof("Request: %s | Context: %s", request, ctx)
	panic("Not Implemented!")
}

func (c MockSnapshotCache) SetSnapshot(_ context.Context, node string, snapshot cache.ResourceSnapshot) error {
	snapshotsMap[node] = snapshot
	return nil
}

func (c MockSnapshotCache) GetSnapshot(node string) (cache.ResourceSnapshot, error) {
	return snapshotsMap[node], nil
}

func (c MockSnapshotCache) ClearSnapshot(node string) {
	logger.Infof("Node: %s", node)
	panic("Not Implemented!")
}

func (c MockSnapshotCache) GetStatusInfo(s string) cache.StatusInfo {
	logger.Infof("Key: %s", s)
	panic("Not Implemented!")
}

func (c MockSnapshotCache) GetStatusKeys() []string {
	return c.StatusKeys
}

func (c MockSnapshotCache) AddStatusKeys(keys []string) MockSnapshotCache {
	currentKeys := c.GetStatusKeys()
	for _, key := range keys {
		currentKeys = append(currentKeys, key)
	}
	msc := MockSnapshotCache{
		StatusKeys: currentKeys,
	}
	for _, key := range c.GetStatusKeys() {
		sc, _ := c.GetSnapshot(key)
		err := msc.SetSnapshot(nil, key, sc)
		if err != nil {
			return MockSnapshotCache{}
		}
	}
	return msc
}
