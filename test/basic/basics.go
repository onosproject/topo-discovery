// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package basic

import (
	"context"
	"github.com/onosproject/onos-api/go/onos/discovery"
	"github.com/onosproject/onos-api/go/onos/topo"
	libtest "github.com/onosproject/onos-lib-go/pkg/test"
	"github.com/stretchr/testify/assert"
	"io"
	"testing"
	"time"
)

const pod = "all"

// TestAPIBasics validates the topology discovery API implementation
func (s *TestSuite) TestAPIBasics(t *testing.T) {
	topoClient, discoClient := getConnections(t)

	// Create a new POD and a new rack
	ctx := context.TODO()

	t.Log("Adding pod...")
	_, err := discoClient.AddPod(ctx, &discovery.AddPodRequest{ID: pod})
	assert.NoError(t, err)

	t.Log("Adding rack...")
	_, err = discoClient.AddRack(ctx, &discovery.AddRackRequest{ID: "rack-01", PodID: pod})
	assert.NoError(t, err)

	t.Log("Adding switch spine1...")
	_, err = discoClient.AddSwitch(ctx, &discovery.AddSwitchRequest{
		ID:               "spine1",
		PodID:            pod,
		RackID:           "rack-01",
		P4Endpoint:       "fabric-sim:20000",
		GNMIEndpoint:     "fabric-sim:20000",
		PipelineConfigID: "fabric-spine-v1-tofino-pipeline",
		ChassisConfigID:  "fabric-spine-v1-tofino-chassis",
	})
	assert.NoError(t, err)

	t.Log("Adding switch spine2...")
	_, err = discoClient.AddSwitch(ctx, &discovery.AddSwitchRequest{
		ID:               "spine2",
		PodID:            pod,
		RackID:           "rack-01",
		P4Endpoint:       "fabric-sim:20001",
		GNMIEndpoint:     "fabric-sim:20001",
		PipelineConfigID: "fabric-spine-v1-tofino-pipeline",
		ChassisConfigID:  "fabric-spine-v1-tofino-chassis",
	})
	assert.NoError(t, err)

	t.Log("Validating seed entities...")
	stream, err := topoClient.Query(ctx, &topo.QueryRequest{Filters: &topo.Filters{KindFilter: &topo.Filter{
		Filter: &topo.Filter_In{In: &topo.InFilter{Values: []string{topo.PodKind, topo.RackKind, topo.SwitchKind, topo.ContainsKind}}},
	}}})
	assert.NoError(t, err)
	assert.Len(t, readTopoStream(stream), 7) // pod, rack, pod-rack, spine1, rack-spine1, spine2, rack-spine2

	time.Sleep(5 * time.Second)

	t.Log("Validating port entities and relations...")
	stream, err = topoClient.Query(ctx, &topo.QueryRequest{Filters: &topo.Filters{KindFilter: &topo.Filter{
		Filter: &topo.Filter_In{In: &topo.InFilter{Values: []string{topo.PortKind, topo.HasKind}}},
	}}})
	assert.NoError(t, err)
	assert.Len(t, readTopoStream(stream), 2*2*32) // ports and relations

}

func readTopoStream(stream topo.Topo_QueryClient) []*topo.Object {
	objects := make([]*topo.Object, 0)
	for {
		resp, err := stream.Recv()
		switch err {
		case nil:
			objects = append(objects, resp.Object)
		case io.EOF:
			return objects
		}
	}
}

func getConnections(t *testing.T) (topo.TopoClient, discovery.DiscoveryServiceClient) {
	topoConn, err := libtest.CreateConnection("onos-topo:5150", false)
	assert.NoError(t, err)

	discoConn, err := libtest.CreateConnection("topo-discovery:5150", false)
	assert.NoError(t, err)

	topoClient := topo.NewTopoClient(topoConn)
	assert.NotNil(t, topoClient)
	discoClient := discovery.NewDiscoveryServiceClient(discoConn)
	assert.NotNil(t, discoClient)

	return topoClient, discoClient
}
