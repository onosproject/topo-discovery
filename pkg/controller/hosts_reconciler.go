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
}

// NewHostReconciler creates a new host reconciler context
func NewHostReconciler(ctx context.Context, topoClient topo.TopoClient) *HostReconciler {
	return &HostReconciler{
		topoClient:    topoClient,
		ctx:           ctx,
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

	// process all hosts from the report
	for _, host := range hostReport.Hosts {
		r.reconcileHost(host)
	}
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
	// ToDo - a placeholder for pruning hosts
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
