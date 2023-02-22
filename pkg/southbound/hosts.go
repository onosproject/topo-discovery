// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package southbound

import (
	"context"
	"github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-lib-go/pkg/errors"
	"github.com/onosproject/onos-net-lib/pkg/gnmiutils"
	"github.com/onosproject/onos-net-lib/pkg/stratum"
	"github.com/openconfig/gnmi/proto/gnmi"
	"io"
	"sync"
)

// Host is a simple representation of a host network interface discovered by the ONOS lite
type Host struct {
	MAC        string
	IP         string
	Port       uint32
	CreateTime uint64
}

// HostReport provides results of a host query against the host local agent
type HostReport struct {
	AgentID string
	Hosts   map[string]*Host
}

// HostListener is an abstraction of an entity capable of handling host changes
type HostListener interface {
	HostAdded(host *Host, agentID string)
	HostDeleted(host *Host, agentID string)
}

// HostDiscovery is an abstraction of an entity capable of discovering hosts
type HostDiscovery interface {
	GetHosts(object *topo.Object, listener HostListener) (*HostReport, error)
}

// Implementation of HostDiscovery via gNMI against host agent
type gNMIHostDiscovery struct {
	HostDiscovery
	lock         sync.RWMutex
	hostContexts map[topo.ID]*hostContext
}

// Host agent gNMI link discovery context
type hostContext struct {
	object    *topo.Object
	agent     *stratum.GNMI
	agentID   string
	listener  HostListener
	ctx       context.Context
	ctxCancel context.CancelFunc

	lock   sync.RWMutex
	report *HostReport
}

// NewGNMIHostDiscovery returns new host discovery based on gNMI
func NewGNMIHostDiscovery() HostDiscovery {
	return &gNMIHostDiscovery{hostContexts: make(map[topo.ID]*hostContext, 4)} // ToDo - why we declare map of size 4?
}

// GetHosts returns a map of link descriptors obtained via gNMI get request on state/host[mac=...] query
func (ld *gNMIHostDiscovery) GetHosts(object *topo.Object, listener HostListener) (*HostReport, error) {
	// Get the gNMI host context; get existing one or create one
	ac, err := ld.getHostContext(object, listener)
	if err != nil {
		return nil, err
	}

	report := &HostReport{AgentID: ac.agentID, Hosts: make(map[string]*Host)}

	// Get the list of hosts
	resp, err := ac.agent.Client.Get(ac.agent.Context, &gnmi.GetRequest{
		Path: []*gnmi.Path{gnmiutils.ToPath("state/host[mac=...]")},
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Notification) == 0 {
		return nil, errors.NewInvalid("no host data received")
	}

	log.Debugf("%s: Got hosts: %+v", object.ID, resp.Notification)
	for _, notification := range resp.Notification {
		ac.processHostNotification(notification, report.Hosts)
	}

	ac.lock.Lock()
	ac.report = report
	ac.lock.Unlock()

	// Once hosts are discovered kick off subscription-base host monitor, if not started yet
	ac.startMonitor()

	return report, nil
}

func (ld *gNMIHostDiscovery) getHostContext(object *topo.Object, listener HostListener) (*hostContext, error) {
	ld.lock.Lock()
	defer ld.lock.Unlock()

	ac, ok := ld.hostContexts[object.ID]
	if !ok {
		ac = &hostContext{object: object, listener: listener}
		ld.hostContexts[object.ID] = ac
	}

	// If we haven't established a host context yet, do so.
	if ac.agent == nil {
		// Connect to the host using gNMI
		var err error
		localAgents := &topo.LocalAgents{}
		if err = object.GetAspect(localAgents); err != nil {
			log.Warnf("Object %s doesn't have onos.topo.LocalAgents aspect", object.ID)
			return nil, err
		}

		// Connect to the device's host local agent using gNMI
		ac.agent, err = stratum.NewGNMI(string(object.ID), localAgents.HostAgentEndpoint, true)
		if err != nil {
			log.Warnf("Unable to connect to Stratum host local agent gNMI %s: %+v", object.ID, err)
			return nil, err
		}

		// Get the agent ID
		if ac.agentID, err = getAgentID(ac.agent); err != nil {
			log.Warnf("Unable to retrieve agent ID for %s: %+v", ac.object.ID, err)
			return nil, err
		}
	}
	return ac, nil
}

func (hc *hostContext) processHostNotification(notification *gnmi.Notification, hosts map[string]*Host) {
	var host *Host
	for _, update := range notification.Update {
		if update.Path.Elem[1].Name == "host" { // FIXME elsewhere: this is needed to only pick-up host updates
			mac := getMacAddress(update.Path)
			host = hc.getHost(hosts, mac)
			last := len(update.Path.Elem) - 1
			switch update.Path.Elem[last].Name {
			case "port":
				host.Port = uint32(update.Val.GetIntVal())
			case "ip-address":
				host.IP = update.Val.GetStringVal()
			case "create-time":
				host.CreateTime = update.Val.GetUintVal()
			}
		}
	}
}

func getMacAddress(path *gnmi.Path) string {
	mac, ok := path.Elem[1].Key["mac"]
	if !ok {
		log.Errorf("Key 'mac' was not found on %+v", path.Elem[1])
		return ""
	}
	return mac
}

func (hc *hostContext) getHost(hosts map[string]*Host, mac string) *Host {
	host, ok := hosts[mac]
	if !ok {
		host = &Host{MAC: mac}
		hosts[mac] = host
	}
	return host
}

// Starts the host monitor if not already started
func (hc *hostContext) startMonitor() {
	if hc.ctxCancel == nil {
		hc.stopMonitor()
		log.Infof("Starting host monitor...")
		go hc.monitorHostChanges()
	}
}

// Stops the host monitor
func (hc *hostContext) stopMonitor() {
	if hc.ctxCancel != nil {
		log.Infof("Stopping link monitor...")
		hc.ctxCancel()
		hc.ctxCancel = nil
	}
}

// Issues subscribe request for host state updates and monitors the stream for update notifications
func (hc *hostContext) monitorHostChanges() {
	log.Infof("Host monitor started")
	hc.ctx, hc.ctxCancel = context.WithCancel(context.Background())
	stream, err := hc.agent.Client.Subscribe(hc.ctx)
	if err != nil {
		log.Warn("Unable to subscribe for host changes: %+v", err)
		return
	}

	subscriptions := []*gnmi.Subscription{{Path: gnmiutils.ToPath("state/host[mac=...]")}}
	if err = stream.Send(&gnmi.SubscribeRequest{
		Request: &gnmi.SubscribeRequest_Subscribe{
			Subscribe: &gnmi.SubscriptionList{Subscription: subscriptions},
		}}); err != nil {
		log.Warn("Unable to send subscription request for host changes: %+v", err)
		return
	}

	for {
		resp, err := stream.Recv()
		if err != nil {
			if err != io.EOF {
				log.Warn("Unable to read subscription response for host changes: %+v", err)
			}
			log.Infof("Host monitor stopped")
			return
		}
		log.Debugf("Got host update %+v", resp.GetUpdate())
		hc.processHostResponse(resp)
	}
}

func (hc *hostContext) processHostResponse(resp *gnmi.SubscribeResponse) {
	hc.lock.Lock()
	defer hc.lock.Unlock()

	// Handle deletions
	for _, path := range resp.GetUpdate().Delete {
		mac := getMacAddress(path)
		if host := hc.getHost(hc.report.Hosts, mac); host != nil {
			delete(hc.report.Hosts, mac) // update the most recent report by deleting this host
			hc.listener.HostDeleted(host, hc.agentID)
		}
	}

	// Handle additions
	hosts := make(map[string]*Host, 0)
	hc.processHostNotification(resp.GetUpdate(), hosts)
	for _, host := range hosts {
		hc.report.Hosts[host.MAC] = host // update the most recent report with this new host
		hc.listener.HostAdded(host, hc.agentID)
	}
}
