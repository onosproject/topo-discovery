// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/topo-discovery/pkg/southbound"
	"sync"
)

// TODO: Presently, the link discovery is implemented using a periodic poll via gNMI Get.
// It should be augmented with gNMI subscribe since link agent supports it.

// LinkReconciler provides state and context required for link discovery and reconciliation
type LinkReconciler struct {
	topoClient topo.TopoClient
	ctx        context.Context
	lock       sync.RWMutex

	// Map of agent-id to topo ID required to resolve link reports to a device
	agentDevices map[string]topo.ID

	// Map of agent-id to a list of links that reference that agent ID, but cannot be
	// resolved yet because that agent-id has not yet been registered
	pendingLinks map[string][]*southbound.Link
}

// NewLinkReconciler creates a new link reconciler context
func NewLinkReconciler(ctx context.Context, topoClient topo.TopoClient) *LinkReconciler {
	return &LinkReconciler{
		topoClient:   topoClient,
		ctx:          ctx,
		agentDevices: make(map[string]topo.ID),
		pendingLinks: make(map[string][]*southbound.Link),
	}
}

// DiscoverLinks discovers links and reconciles their topology entity counterparts
func (r *LinkReconciler) DiscoverLinks(object *topo.Object) {
	// Connect to the link agent gNMI server and get its agent ID and a map of ingress links
	linkReport, err := southbound.GetIngressLinks(object)
	if err != nil {
		log.Warnf("Unable to get links from device link agent %s", object.ID)
		return
	}

	// Register the report and agent ID
	linksToProcess := r.registerReport(linkReport)

	for _, link := range linksToProcess {
		r.reconcileLink(link)
	}
}

func (r *LinkReconciler) reconcileLink(link *southbound.Link) {
	// Start by resolving the ingress and egress devices
	ingressID, egressID := r.resolveDevices(link)

	egressPortID := topo.ID(fmt.Sprintf("%s/%d", egressID, link.EgressPort))
	ingressPortID := topo.ID(fmt.Sprintf("%s/%d", ingressID, link.IngressPort))
	linkID := topo.ID(fmt.Sprintf("%s-%s", egressPortID, ingressPortID))

	log.Infof("Reconciling link %s", linkID)
	log.Debugf("... using discovered link %+v", link)

	// Try to get the link
	gr, err := r.topoClient.Get(r.ctx, &topo.GetRequest{ID: linkID})
	if err != nil {
		// If it is not there, create it and its originates/terminates relations
		r.createLink(linkID, egressPortID, ingressPortID, link)
		return
	}

	// Otherwise, if it needs an update, update it
	r.updateLinkIfNeeded(gr.Object, link)
}

// Register the given report, the agent ID and return....
func (r *LinkReconciler) registerReport(report *southbound.LinkReport) []*southbound.Link {
	r.lock.Lock()
	defer r.lock.Unlock()

	// (Re)create the agent ID to device entity ID binding
	r.agentDevices[report.AgentID] = report.ID

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

func (r *LinkReconciler) addToPendingLinks(link *southbound.Link) {
	pending, ok := r.pendingLinks[link.EgressDevice]
	if !ok {
		pending = []*southbound.Link{link}
	} else {
		pending = append(pending, link)
	}
	r.pendingLinks[link.EgressDevice] = pending
}

func (r *LinkReconciler) resolveDevices(link *southbound.Link) (topo.ID, topo.ID) {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.agentDevices[link.IngressDevice], r.agentDevices[link.EgressDevice]
}

func (r *LinkReconciler) createLink(linkID topo.ID, egressPortID topo.ID, ingressPortID topo.ID, link *southbound.Link) {
	object := topo.NewEntity(linkID, topo.LinkKind)
	if _, err := r.topoClient.Create(r.ctx, &topo.CreateRequest{Object: object}); err != nil {
		log.Warnf("Unable to create link %s: %+v", linkID, err)
		return
	}

	originates := topo.NewRelation(egressPortID, linkID, topo.OriginatesKind)
	if _, err := r.topoClient.Create(r.ctx, &topo.CreateRequest{Object: originates}); err != nil {
		log.Warnf("Unable to create originates relation for link %s: %+v", linkID, err)
		return
	}

	terminates := topo.NewRelation(ingressPortID, linkID, topo.TerminatesKind)
	if _, err := r.topoClient.Create(r.ctx, &topo.CreateRequest{Object: terminates}); err != nil {
		log.Warnf("Unable to create terminates relation for link %s: %+v", linkID, err)
		return
	}
	log.Infof("Created link %s", linkID)
}

func (r *LinkReconciler) updateLinkIfNeeded(object *topo.Object, link *southbound.Link) {
	// TODO: implement this
}
