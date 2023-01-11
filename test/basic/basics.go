// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package basic

import (
	"github.com/onosproject/onos-api/go/onos/discovery"
	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	libtest "github.com/onosproject/onos-lib-go/pkg/test"
	"github.com/stretchr/testify/assert"
	"testing"
)

// TestAPIBasics validates the topology discovery API implementation
func (s *TestSuite) TestAPIBasics(t *testing.T) {
	topoClient, discoClient := getConnections(t)
	assert.NotNil(t, topoClient)
	assert.NotNil(t, discoClient)
}

func getConnections(t *testing.T) (topoapi.TopoClient, discovery.DiscoveryServiceClient) {
	topoConn, err := libtest.CreateConnection("onos-topo:5150", false)
	assert.NoError(t, err)

	discoConn, err := libtest.CreateConnection("topo-discovery:5150", false)
	assert.NoError(t, err)

	topoClient := topoapi.NewTopoClient(topoConn)
	assert.NotNil(t, topoClient)
	discoClient := discovery.NewDiscoveryServiceClient(discoConn)
	return topoClient, discoClient
}
