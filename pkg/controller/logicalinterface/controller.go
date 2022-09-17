// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package logicalinterface

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
	logInterfaceEntityID = "Logical Interface entity ID"
	logTargetID          = "TargetID"
	interfacesPath       = "openconfig-interfaces:interfaces/interface"
)

// NewController returns a new gNMI connection  controller
func NewController(topo topo.Store) *controller.Controller {
	c := controller.NewController("logical-interfaces")
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

func (r *Reconciler) extractInterfaces(interfaces types.OpenconfigInterfacesInterfacesInterface, targetID topoapi.ID) ([]*topoapi.LogicalInterface, error) {
	var logicalInterfaces []*topoapi.LogicalInterface
	for _, interfaceVal := range interfaces.OpenconfigInterfacesInterface {
		logicalInterface := &topoapi.LogicalInterface{}
		logicalInterface.DisplayName = interfaceVal.Name
		logicalInterface.TargetID = string(targetID)
		if len(interfaceVal.Subinterfaces.Subinterface) != 0 {
			if len(interfaceVal.Subinterfaces.Subinterface[0].OpenconfigIfIPIpv4.Addresses.Address) != 0 {
				logicalInterface.Ip = &topoapi.IPAddress{
					Type: topoapi.IPAddress_IPV4,
					IP:   interfaceVal.Subinterfaces.Subinterface[0].OpenconfigIfIPIpv4.Addresses.Address[0].IP,
				}
			}

		}

		logicalInterfaces = append(logicalInterfaces, logicalInterface)
	}
	return logicalInterfaces, nil
}

// Reconcile reconciles logical interface entities for a programmable target
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

	logicalInterfaces, err := r.extractInterfaces(interfaces, targetID)
	if err != nil {
		return controller.Result{}, err
	}

	if ok, err := r.createLogicalInterfaceEntities(ctx, logicalInterfaces); err != nil {
		return controller.Result{}, err
	} else if ok {
		return controller.Result{}, nil
	}
	return controller.Result{}, nil
}

func (r *Reconciler) createLogicalInterfaceEntities(ctx context.Context, logicalInterfaces []*topoapi.LogicalInterface) (bool, error) {
	for _, logicalInterface := range logicalInterfaces {
		if _, err := r.createLogicalInterfaceEntity(ctx, logicalInterface); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (r *Reconciler) createLogicalInterfaceEntity(ctx context.Context, logicalInterface *topoapi.LogicalInterface) (bool, error) {
	targetID := logicalInterface.TargetID
	logicalInterfaceEntityID := utils.GetLogicalInterfaceID(targetID, logicalInterface.DisplayName)
	log.Infow("Creating logical interface entity", logTargetID, targetID, logInterfaceEntityID, logicalInterfaceEntityID)
	object, err := r.topo.Get(ctx, logicalInterfaceEntityID)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Warnw("Creating logical interface entity failed", logTargetID, targetID, logInterfaceEntityID, logicalInterfaceEntityID)
			return false, err
		}

		logicalInterfaceEntity := &topoapi.Object{
			ID:   logicalInterfaceEntityID,
			Type: topoapi.Object_ENTITY,
			Obj: &topoapi.Object_Entity{
				Entity: &topoapi.Entity{
					KindID: topoapi.InterfaceKind,
				},
			},
			Aspects: make(map[string]*gogotypes.Any),
			Labels:  map[string]string{},
		}

		err = logicalInterfaceEntity.SetAspect(logicalInterface)
		if err != nil {
			return false, err
		}

		err = r.topo.Create(ctx, logicalInterfaceEntity)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				log.Warnw("Creating logical interface entity failed", logTargetID, targetID, logInterfaceEntityID, logicalInterfaceEntityID, "error", err)
				return false, err
			}
			return false, nil
		}
		return true, nil
	}

	logicalInterfaceAspect := &topoapi.LogicalInterface{}
	err = object.GetAspect(logicalInterfaceAspect)
	if err == nil {
		log.Debugf("Logical interface aspect is already set", logicalInterfaceAspect)
		return false, nil
	}

	log.Debugw("Updating logical interface aspect", logTargetID, targetID, logInterfaceEntityID, logicalInterfaceEntityID)
	err = object.SetAspect(logicalInterfaceAspect)
	if err != nil {
		log.Warnw("Updating logical interface aspect failed", logTargetID, targetID, logInterfaceEntityID, logicalInterfaceEntityID, "error", err)
		return false, err
	}
	err = r.topo.Update(ctx, object)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Warnw("Updating logical interface entity failed", logTargetID, targetID, logInterfaceEntityID, logicalInterfaceEntityID, "error", err)
			return false, err
		}
		return false, nil
	}
	return true, nil

}
