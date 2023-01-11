// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"
	"github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/topo-discovery/pkg/southbound"
	"io"
)

func (c *Controller) discoverPorts(object *topo.Object) {
	// Connect to the gNMI server and get list of ports
	devicePorts, err := southbound.GetPorts(object)
	if err != nil {
		log.Warnf("Unable to get ports from device %s", object.ID)
		return
	}

	// Get device port entities from topology
	topoPorts, err := c.getPorts(object)
	if err != nil {
		log.Warnf("Unable to get existing ports for device %s", object.ID)
		return
	}

	// For each gNMI port
	usedPortIDs := make(map[topo.ID]topo.ID, 0)
	for _, p := range devicePorts {
		portID := topo.ID(fmt.Sprintf("%s/%d", object.ID, p.Number))
		topoPort, ok := topoPorts[portID]
		if !ok {
			// port object not found, create one with port aspect and a switch->port 'has' relation
			c.createPort(object, portID, p)
		} else {
			// update port object with port aspect if needed
			c.updatePortIfNeeded(topoPort, p)
		}
		usedPortIDs[portID] = portID
	}

	// Remove any ports if needed
	for _, port := range topoPorts {
		if _, ok := usedPortIDs[port.ID]; !ok {
			c.deletePort(port.ID)
		}
	}
}

func (c *Controller) getPorts(object *topo.Object) (map[topo.ID]*topo.Object, error) {
	stream, err := c.topoClient.Query(c.ctx, &topo.QueryRequest{Filters: &topo.Filters{
		RelationFilter: &topo.RelationFilter{
			SrcId:        string(object.ID),
			RelationKind: topo.HasKind,
			TargetKind:   topo.PortKind,
		},
		WithAspects: []string{"onos.topo.Port"},
	}})
	if err != nil {
		return nil, err
	}

	ports := make(map[topo.ID]*topo.Object, 0)
	var resp *topo.QueryResponse
	for {
		resp, err = stream.Recv()
		if err == io.EOF {
			return ports, nil
		} else if err != nil {
			return nil, err
		}
		ports[resp.Object.ID] = resp.Object
	}
}

func (c *Controller) createPort(object *topo.Object, portID topo.ID, port *topo.Port) {
	portObject, err := topo.NewEntity(portID, topo.PortKind).WithAspects(port)
	if err != nil {
		log.Warnf("Unable to allocate port entity %s: %+v", portID, err)
		return
	}
	if _, err = c.topoClient.Create(c.ctx, &topo.CreateRequest{Object: portObject}); err != nil {
		log.Warnf("Unable to create port entity %s: %+v", portID, err)
		return
	}
	hasRelation := topo.NewRelation(object.ID, portID, topo.HasKind)
	if _, err = c.topoClient.Create(c.ctx, &topo.CreateRequest{Object: hasRelation}); err != nil {
		log.Warnf("Unable to create switch-port relation %s: %+v", hasRelation.ID, err)
		return
	}
	log.Infof("Created port %s: %+v", portID, port)
}

func (c *Controller) updatePortIfNeeded(topoPort *topo.Object, port *topo.Port) {
	topoPortAspect := &topo.Port{}
	if err := topoPort.GetAspect(topoPortAspect); err != nil {
		log.Warnf("Unable to get port aspect for %s: %+v", topoPort.ID, err)
		return
	}

	if portStateChanged(topoPortAspect, port) {
		if err := topoPort.SetAspect(port); err != nil {
			log.Warnf("Unable to update port aspect for %s: %+v", topoPort.ID, err)
			return
		}
		if _, err := c.topoClient.Update(c.ctx, &topo.UpdateRequest{Object: topoPort}); err != nil {
			log.Warnf("Unable to update port entity %s: %+v", topoPort.ID, err)
			return
		}
		log.Infof("Updated port %s: %+v", topoPort.ID, port)
	}
}

func (c *Controller) deletePort(portID topo.ID) {
	if _, err := c.topoClient.Delete(c.ctx, &topo.DeleteRequest{ID: portID}); err != nil {
		log.Warnf("Unable to delete port entity %s: %+v", portID, err)
		return
	}
	log.Infof("Deleted port %s", portID)
}

func portStateChanged(a *topo.Port, b *topo.Port) bool {
	return a.LastChange != b.LastChange || a.Enabled != b.Enabled || a.Status != b.Status
}
