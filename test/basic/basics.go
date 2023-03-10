// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package basic

import (
	"context"
	"fmt"
	"github.com/onosproject/onos-api/go/onos/discovery"
	fsimapi "github.com/onosproject/onos-api/go/onos/fabricsim"
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

	addSwitch(t, discoClient, "spine1", 0)
	addSwitch(t, discoClient, "spine2", 1)
	addSwitch(t, discoClient, "leaf1", 2)
	addSwitch(t, discoClient, "leaf2", 3)

	t.Log("Validating seed entities...")
	stream, err := topoClient.Query(ctx, &topo.QueryRequest{Filters: &topo.Filters{KindFilter: &topo.Filter{
		Filter: &topo.Filter_In{In: &topo.InFilter{Values: []string{topo.PodKind, topo.RackKind, topo.SwitchKind, topo.ContainsKind}}},
	}}})
	assert.NoError(t, err)
	assert.Len(t, readTopoStream(stream), 11) // pod, rack, 4 switches, 5 relations

	// Allow just a short time for the discovery to react to the new seed entities and discover their ports
	time.Sleep(5 * time.Second)

	t.Log("Validating port entities and relations...")
	stream, err = topoClient.Query(ctx, &topo.QueryRequest{Filters: &topo.Filters{KindFilter: &topo.Filter{
		Filter: &topo.Filter_In{In: &topo.InFilter{Values: []string{topo.PortKind, topo.HasKind}}},
	}}})
	assert.NoError(t, err)
	assert.Len(t, readTopoStream(stream), 4*2*32) // ports and relations

	// Allow enough time for the link agent to pick-up on the new devices and their ports and consequently
	// discover the links involving those ports. Just a tad more than a full-sweep discovery cycle should suffice.
	time.Sleep(35 * time.Second)

	t.Log("Validating link entities and relations...")
	stream, err = topoClient.Query(ctx, &topo.QueryRequest{Filters: &topo.Filters{KindFilter: &topo.Filter{
		Filter: &topo.Filter_In{In: &topo.InFilter{Values: []string{topo.LinkKind, topo.OriginatesKind, topo.TerminatesKind}}},
	}}})
	assert.NoError(t, err)
	assert.Len(t, readTopoStream(stream), 8*2*(1+2)) // links and relations

	// Disable the spine1/1 port...
	fsimClient := fsimapi.NewDeviceServiceClient(s.fsimConn)
	t.Log("Disabling port spine1/1...")
	_, err = fsimClient.DisablePort(ctx, &fsimapi.DisablePortRequest{ID: fsimapi.PortID("spine1/1")})
	assert.NoError(t, err)

	// Allow just a short time for the port status change to be detected and reflected in the topo entities
	time.Sleep(5 * time.Second)

	// Then validate that the port entity was marked down
	t.Log("Validating disabled port and link...")
	resp, err := topoClient.Get(ctx, &topo.GetRequest{ID: "spine1/201"})
	assert.NoError(t, err)
	port := &topo.Port{}
	err = resp.Object.GetAspect(port)
	assert.NoError(t, err)
	assert.Equal(t, "DOWN", port.Status)

	// ... and that the link entity was marked down
	resp, err = topoClient.Get(ctx, &topo.GetRequest{ID: "leaf1/201-spine1/201"})
	assert.NoError(t, err)
	link := &topo.Link{}
	err = resp.Object.GetAspect(link)
	assert.NoError(t, err)
	assert.Equal(t, "DOWN", link.Status)

	// Re-enable the spine1/1 port...
	t.Log("Re-enabling port spine1/1...")
	_, err = fsimClient.EnablePort(ctx, &fsimapi.EnablePortRequest{ID: fsimapi.PortID("spine1/1")})
	assert.NoError(t, err)

	// Again, allow just a short time for the port status change to be detected and reflected in the topo entities
	time.Sleep(5 * time.Second)

	// Then validate that the port entity was marked down
	t.Log("Validating re-enabled port and link...")
	resp, err = topoClient.Get(ctx, &topo.GetRequest{ID: "spine1/201"})
	assert.NoError(t, err)
	err = resp.Object.GetAspect(port)
	assert.NoError(t, err)
	assert.Equal(t, "UP", port.Status)

	// ... and that the link entity was marked down
	resp, err = topoClient.Get(ctx, &topo.GetRequest{ID: "leaf1/201-spine1/201"})
	assert.NoError(t, err)
	err = resp.Object.GetAspect(link)
	assert.NoError(t, err)
	assert.Equal(t, "UP", link.Status)
}

func addSwitch(t *testing.T, discoClient discovery.DiscoveryServiceClient, name string, num int) {
	t.Logf("Adding switch %s...", name)
	stratumEndpoint := fmt.Sprintf("fabric-sim:%d", 20000+num)
	linkAgentEndpoint := fmt.Sprintf("discovery-agent-%d.discovery-agent:30000", num)
	_, err := discoClient.AddSwitch(context.TODO(), &discovery.AddSwitchRequest{
		ID:     name,
		PodID:  pod,
		RackID: rack,
		ManagementInfo: &discovery.ManagementInfo{
			P4RTEndpoint:      stratumEndpoint,
			GNMIEndpoint:      stratumEndpoint,
			PipelineConfigID:  pipelineConfigID,
			ChassisConfigID:   chassisConfigID,
			LinkAgentEndpoint: linkAgentEndpoint,
			HostAgentEndpoint: linkAgentEndpoint, // Same agent for both
		}})
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

	discoConn, err := libtest.CreateConnection("onos-umbrella-topo-discovery:5150", false)
	assert.NoError(t, err)

	topoClient := topo.NewTopoClient(topoConn)
	assert.NotNil(t, topoClient)
	discoClient := discovery.NewDiscoveryServiceClient(discoConn)
	assert.NotNil(t, discoClient)

	return topoClient, discoClient
}
