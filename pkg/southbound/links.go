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
	"strconv"
	"sync"
)

// Link holds information about an ingress link
type Link struct {
	IngressDevice string
	IngressPort   uint32
	EgressDevice  string
	EgressPort    uint32
	CreateTime    uint64
}

// LinkReport provides results of a link query against the link local agent
type LinkReport struct {
	AgentID string
	Links   map[uint32]*Link
}

// IngressLinkListener is an abstraction of an entity capable of handling ingress link changes
type IngressLinkListener interface {
	LinkAdded(link *Link)
	LinkDeleted(link *Link)
}

// IngressLinkDiscovery is an abstraction of an entity capable of discovering ingress links
type IngressLinkDiscovery interface {
	GetIngressLinks(object *topo.Object, listener IngressLinkListener) (*LinkReport, error)
}

// Implementation of IngressLinkDiscovery via gNMI against link agent
type gNMILinkDiscovery struct {
	IngressLinkDiscovery
	lock          sync.RWMutex
	agentContexts map[topo.ID]*agentContext
}

// Link agent gNMI link discovery context
type agentContext struct {
	object    *topo.Object
	agent     *stratum.GNMI
	agentID   string
	listener  IngressLinkListener
	ctx       context.Context
	ctxCancel context.CancelFunc

	lock   sync.RWMutex
	report *LinkReport
}

// NewGNMILinkDiscovery returns new ingress link discovery based on gNMI
func NewGNMILinkDiscovery() IngressLinkDiscovery {
	return &gNMILinkDiscovery{agentContexts: make(map[topo.ID]*agentContext, 4)}
}

// GetIngressLinks returns a map of link descriptors obtained via gNMI get request on state/link[port=...] query
func (ld *gNMILinkDiscovery) GetIngressLinks(object *topo.Object, listener IngressLinkListener) (*LinkReport, error) {
	// Get the gNMI agent context; get existing one or create one
	ac, err := ld.getAgentContext(object, listener)
	if err != nil {
		return nil, err
	}

	report := &LinkReport{AgentID: ac.agentID, Links: make(map[uint32]*Link)}

	// If listeners has been specified, do the link discovery; otherwise, we just wanted the agent ID
	if listener != nil {
		// Get the list of links
		resp, err := ac.agent.Client.Get(ac.agent.Context, &gnmi.GetRequest{
			Path: []*gnmi.Path{gnmiutils.ToPath("state/link[port=...]")},
		})
		if err != nil {
			return nil, err
		}
		if len(resp.Notification) == 0 {
			return nil, errors.NewInvalid("no link data received")
		}

		log.Debugf("%s: Got links: %+v", object.ID, resp.Notification)
		for _, notification := range resp.Notification {
			ac.processLinkNotification(notification, report.Links)
		}

		ac.lock.Lock()
		ac.report = report
		ac.lock.Unlock()

		// Once links are discovered kick off subscription-base link monitor, if not started yet
		ac.startMonitor()
	}
	return report, nil
}

func (ld *gNMILinkDiscovery) getAgentContext(object *topo.Object, listener IngressLinkListener) (*agentContext, error) {
	ld.lock.Lock()
	defer ld.lock.Unlock()

	ac, ok := ld.agentContexts[object.ID]
	if !ok {
		ac = &agentContext{object: object, listener: listener}
		ld.agentContexts[object.ID] = ac
	}

	// If we haven't established an agent context yet, do so.
	if ac.agent == nil {
		// Connect to the agent using gNMI
		var err error
		localAgents := &topo.LocalAgents{}
		if err = object.GetAspect(localAgents); err != nil {
			log.Warnf("Object %s doesn't have onos.topo.LocalAgents aspect", object.ID)
			return nil, err
		}

		// Connect to the device's link local agent using gNMI
		ac.agent, err = stratum.NewGNMI(string(object.ID), localAgents.LinkAgentEndpoint, true)
		if err != nil {
			log.Warnf("Unable to connect to Stratum link local agent gNMI %s: %+v", object.ID, err)
			return nil, err
		}
	}

	if ac.agentID == "" {
		// Get the agent ID
		var err error
		if ac.agentID, err = getAgentID(ac.agent); err != nil {
			log.Warnf("Unable to retrieve agent ID for %s: %+v", ac.object.ID, err)
			return nil, err
		}
	}
	return ac, nil
}

func getAgentID(device *stratum.GNMI) (string, error) {
	resp, err := device.Client.Get(device.Context, &gnmi.GetRequest{
		Path: []*gnmi.Path{gnmiutils.ToPath("state/agent-id")},
	})
	if err != nil {
		return "", err
	}
	if len(resp.Notification) == 0 || len(resp.Notification[0].Update) == 0 {
		return "", errors.NewInvalid("agent-id not received")
	}
	return resp.Notification[0].Update[0].Val.GetStringVal(), nil
}

func (ac *agentContext) processLinkNotification(notification *gnmi.Notification, links map[uint32]*Link) {
	var link *Link
	for _, update := range notification.Update {
		if update.Path.Elem[1].Name == "link" { // FIXME elsewhere: this is needed to only pick-up link updates
			port := getPortNumber(update.Path)
			if port == 0 {
				continue
			}
			link = ac.getLink(links, port)
			if link != nil {
				last := len(update.Path.Elem) - 1
				switch update.Path.Elem[last].Name {
				case "egress-port":
					link.EgressPort = uint32(update.Val.GetIntVal())
				case "egress-device":
					link.EgressDevice = update.Val.GetStringVal()
				case "create-time":
					link.CreateTime = update.Val.GetUintVal()
				}
			}
		}
	}
}

func getPortNumber(path *gnmi.Path) uint32 {
	port, err := strconv.ParseInt(path.Elem[1].Key["port"], 10, 32)
	if err != nil {
		log.Warnf("Key 'port' is not a number on %+v: %+v", path.Elem[1], err)
		return 0
	}
	return uint32(port)
}

func (ac *agentContext) getLink(links map[uint32]*Link, port uint32) *Link {
	link, ok := links[port]
	if !ok {
		link = &Link{IngressDevice: ac.agentID, IngressPort: port}
		links[port] = link
	}

	if link.IngressDevice == "" {
		// If the link we found has no ingress device set, and there is no agentID yet to update it with, bail
		if ac.agentID == "" {
			return nil
		}

		// Otherwise update the agent ID
		link.IngressDevice = ac.agentID
	}
	return link
}

// Starts the link monitor if not already started
func (ac *agentContext) startMonitor() {
	if ac.ctxCancel == nil {
		ac.stopMonitor()
		log.Infof("Starting link monitor...")
		go ac.monitorLinkChanges()
	}
}

// Stops the link monitor
func (ac *agentContext) stopMonitor() {
	if ac.ctxCancel != nil {
		log.Infof("Stopping link monitor...")
		ac.ctxCancel()
		ac.ctxCancel = nil
	}
}

// Issues subscribe request for port state updates and monitors the stream for update notifications
func (ac *agentContext) monitorLinkChanges() {
	log.Infof("Link monitor started")
	ac.ctx, ac.ctxCancel = context.WithCancel(context.Background())
	stream, err := ac.agent.Client.Subscribe(ac.ctx)
	if err != nil {
		log.Warn("Unable to subscribe for link changes: %+v", err)
		return
	}

	subscriptions := []*gnmi.Subscription{{Path: gnmiutils.ToPath("state/link[port=...]")}}
	if err = stream.Send(&gnmi.SubscribeRequest{
		Request: &gnmi.SubscribeRequest_Subscribe{
			Subscribe: &gnmi.SubscriptionList{Subscription: subscriptions},
		}}); err != nil {
		log.Warn("Unable to send subscription request for link changes: %+v", err)
		return
	}

	for {
		resp, err := stream.Recv()
		if err != nil {
			if err != io.EOF {
				log.Warn("Unable to read subscription response for link changes: %+v", err)
			}
			log.Infof("Link monitor stopped")
			return
		}
		log.Debugf("Got link update %+v", resp.GetUpdate())
		ac.processLinkResponse(resp)
	}
}

func (ac *agentContext) processLinkResponse(resp *gnmi.SubscribeResponse) {
	ac.lock.Lock()
	defer ac.lock.Unlock()

	// Handle deletions
	for _, path := range resp.GetUpdate().Delete {
		ingressPort := getPortNumber(path)
		if link := ac.getLink(ac.report.Links, ingressPort); link != nil {
			delete(ac.report.Links, ingressPort) // update the most recent report by deleting this link
			ac.listener.LinkDeleted(link)
		}
	}

	// Handle additions
	links := make(map[uint32]*Link, 0)
	ac.processLinkNotification(resp.GetUpdate(), links)
	for _, link := range links {
		ac.report.Links[link.IngressPort] = link // update the most recent report with this new link
		ac.listener.LinkAdded(link)
	}
}
