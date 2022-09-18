// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package phyinterface

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
	defaultTimeout       = 30 * time.Second
	logInterfaceEntityID = "Phy Interface entity ID"
	logTargetID          = "TargetID"
	interfacesPath       = "openconfig-interfaces:interfaces/interface"
)

// NewController returns a new gNMI connection  controller
func NewController(topo topo.Store) *controller.Controller {
	c := controller.NewController("phy-interfaces")
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

func (r *Reconciler) extractInterfaces(interfaces types.OpenconfigInterfacesInterfacesInterface, targetID topoapi.ID) ([]*topoapi.PhyInterface, error) {
	var phyInterfaces []*topoapi.PhyInterface
	for _, interfaceVal := range interfaces.OpenconfigInterfacesInterface {
		phyInterface := &topoapi.PhyInterface{}
		phyInterface.DisplayName = interfaceVal.Name
		phyInterface.TargetID = string(targetID)
		if len(interfaceVal.Subinterfaces.Subinterface) != 0 {
			if len(interfaceVal.Subinterfaces.Subinterface[0].OpenconfigIfIPIpv4.Addresses.Address) != 0 {
				phyInterface.Ip = &topoapi.IPAddress{
					Type: topoapi.IPAddress_IPV4,
					IP:   interfaceVal.Subinterfaces.Subinterface[0].OpenconfigIfIPIpv4.Addresses.Address[0].IP,
				}
			}
		}

		phyInterfaces = append(phyInterfaces, phyInterface)
	}
	return phyInterfaces, nil
}

// Reconcile reconciles phy interface entities for a programmable target
func (r *Reconciler) Reconcile(id controller.ID) (controller.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	targetID := id.Value.(topoapi.ID)
	log.Infow("Reconciling interfaces for target", logTargetID, targetID)
	targetEntity, err := r.topo.Get(ctx, targetID)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Errorw("Failed reconciling interfaces for Target", logTargetID, targetID, "error", err)
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
		log.Warnw("Failed reconciling interfaces for Target", logTargetID, targetID, "error", err)
		return controller.Result{}, err
	}

	gnmiConn, err := grpc.Dial("onos-config:5150", opts...)
	if err != nil {
		log.Warnw("Failed reconciling interfaces for Target", logTargetID, targetID, "error", err)
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
		log.Warnw("Failed reconciling interfaces for Target", logTargetID, targetID, "error", err)
		return controller.Result{}, err
	}

	interfaces, err := r.unmarshalNotifications(getResponse.Notification)
	if err != nil {
		log.Warnw("Failed reconciling interfaces for Target", logTargetID, targetID, "error", err)
		return controller.Result{}, err
	}

	phyInterfaces, err := r.extractInterfaces(interfaces, targetID)
	if err != nil {
		return controller.Result{}, err
	}

	if ok, err := r.createInterfaceEntities(ctx, phyInterfaces); err != nil {
		return controller.Result{}, err
	} else if ok {
		return controller.Result{}, nil
	}
	return controller.Result{}, nil
}

func (r *Reconciler) createInterfaceEntities(ctx context.Context, phyInterfaces []*topoapi.PhyInterface) (bool, error) {
	for _, phyInterface := range phyInterfaces {
		if _, err := r.createInterfaceEntity(ctx, phyInterface); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (r *Reconciler) createInterfaceEntity(ctx context.Context, phyInterface *topoapi.PhyInterface) (bool, error) {
	targetID := phyInterface.TargetID
	phyInterfaceEntityID := utils.GetPhyInterfaceID(targetID, phyInterface.DisplayName)
	log.Infow("Creating phy interface entity", logTargetID, targetID, logInterfaceEntityID, phyInterfaceEntityID)
	object, err := r.topo.Get(ctx, phyInterfaceEntityID)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Warnw("Creating phy interface entity failed", logTargetID, targetID, logInterfaceEntityID, phyInterfaceEntityID)
			return false, err
		}

		phyInterfaceEntity := &topoapi.Object{
			ID:   phyInterfaceEntityID,
			Type: topoapi.Object_ENTITY,
			Obj: &topoapi.Object_Entity{
				Entity: &topoapi.Entity{
					KindID: topoapi.InterfaceKind,
				},
			},
			Aspects: make(map[string]*gogotypes.Any),
			Labels:  map[string]string{},
		}

		err = phyInterfaceEntity.SetAspect(phyInterface)
		if err != nil {
			return false, err
		}

		err = r.topo.Create(ctx, phyInterfaceEntity)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				log.Warnw("Creating phy interface entity failed", logTargetID, targetID, logInterfaceEntityID, phyInterfaceEntityID, "error", err)
				return false, err
			}
			return false, nil
		}
		return true, nil
	}

	phyInterfaceAspect := &topoapi.PhyInterface{}
	err = object.GetAspect(phyInterfaceAspect)
	if err == nil {
		log.Debugf("Phy interface aspect is already set", phyInterfaceAspect)
		return false, nil
	}

	log.Debugw("Updating phy interface aspect", logTargetID, targetID, logInterfaceEntityID, phyInterfaceEntityID)
	err = object.SetAspect(phyInterfaceAspect)
	if err != nil {
		log.Warnw("Updating phy interface aspect failed", logTargetID, targetID, logInterfaceEntityID, phyInterfaceEntityID, "error", err)
		return false, err
	}
	err = r.topo.Update(ctx, object)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Warnw("Updating phy interface entity failed", logTargetID, targetID, logInterfaceEntityID, phyInterfaceEntityID, "error", err)
			return false, err
		}
		return false, nil
	}
	return true, nil

}
