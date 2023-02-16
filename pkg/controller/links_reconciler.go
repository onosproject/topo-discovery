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
	"sync"
	"time"
)

const (
	statusUp   = "UP"
	statusDown = "DOWN"
)

// LinkReconciler provides state and context required for link discovery and reconciliation
type LinkReconciler struct {
	southbound.IngressLinkListener
	linkDiscovery southbound.IngressLinkDiscovery
	topoClient    topo.TopoClient
	ctx           context.Context
	lock          sync.RWMutex

	// Map of agent-id to topo object required to resolve link reports to a device
	agentDevices map[string]*topo.Object

	// Map of agent-id to a list of links that reference that agent ID, but cannot be
	// resolved yet because that agent-id has not yet been registered
	pendingLinks map[string][]*southbound.Link
}

// NewLinkReconciler creates a new link reconciler context
func NewLinkReconciler(ctx context.Context, topoClient topo.TopoClient) *LinkReconciler {
	return &LinkReconciler{
		topoClient:    topoClient,
		ctx:           ctx,
		agentDevices:  make(map[string]*topo.Object),
		pendingLinks:  make(map[string][]*southbound.Link),
		linkDiscovery: southbound.NewGNMILinkDiscovery(),
	}
}

// DiscoverLinks discovers links and reconciles their topology entity counterparts
func (r *LinkReconciler) DiscoverLinks(object *topo.Object) {
	// Connect to the link agent gNMI server and get its agent ID and a map of ingress links
	linkReport, err := r.linkDiscovery.GetIngressLinks(object, r)
	if err != nil {
		log.Warnf("Unable to get links from device link agent %s: %+v", object.ID, err)
		return
	}

	// Register the report and agent ID
	linksToProcess := r.registerReport(object, linkReport)
	for _, link := range linksToProcess {
		r.reconcileLink(link, statusUp)
	}
	r.updateDownedLinks(object, linkReport)
}

// RegisterAgent discovers agentID and binds it to the specified object ID
func (r *LinkReconciler) RegisterAgent(object *topo.Object) {
	report, err := r.linkDiscovery.GetIngressLinks(object, nil)
	if err != nil {
		log.Warnf("Unable to get agent ID from device link agent %s: %+v", object.ID, err)
		return
	}

	// (Re)create the agent ID to device entity ID binding
	r.lock.Lock()
	defer r.lock.Unlock()
	r.agentDevices[report.AgentID] = object
}

// LinkAdded handles link addition event
func (r *LinkReconciler) LinkAdded(link *southbound.Link) {
	r.reconcileLink(link, statusUp)
}

// LinkDeleted handles link deletion event
func (r *LinkReconciler) LinkDeleted(link *southbound.Link) {
	link.CreateTime = uint64(time.Now().UnixNano())
	r.reconcileLink(link, statusDown)
}

// Reconciles the specified southbound link against its topology entity counterpart
func (r *LinkReconciler) reconcileLink(link *southbound.Link, status string) {
	// Start by resolving the ingress and egress devices
	ingressDevice, egressDevice := r.resolveDevices(link)
	if ingressDevice == nil {
		return
	}

	if egressDevice == nil {
		// If the egress device is now yet resolved, add the link to its pending links
		r.lock.Lock()
		r.addToPendingLinks(link)
		r.lock.Unlock()
		return
	}

	egressPortID := topo.ID(fmt.Sprintf("%s/%d", egressDevice.ID, link.EgressPort))
	ingressPortID := topo.ID(fmt.Sprintf("%s/%d", ingressDevice.ID, link.IngressPort))
	linkID := topo.ID(fmt.Sprintf("%s-%s", egressPortID, ingressPortID))

	log.Infof("Reconciling link %s", linkID)
	log.Debugf("... using discovered link %+v", link)

	// Try to get the link
	gr, err := r.topoClient.Get(r.ctx, &topo.GetRequest{ID: linkID})
	if err != nil {
		// If it is not there, create it and its originates/terminates relations
		r.createLink(linkID, egressPortID, ingressPortID, link, egressDevice.Labels)
		return
	}

	// Otherwise, if it needs an update, update it
	r.updateLinkIfNeeded(gr.Object, link, status)
}

// Register the given report, the agent ID and return....
func (r *LinkReconciler) registerReport(object *topo.Object, report *southbound.LinkReport) []*southbound.Link {
	r.lock.Lock()
	defer r.lock.Unlock()

	// (Re)create the agent ID to device entity ID binding
	r.agentDevices[report.AgentID] = object

	// See if all links in the report can be processed, if not, register them in the pending links
	// Otherwise, add them to the links to be processed now
	links := make([]*southbound.Link, 0, len(report.Links))
	for _, link := range report.Links {
		if _, ok := r.agentDevices[link.EgressDevice]; ok {
			links = append(links, link)
		} else {
			r.addToPendingLinks(link)
		}
	}

	// Now check if there are any pending links for the reported agent ID
	pendingLinks, ok := r.pendingLinks[report.AgentID]
	if ok {
		// If so, add them to the links to be processed and remove them from the pending links map
		links = append(links, pendingLinks...)
		delete(r.pendingLinks, report.AgentID)
	}

	return links
}

// Adds the given southbound link to the list of pending links for its egress device
func (r *LinkReconciler) addToPendingLinks(link *southbound.Link) {
	pending, ok := r.pendingLinks[link.EgressDevice]
	if !ok {
		pending = []*southbound.Link{link}
	} else {
		pending = append(pending, link)
	}
	r.pendingLinks[link.EgressDevice] = pending
}

// Resolves link ingress/egress agent IDs into corresponding device topo entities
func (r *LinkReconciler) resolveDevices(link *southbound.Link) (*topo.Object, *topo.Object) {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.agentDevices[link.IngressDevice], r.agentDevices[link.EgressDevice]
}

// Creates link topo object and its originates/terminates relations
func (r *LinkReconciler) createLink(linkID topo.ID, egressPortID topo.ID, ingressPortID topo.ID,
	link *southbound.Link, labels map[string]string) {
	linkAspect := &topo.Link{Status: "UP", LastChange: link.CreateTime}
	object, err := topo.NewEntity(linkID, topo.LinkKind).WithAspects(linkAspect)
	if err != nil {
		log.Warnf("Unable to allocate link %s: %+v", linkID, err)
		return
	}
	object.Labels = labels
	if _, err = r.topoClient.Create(r.ctx, &topo.CreateRequest{Object: object}); err != nil {
		log.Warnf("Unable to create link %s: %+v", linkID, err)
		return
	}

	originates := topo.NewRelation(egressPortID, linkID, topo.OriginatesKind)
	if _, err = r.topoClient.Create(r.ctx, &topo.CreateRequest{Object: originates}); err != nil {
		log.Warnf("Unable to create originates relation for link %s: %+v", linkID, err)
		return
	}

	terminates := topo.NewRelation(ingressPortID, linkID, topo.TerminatesKind)
	if _, err = r.topoClient.Create(r.ctx, &topo.CreateRequest{Object: terminates}); err != nil {
		log.Warnf("Unable to create terminates relation for link %s: %+v", linkID, err)
		return
	}
	log.Infof("Created link %s", linkID)
}

// Updates link if the link aspect update time differs from the southbound link create time
func (r *LinkReconciler) updateLinkIfNeeded(linkObject *topo.Object, link *southbound.Link, status string) {
	linkAspect := &topo.Link{}
	if err := linkObject.GetAspect(linkAspect); err != nil || linkAspect.LastChange < link.CreateTime {
		linkAspect.Status = status
		linkAspect.LastChange = link.CreateTime

		if err = linkObject.SetAspect(linkAspect); err != nil {
			log.Warnf("Unable to set link %s aspect %+v: %+v", linkObject.ID, linkAspect, err)
			return
		}

		if _, err = r.topoClient.Update(r.ctx, &topo.UpdateRequest{Object: linkObject}); err != nil {
			log.Warnf("Unable to update link %s with %+v: %+v", linkObject.ID, linkAspect, err)
			return
		}
		log.Infof("Updated status of link %s: %+v", linkObject.ID, linkAspect)
	}
}

// Updates any topology link entities to down state if they don't have a counterpart in the southbound links report
func (r *LinkReconciler) updateDownedLinks(object *topo.Object, report *southbound.LinkReport) {
	// TODO: implement me; may require enhanced relations query for efficient implementation
	// Get the device ports first
	portsFilter := &topo.RelationFilter{SrcId: string(object.ID), RelationKind: topo.HasKind, TargetKind: topo.PortKind}
	stream, err := r.topoClient.Query(r.ctx, &topo.QueryRequest{Filters: &topo.Filters{RelationFilter: portsFilter}})
	if err != nil {
		log.Warnf("Unable to query device ports for %s: %+v", object.ID, err)
	}

	for {
		resp, err := stream.Recv()
		if err != nil {
			if err != io.EOF {
				log.Warnf("Unable to read device ports for %s: %+v", object.ID, err)
			}
			return
		}

		// If the returned object isn't a source of any relations, ignore it
		if len(resp.Object.GetEntity().SrcRelationIDs) == 0 {
			continue
		}

		// If the port is in the southbound link report link map, it means no pruning is needed
		portAspect := &topo.Port{}
		if err = resp.Object.GetAspect(portAspect); err != nil {
			log.Warnf("Unable to get port aspect from port entity %s: %+v", resp.Object.ID, err)
			continue
		}
		if _, ok := report.Links[portAspect.Number]; ok {
			// Port has an ingress link, no need to prune
			continue
		}

		// Otherwise, get the link that terminates at this port and mark it as DOWN
		r.updateDownedIngressLink(resp.Object)
	}
}

func (r *LinkReconciler) updateDownedIngressLink(portObject *topo.Object) {
	linkFilter := &topo.RelationFilter{SrcId: string(portObject.ID), RelationKind: topo.TerminatesKind, TargetKind: topo.LinkKind}
	stream, err := r.topoClient.Query(r.ctx, &topo.QueryRequest{Filters: &topo.Filters{RelationFilter: linkFilter}})
	if err != nil {
		log.Warnf("Unable to query ingress link for port %s: %+v", portObject.ID, err)
	}

	// Assume at most one response
	resp, err := stream.Recv()
	if err != nil {
		if err != io.EOF {
			log.Warnf("Unable to read ingress link for port %s: %+v", portObject.ID, err)
		}
		return
	}

	linkAspect := &topo.Link{}
	if err = resp.Object.GetAspect(linkAspect); err != nil {
		log.Warnf("Unable to get ingress link aspect for %s: %v", resp.Object.ID, err)
		return
	}

	if linkAspect.Status != "DOWN" {
		// If the link is not already marked as down, mark it as such
		linkAspect = &topo.Link{Status: "DOWN", LastChange: uint64(time.Now().UnixNano())}
		if err = resp.Object.SetAspect(linkAspect); err != nil {
			log.Warnf("Unable to set ingress link aspect for %s: %v", resp.Object.ID, err)
			return
		}

		if _, err = r.topoClient.Update(r.ctx, &topo.UpdateRequest{Object: resp.Object}); err != nil {
			log.Warnf("Unable to update ingress link aspect for %s: %v", resp.Object.ID, err)
			return
		}
		log.Infof("Updated status of link %s: %+v", resp.Object.ID, linkAspect)
	}
}
