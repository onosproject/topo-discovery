// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/topo-discovery/pkg/southbound"
	"io"
)

// TODO: Presently, the port discovery is implemented using a periodic poll via gNMI Get.
// It should be augmented with gNMI subscribe since Stratum does appears to support it.

// PortReconciler provides state and context required for port discovery and reconciliation
type PortReconciler struct {
	topoClient topo.TopoClient
	ctx        context.Context
}

// NewPortReconciler creates a new port reconciler context
func NewPortReconciler(ctx context.Context, topoClient topo.TopoClient) *PortReconciler {
	return &PortReconciler{topoClient: topoClient, ctx: ctx}
}

// DiscoverPorts discovers ports and reconciles their topology entity counterparts
func (r *PortReconciler) DiscoverPorts(object *topo.Object) {
	// Connect to the gNMI server and get list of ports
	devicePorts, err := southbound.GetPorts(object)
	if err != nil {
		log.Warnf("Unable to get ports from device %s", object.ID)
		return
	}

	// Get device port entities from topology
	topoPorts, err := r.getPorts(object)
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
			r.createPort(object, portID, p)
		} else {
			// update port object with port aspect if needed
			r.updatePortIfNeeded(topoPort, p)
		}
		usedPortIDs[portID] = portID
	}

	// Remove any ports if needed
	for _, port := range topoPorts {
		if _, ok := usedPortIDs[port.ID]; !ok {
			r.deletePort(port.ID)
		}
	}
}

func (r *PortReconciler) getPorts(object *topo.Object) (map[topo.ID]*topo.Object, error) {
	stream, err := r.topoClient.Query(r.ctx, &topo.QueryRequest{Filters: &topo.Filters{
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

func (r *PortReconciler) createPort(object *topo.Object, portID topo.ID, port *topo.Port) {
	portObject, err := topo.NewEntity(portID, topo.PortKind).WithAspects(port)
	portObject.Labels = object.Labels // Copy the parent device labels
	if err != nil {
		log.Warnf("Unable to allocate port entity %s: %+v", portID, err)
		return
	}
	if _, err = r.topoClient.Create(r.ctx, &topo.CreateRequest{Object: portObject}); err != nil {
		log.Warnf("Unable to create port entity %s: %+v", portID, err)
		return
	}
	hasRelation := topo.NewRelation(object.ID, portID, topo.HasKind)
	if _, err = r.topoClient.Create(r.ctx, &topo.CreateRequest{Object: hasRelation}); err != nil {
		log.Warnf("Unable to create switch-port relation %s: %+v", hasRelation.ID, err)
		return
	}
	log.Infof("Created port %s: %+v", portID, port)
}

func (r *PortReconciler) updatePortIfNeeded(topoPort *topo.Object, port *topo.Port) {
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
		if _, err := r.topoClient.Update(r.ctx, &topo.UpdateRequest{Object: topoPort}); err != nil {
			log.Warnf("Unable to update port entity %s: %+v", topoPort.ID, err)
			return
		}
		log.Infof("Updated port %s: %+v", topoPort.ID, port)
	}
}

func (r *PortReconciler) deletePort(portID topo.ID) {
	if _, err := r.topoClient.Delete(r.ctx, &topo.DeleteRequest{ID: portID}); err != nil {
		log.Warnf("Unable to delete port entity %s: %+v", portID, err)
		return
	}
	log.Infof("Deleted port %s", portID)
}

func portStateChanged(a *topo.Port, b *topo.Port) bool {
	return a.LastChange != b.LastChange || a.Enabled != b.Enabled || a.Status != b.Status
}
