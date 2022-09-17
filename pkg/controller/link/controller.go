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
	"github.com/onosproject/topo-discovery/pkg/controller/types"
	"github.com/onosproject/topo-discovery/pkg/controller/utils"
	"github.com/onosproject/topo-discovery/pkg/store/topo"
	"github.com/openconfig/gnmi/proto/gnmi"

	"google.golang.org/grpc"
	"time"
)

var log = logging.GetLogger()

const (
	defaultTimeout  = 30 * time.Second
	logLinkEntityID = "Link entity ID"
	logTargetID     = "TargetID"
	interfacesPath  = "openconfig-interfaces:interfaces/interface"
)

// NewController returns a new gNMI connection  controller
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

// Reconciler reconciles gNMI connections
type Reconciler struct {
	topo topo.Store
}

func (r *Reconciler) unmarshalNotifications(notification []*gnmi.Notification) (types.OpenconfigInterfacesInterfacesInterface, error) {
	var interfaces types.OpenconfigInterfacesInterfacesInterface
	err := json.Unmarshal(notification[0].Update[0].Val.GetJsonIetfVal(), &interfaces)
	if err != nil {
		return interfaces, err
	}
	return interfaces, nil
}

func (r *Reconciler) extractLinks(interfaces types.OpenconfigInterfacesInterfacesInterface) ([]*topoapi.Link, error) {
	var links []*topoapi.Link
	for _, interfaceVal := range interfaces.OpenconfigInterfacesInterface {
		subInterfaces := interfaceVal.Subinterfaces.Subinterface
		for _, subInterface := range subInterfaces {
			subIntAddresses := subInterface.OpenconfigIfIPIpv4.Addresses.Address
			subIntNeighbors := subInterface.OpenconfigIfIPIpv4.Neighbors.Neighbor
			if len(subIntNeighbors) != 0 {
				for _, neighbor := range subIntNeighbors {
					link := &topoapi.Link{
						SourceIP: &topoapi.IPAddress{
							Type: topoapi.IPAddress_IPV4,
							IP:   subIntAddresses[0].IP,
						},
						DestinationIP: &topoapi.IPAddress{
							Type: topoapi.IPAddress_IPV4,
							IP:   neighbor.IP,
						},
					}
					links = append(links, link)
				}
			}
		}
	}

	return links, nil
}

// Reconcile reconciles port entities for a programmable target
func (r *Reconciler) Reconcile(id controller.ID) (controller.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	targetID := id.Value.(topoapi.ID)
	log.Infow("Reconciling links for target", logTargetID, targetID)
	targetEntity, err := r.topo.Get(ctx, targetID)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Errorw("Failed reconciling Ports for Target", logTargetID, targetID, "error", err)
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
		log.Warnw("Failed reconciling Ports for Target", logTargetID, targetID, "error", err)
		return controller.Result{}, err
	}

	gnmiConn, err := grpc.Dial("onos-config:5150", opts...)
	if err != nil {
		log.Warnw("Failed reconciling Ports for Target", logTargetID, targetID, "error", err)
		return controller.Result{}, err
	}
	gnmiClient := gnmi.NewGNMIClient(gnmiConn)

	var pbPathElements []*gnmi.PathElem
	pbPathElements = append(pbPathElements, &gnmi.PathElem{Name: interfacesPath})
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
		log.Warnw("Failed reconciling Ports for Target", logTargetID, targetID, "error", err)
		return controller.Result{}, err
	}

	interfaces, err := r.unmarshalNotifications(getResponse.Notification)
	if err != nil {
		log.Warnw("Failed reconciling Ports for Target", logTargetID, targetID, "error", err)
		return controller.Result{}, err
	}

	links, err := r.extractLinks(interfaces)
	if err != nil {
		return controller.Result{}, err
	}

	if ok, err := r.createLinkEntities(ctx, targetID, links); err != nil {
		return controller.Result{}, err
	} else if ok {
		return controller.Result{}, nil
	}
	return controller.Result{}, nil
}

func (r *Reconciler) createLinkEntities(ctx context.Context, targetID topoapi.ID, links []*topoapi.Link) (bool, error) {
	for _, link := range links {
		if _, err := r.createLinkEntity(ctx, targetID, link); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (r *Reconciler) createLinkEntity(ctx context.Context, targetID topoapi.ID, link *topoapi.Link) (bool, error) {
	linkEntityID := utils.GetLinkID("", "")
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
