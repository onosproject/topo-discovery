// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package basic

import (
	"context"
	"fmt"
	"github.com/onosproject/onos-api/go/onos/discovery"
	"github.com/onosproject/onos-api/go/onos/topo"
	libtest "github.com/onosproject/onos-lib-go/pkg/test"
	"github.com/stretchr/testify/assert"
	"io"
	"testing"
	"time"
)

const (
	pod  = "all"
	rack = "rack-01-1"

	pipelineConfigID = "fabric-spine-v1-tofino-pipeline"
	chassisConfigID  = "fabric-spine-v1-tofino-chassis"
)

// TestAPIBasics validates the topology discovery API implementation
func (s *TestSuite) TestAPIBasics(t *testing.T) {
	topoClient, discoClient := getConnections(t)

	// Create a new POD and a new rack
	ctx := context.TODO()

	t.Log("Adding pod...")
	_, err := discoClient.AddPod(ctx, &discovery.AddPodRequest{ID: pod})
	assert.NoError(t, err)

	t.Log("Adding rack...")
	_, err = discoClient.AddRack(ctx, &discovery.AddRackRequest{ID: rack, PodID: pod})
	assert.NoError(t, err)

	addSwitch(t, discoClient, "spine1", 20000)
	addSwitch(t, discoClient, "spine2", 20001)
	addSwitch(t, discoClient, "leaf1", 20002)
	addSwitch(t, discoClient, "leaf2", 20003)

	t.Log("Validating seed entities...")
	stream, err := topoClient.Query(ctx, &topo.QueryRequest{Filters: &topo.Filters{KindFilter: &topo.Filter{
		Filter: &topo.Filter_In{In: &topo.InFilter{Values: []string{topo.PodKind, topo.RackKind, topo.SwitchKind, topo.ContainsKind}}},
	}}})
	assert.NoError(t, err)
	assert.Len(t, readTopoStream(stream), 11) // pod, rack, 4 switches, 5 relations

	time.Sleep(5 * time.Second)

	t.Log("Validating port entities and relations...")
	stream, err = topoClient.Query(ctx, &topo.QueryRequest{Filters: &topo.Filters{KindFilter: &topo.Filter{
		Filter: &topo.Filter_In{In: &topo.InFilter{Values: []string{topo.PortKind, topo.HasKind}}},
	}}})
	assert.NoError(t, err)
	assert.Len(t, readTopoStream(stream), 4*2*32) // ports and relations

}

func addSwitch(t *testing.T, discoClient discovery.DiscoveryServiceClient, name string, port int) {
	t.Logf("Adding switch %s...", name)
	endpoint := fmt.Sprintf("fabric-sim:%d", port)
	_, err := discoClient.AddSwitch(context.TODO(), &discovery.AddSwitchRequest{
		ID:               name,
		PodID:            pod,
		RackID:           rack,
		P4Endpoint:       endpoint,
		GNMIEndpoint:     endpoint,
		PipelineConfigID: pipelineConfigID,
		ChassisConfigID:  chassisConfigID,
	})
	assert.NoError(t, err)
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
