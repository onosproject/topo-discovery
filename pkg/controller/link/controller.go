// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package link

import (
	"context"
	"encoding/json"
	gogotypes "github.com/gogo/protobuf/types"
	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-lib-go/pkg/certs"
	"github.com/onosproject/onos-lib-go/pkg/controller"
	"github.com/onosproject/onos-lib-go/pkg/errors"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	gclient "github.com/onosproject/topo-discovery/pkg/client/gnmi"
	"github.com/onosproject/topo-discovery/pkg/controller/types"
	"github.com/onosproject/topo-discovery/pkg/controller/utils"
	"github.com/onosproject/topo-discovery/pkg/store/topo"
	"github.com/openconfig/gnmi/proto/gnmi"

	"time"
)

var log = logging.GetLogger()

const (
	defaultTimeout  = 30 * time.Second
	logLinkEntityID = "Link entity ID"
	logTargetID     = "TargetID"
)

// NewController returns a new link controller
func NewController(topo topo.Store) *controller.Controller {
	c := controller.NewController("link")
	c.Watch(&TopoWatcher{
		topo: topo,
	})
	c.Reconcile(&Reconciler{
		topo: topo,
	})
	return c
}

// Reconciler reconciles link entities
type Reconciler struct {
	topo topo.Store
}

func (r *Reconciler) unmarshalNotifications(notification []*gnmi.Notification) (types.OpenconfigInterfaces, error) {
	var interfaces types.OpenconfigInterfaces
	err := json.Unmarshal(notification[0].Update[0].Val.GetJsonIetfVal(), &interfaces)
	if err != nil {
		return interfaces, err
	}
	return interfaces, nil
}

func (r *Reconciler) extractLinks(interfaces types.OpenconfigInterfaces) ([]*topoapi.Link, error) {
	var links []*topoapi.Link
	for _, interfaceVal := range interfaces.OpenconfigInterfacesInterface {
		subInterfaces := interfaceVal.Subinterfaces.Subinterface
		for _, subInterface := range subInterfaces {
			subIntAddresses := subInterface.OpenconfigIfIPIpv4.Addresses.Address
			subIntNeighbors := subInterface.OpenconfigIfIPIpv4.Neighbors.Neighbor
			if len(subIntNeighbors) != 0 {
				for _, neighbor := range subIntNeighbors {
					forwardLink := &topoapi.Link{
						SourceIP: &topoapi.IPAddress{
							Type: topoapi.IPAddress_IPV4,
							IP:   subIntAddresses[0].IP,
						},
						DestinationIP: &topoapi.IPAddress{
							Type: topoapi.IPAddress_IPV4,
							IP:   neighbor.IP,
						},
					}
					reverseLink := &topoapi.Link{
						SourceIP: &topoapi.IPAddress{
							Type: topoapi.IPAddress_IPV4,
							IP:   neighbor.IP,
						},
						DestinationIP: &topoapi.IPAddress{
							Type: topoapi.IPAddress_IPV4,
							IP:   subIntAddresses[0].IP,
						},
					}
					links = append(links, forwardLink)
					links = append(links, reverseLink)
				}
			}
		}
	}

	return links, nil
}

// Reconcile reconciles link entities for interfaces between programmable entities
func (r *Reconciler) Reconcile(id controller.ID) (controller.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	targetID := id.Value.(topoapi.ID)
	log.Infow("Reconciling links for target", logTargetID, targetID)
	targetEntity, err := r.topo.Get(ctx, targetID)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Errorw("Failed reconciling links for Target", logTargetID, targetID, "error", err)
			return controller.Result{}, err
		}
		return controller.Result{}, nil
	}

	opts, err := certs.HandleCertPaths("", "", "", true)
	if err != nil {
		return controller.Result{}, err
	}
	configurable := &topoapi.Configurable{}
	err = targetEntity.GetAspect(configurable)
	if err != nil {
		log.Warnw("Failed reconciling links for Target", logTargetID, targetID, "error", err)
		return controller.Result{}, err
	}

	onosConfigDest, err := gclient.NewDestination("onos-config:5150", targetID, &topoapi.TLSOptions{
		Insecure: true,
	})
	if err != nil {
		log.Warnw("Failed reconciling interfaces for Target", logTargetID, targetID, "error", err)
		return controller.Result{}, err
	}

	gnmiClient, err := gclient.Connect(ctx, *onosConfigDest, opts...)
	if err != nil {
		log.Warnw("Failed reconciling interfaces for Target", logTargetID, targetID, "error", err)
		return controller.Result{}, err
	}

	var pbPathElements []*gnmi.PathElem
	pbPathElements = append(pbPathElements, &gnmi.PathElem{Name: types.InterfacesPath})
	gnmiPath := &gnmi.Path{
		Elem:   pbPathElements,
		Target: string(targetID),
	}
	var paths []*gnmi.Path
	paths = append(paths, gnmiPath)
	gnmiGetReq := &gnmi.GetRequest{
		Type:     gnmi.GetRequest_STATE,
		Encoding: gnmi.Encoding_JSON_IETF,
		Path:     paths,
	}

	getResponse, err := gnmiClient.Get(ctx, gnmiGetReq)
	if err != nil {
		log.Warnw("Failed reconciling links for Target", logTargetID, targetID, "error", err)
		return controller.Result{}, err
	}

	interfaces, err := r.unmarshalNotifications(getResponse.Notification)
	if err != nil {
		log.Warnw("Failed reconciling links for Target", logTargetID, targetID, "error", err)
		return controller.Result{}, err
	}

	links, err := r.extractLinks(interfaces)
	if err != nil {
		return controller.Result{}, err
	}

	if ok, err := r.createLinkEntities(ctx, links); err != nil {
		return controller.Result{}, err
	} else if ok {
		return controller.Result{}, nil
	}
	return controller.Result{}, nil
}

func (r *Reconciler) createLinkEntities(ctx context.Context, links []*topoapi.Link) (bool, error) {
	for _, link := range links {
		if _, err := r.createLinkEntity(ctx, link); err != nil {
			return false, err
		}
	}
	return true, nil
}

// findInterface finds an interface which has a specific ip address in its phy interface aspect
func (r *Reconciler) findInterface(ctx context.Context, ip string) (topoapi.ID, error) {
	filter := &topoapi.Filters{
		KindFilter: &topoapi.Filter{
			Filter: &topoapi.Filter_Equal_{
				Equal_: &topoapi.EqualFilter{
					Value: topoapi.InterfaceKind,
				},
			},
		},
	}
	interfaceList, err := r.topo.List(ctx, filter)
	if err != nil {
		return "", err
	}
	for _, object := range interfaceList {
		phyInterfaceAspect := &topoapi.PhyInterface{}
		err = object.GetAspect(phyInterfaceAspect)
		if err != nil {
			return "", err
		}
		if phyInterfaceAspect.Ip.GetIP() == ip {
			return object.ID, nil
		}
	}
	return "", errors.NewNotFound("interface with the given ip is not found")
}

func (r *Reconciler) createLinkEntity(ctx context.Context, link *topoapi.Link) (bool, error) {
	sourceInterfaceID, err := r.findInterface(ctx, link.SourceIP.GetIP())
	if err != nil {
		return false, errors.NewNotFound("source interface not found:", link.SourceIP.GetIP())
	}
	destInterfaceID, err := r.findInterface(ctx, link.DestinationIP.GetIP())
	if err != nil {
		return false, errors.NewNotFound("dest interface not found:", link.DestinationIP.GetIP())
	}

	linkEntityID := utils.GetLinkID(sourceInterfaceID, destInterfaceID)
	object, err := r.topo.Get(ctx, linkEntityID)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Warnw("Creating link entity failed")
			return false, err
		}
		log.Infow("Creating link entity", logLinkEntityID, linkEntityID)
		linkEntity := &topoapi.Object{
			ID:   linkEntityID,
			Type: topoapi.Object_ENTITY,
			Obj: &topoapi.Object_Entity{
				Entity: &topoapi.Entity{
					KindID: topoapi.LinkKind,
				},
			},
			Aspects: make(map[string]*gogotypes.Any),
			Labels:  map[string]string{},
		}

		err = linkEntity.SetAspect(link)
		if err != nil {
			return false, err
		}

		err = r.topo.Create(ctx, linkEntity)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				log.Warnw("Creating link entity failed", logLinkEntityID, linkEntityID, "error", err)
				return false, err
			}
			return false, nil
		}
		return true, nil
	}

	linkAspect := &topoapi.Link{}
	err = object.GetAspect(linkAspect)
	if err == nil {
		log.Debugf("link aspect is already set", linkAspect)
		return false, nil
	}

	log.Debugw("Updating link aspect", logLinkEntityID, linkEntityID)
	err = object.SetAspect(link)
	if err != nil {
		log.Warnw("Updating link aspect failed", logLinkEntityID, linkEntityID, "error", err)
		return false, err
	}
	err = r.topo.Update(ctx, object)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Warnw("Updating link entity failed", logLinkEntityID, linkEntityID, "error", err)
			return false, err
		}
		return false, nil
	}
	return true, nil

}
