// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/topo-discovery/pkg/southbound"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestAddToPending(t *testing.T) {
	r := NewLinkReconciler(context.TODO(), nil)
	r.addToPendingLinks(&southbound.Link{IngressDevice: "a", EgressDevice: "b", IngressPort: 1})
	assert.Len(t, r.pendingLinks, 1)
	assert.Len(t, r.pendingLinks["b"], 1)
	r.addToPendingLinks(&southbound.Link{IngressDevice: "c", EgressDevice: "b", IngressPort: 2})
	assert.Len(t, r.pendingLinks, 1)
	assert.Len(t, r.pendingLinks["b"], 2)
}

func TestRegisterReport(t *testing.T) {
	r := NewLinkReconciler(context.TODO(), nil)
	ta := topo.NewEntity(topo.ID("ta"), topo.SwitchKind)
	links := r.registerReport(ta, &southbound.LinkReport{
		AgentID: "a",
		Links: map[uint32]*southbound.Link{
			1: {IngressDevice: "a", IngressPort: 1, EgressDevice: "b", EgressPort: 10},
			2: {IngressDevice: "a", IngressPort: 2, EgressDevice: "b", EgressPort: 11},
			3: {IngressDevice: "a", IngressPort: 3, EgressDevice: "c", EgressPort: 5},
		},
	})
	assert.Len(t, links, 0)
	assert.Len(t, r.pendingLinks, 2)
	assert.Len(t, r.pendingLinks["b"], 2)
	assert.Len(t, r.pendingLinks["c"], 1)
	assert.Len(t, r.agentDevices, 1)

	tb := topo.NewEntity(topo.ID("tb"), topo.SwitchKind)
	links = r.registerReport(tb, &southbound.LinkReport{
		AgentID: "b",
		Links: map[uint32]*southbound.Link{
			10: {IngressDevice: "b", IngressPort: 10, EgressDevice: "a", EgressPort: 1},
			11: {IngressDevice: "b", IngressPort: 11, EgressDevice: "a", EgressPort: 2},
			12: {IngressDevice: "b", IngressPort: 12, EgressDevice: "c", EgressPort: 6},
		},
	})
	assert.Len(t, links, 2+2)
	assert.Len(t, r.pendingLinks, 1)
	assert.Len(t, r.pendingLinks["c"], 2)
	assert.Len(t, r.agentDevices, 2)

	tc := topo.NewEntity(topo.ID("tc"), topo.SwitchKind)
	links = r.registerReport(tc, &southbound.LinkReport{
		AgentID: "c",
		Links: map[uint32]*southbound.Link{
			5: {IngressDevice: "c", IngressPort: 5, EgressDevice: "a", EgressPort: 3},
			6: {IngressDevice: "c", IngressPort: 6, EgressDevice: "b", EgressPort: 12},
		},
	})
	assert.Len(t, links, 2+2)
	assert.Len(t, r.pendingLinks, 0)
	assert.Len(t, r.agentDevices, 3)
}
