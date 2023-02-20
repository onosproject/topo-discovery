// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/topo-discovery/pkg/southbound"
	"strconv"
	"sync"
	"time"
)

// HostReconciler provides state and context required for host discovery and reconciliation
type HostReconciler struct {
	southbound.HostListener
	hostDiscovery southbound.HostDiscovery
	topoClient    topo.TopoClient
	ctx           context.Context
	lock          sync.RWMutex

	// Map of agent-id to topo object required to resolve link reports to a device
	agentDevices map[string]*topo.Object

	// Map of agent-id to a list of links that reference that agent ID, but cannot be
	// resolved yet because that agent-id has not yet been registered
	pendingHosts map[string][]*southbound.Host
}

// NewHostReconciler creates a new host reconciler context
func NewHostReconciler(ctx context.Context, topoClient topo.TopoClient) *HostReconciler {
	return &HostReconciler{
		topoClient:    topoClient,
		ctx:           ctx,
		agentDevices:  make(map[string]*topo.Object),
		pendingHosts:  make(map[string][]*southbound.Host),
		hostDiscovery: southbound.NewGNMIHostDiscovery(),
	}
}

// DiscoverHosts discovers hosts and reconciles their topology entity counterparts
func (r *HostReconciler) DiscoverHosts(object *topo.Object) {
	// Connect to the host agent gNMI server and get its agent ID and a map of hosts
	hostReport, err := r.hostDiscovery.GetHosts(object, r)
	if err != nil {
		log.Warnf("Unable to get hosts from device host agent %s: %+v", object.ID, err)
		return
	}

	// Register the report and agent ID
	hostsToProcess := r.registerReport(object, hostReport)
	for _, host := range hostsToProcess {
		r.reconcileHost(host)
	}
	//r.updateDownedHosts(object, hostReport)
}

// HostAdded handles host addition event
func (r *HostReconciler) HostAdded(host *southbound.Host) {
	r.reconcileHost(host)
}

// HostDeleted handles host deletion event
func (r *HostReconciler) HostDeleted(host *southbound.Host) {
	host.CreateTime = uint64(time.Now().UnixNano())
	r.reconcileHost(host)
}

// Reconciles the specified southbound host against its topology entity counterpart
func (r *HostReconciler) reconcileHost(host *southbound.Host) {
	// first, compose DeviceMAC/Port ID
	hostID := topo.ID(host.MAC + "/" + strconv.FormatUint(uint64(host.Port), 10))

	//composing IP address
	ipAddr := topo.IPAddress{
		IP:   host.IP,
		Type: topo.IPAddress_IPV4,
	}

	// Try to get the host
	_, err := r.topoClient.Get(r.ctx, &topo.GetRequest{ID: hostID})
	if err != nil {
		// If it is not there, create it and its relation
		r.createHost(hostID, ipAddr, host)
		return
	}

	// Otherwise, if it needs an update, update it
	//r.updateHostIfNeeded(gr.Object, host, status)
}

// Register the given report, the agent ID and return....
func (r *HostReconciler) registerReport(object *topo.Object, report *southbound.HostReport) []*southbound.Host {
	r.lock.Lock()
	defer r.lock.Unlock()

	// (Re)create the agent ID to device entity ID binding
	r.agentDevices[report.AgentID] = object

	// See if all hosts in the report can be processed, if not, register them in the pending hosts
	// Otherwise, add them to the hosts to be processed now
	hosts := make([]*southbound.Host, 0, len(report.Hosts))
	for _, host := range report.Hosts {
		if _, ok := r.agentDevices[host.MAC]; ok {
			hosts = append(hosts, host)
		} else {
			r.addToPendingHosts(host)
		}
	}

	// Now check if there are any pending hosts for the reported agent ID
	pendingHosts, ok := r.pendingHosts[report.AgentID]
	if ok {
		// If so, add them to the hosts to be processed and remove them from the pending hosts map
		hosts = append(hosts, pendingHosts...)
		delete(r.pendingHosts, report.AgentID)
	}

	return hosts
}

// Adds the given southbound host to the list of pending hosts for its MAC
func (r *HostReconciler) addToPendingHosts(host *southbound.Host) {
	pending, ok := r.pendingHosts[host.MAC]
	if !ok {
		pending = []*southbound.Host{host}
	} else {
		pending = append(pending, host)
	}
	r.pendingHosts[host.MAC] = pending
}

// Creates host topo object and its relation
func (r *HostReconciler) createHost(hostID topo.ID, ipAddr topo.IPAddress, host *southbound.Host) {
	hostAspect := &topo.NetworkInterface{MacDevicePort: host.MAC + "/" + strconv.FormatUint(uint64(host.Port), 10),
		Ip: &ipAddr}
	object, err := topo.NewEntity(hostID, topo.HostKind).WithAspects(hostAspect)
	if err != nil {
		log.Warnf("Unable to allocate host %s: %+v", hostID, err)
		return
	}
	// ToDo - where/how can I obtain labels?? Do I need it at all?
	//object.Labels = labels
	if _, err = r.topoClient.Create(r.ctx, &topo.CreateRequest{Object: object}); err != nil {
		log.Warnf("Unable to create host %s: %+v", hostID, err)
		return
	}

	portID := topo.ID(strconv.FormatUint(uint64(host.Port), 10))
	originates := topo.NewRelation(portID, hostID, topo.OriginatesKind)
	if _, err = r.topoClient.Create(r.ctx, &topo.CreateRequest{Object: originates}); err != nil {
		log.Warnf("Unable to create originates relation for host %s: %+v", hostID, err)
		return
	}
	log.Infof("Created host %s", hostID)
}

// Updates host if the host aspect update time differs from the southbound host create time
//func (r *LinkReconciler) updateLinkIfNeeded(linkObject *topo.Object, link *southbound.Link, status string) {
//	linkAspect := &topo.Link{}
//	if err := linkObject.GetAspect(linkAspect); err != nil || linkAspect.LastChange < link.CreateTime {
//		linkAspect.Status = status
//		linkAspect.LastChange = link.CreateTime
//
//		if err = linkObject.SetAspect(linkAspect); err != nil {
//			log.Warnf("Unable to set link %s aspect %+v: %+v", linkObject.ID, linkAspect, err)
//			return
//		}
//
//		if _, err = r.topoClient.Update(r.ctx, &topo.UpdateRequest{Object: linkObject}); err != nil {
//			log.Warnf("Unable to update link %s with %+v: %+v", linkObject.ID, linkAspect, err)
//			return
//		}
//		log.Infof("Updated status of link %s: %+v", linkObject.ID, linkAspect)
//	}
//}
//
//// Updates any topology link entities to down state if they don't have a counterpart in the southbound links report
//func (r *LinkReconciler) updateDownedLinks(object *topo.Object, report *southbound.LinkReport) {
//	// TODO: implement me; may require enhanced relations query for efficient implementation
//	// Get the device ports first
//	portsFilter := &topo.RelationFilter{SrcId: string(object.ID), RelationKind: topo.HasKind, TargetKind: topo.PortKind}
//	stream, err := r.topoClient.Query(r.ctx, &topo.QueryRequest{Filters: &topo.Filters{RelationFilter: portsFilter}})
//	if err != nil {
//		log.Warnf("Unable to query device ports for %s: %+v", object.ID, err)
//	}
//
//	for {
//		resp, err := stream.Recv()
//		if err != nil {
//			if err != io.EOF {
//				log.Warnf("Unable to read device ports for %s: %+v", object.ID, err)
//			}
//			return
//		}
//
//		// If the returned object isn't a source of any relations, ignore it
//		if len(resp.Object.GetEntity().SrcRelationIDs) == 0 {
//			continue
//		}
//
//		// If the port is in the southbound link report link map, it means no pruning is needed
//		portAspect := &topo.Port{}
//		if err = resp.Object.GetAspect(portAspect); err != nil {
//			log.Warnf("Unable to get port aspect from port entity %s: %+v", resp.Object.ID, err)
//			continue
//		}
//		if _, ok := report.Links[portAspect.Number]; ok {
//			// Port has an ingress link, no need to prune
//			continue
//		}
//
//		// Otherwise, get the link that terminates at this port and mark it as DOWN
//		r.updateDownedIngressLink(resp.Object)
//	}
//}
//
//func (r *LinkReconciler) updateDownedIngressLink(portObject *topo.Object) {
//	linkFilter := &topo.RelationFilter{SrcId: string(portObject.ID), RelationKind: topo.TerminatesKind, TargetKind: topo.LinkKind}
//	stream, err := r.topoClient.Query(r.ctx, &topo.QueryRequest{Filters: &topo.Filters{RelationFilter: linkFilter}})
//	if err != nil {
//		log.Warnf("Unable to query ingress link for port %s: %+v", portObject.ID, err)
//	}
//
//	// Assume at most one response
//	resp, err := stream.Recv()
//	if err != nil {
//		if err != io.EOF {
//			log.Warnf("Unable to read ingress link for port %s: %+v", portObject.ID, err)
//		}
//		return
//	}
//
//	linkAspect := &topo.Link{}
//	if err = resp.Object.GetAspect(linkAspect); err != nil {
//		log.Warnf("Unable to get ingress link aspect for %s: %v", resp.Object.ID, err)
//		return
//	}
//
//	if linkAspect.Status != "DOWN" {
//		// If the link is not already marked as down, mark it as such
//		linkAspect = &topo.Link{Status: "DOWN", LastChange: uint64(time.Now().UnixNano())}
//		if err = resp.Object.SetAspect(linkAspect); err != nil {
//			log.Warnf("Unable to set ingress link aspect for %s: %v", resp.Object.ID, err)
//			return
//		}
//
//		if _, err = r.topoClient.Update(r.ctx, &topo.UpdateRequest{Object: resp.Object}); err != nil {
//			log.Warnf("Unable to update ingress link aspect for %s: %v", resp.Object.ID, err)
//			return
//		}
//		log.Infof("Updated status of link %s: %+v", resp.Object.ID, linkAspect)
//	}
//}
