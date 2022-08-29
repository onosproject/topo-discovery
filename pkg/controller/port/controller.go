// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package port

import (
	"context"
	gogotypes "github.com/gogo/protobuf/types"
	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-lib-go/pkg/certs"
	"github.com/onosproject/onos-lib-go/pkg/controller"
	"github.com/onosproject/onos-lib-go/pkg/errors"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/topo-discovery/pkg/controller/utils"
	"github.com/onosproject/topo-discovery/pkg/store/topo"
	"github.com/onosproject/topo-discovery/pkg/utils/gnmiutils"
	"github.com/openconfig/gnmi/proto/gnmi"
	"google.golang.org/grpc"
	"time"
)

var log = logging.GetLogger()

const (
	defaultTimeout          = 30 * time.Second
	interfacesPath          = "/interfaces/interface[name=*]"
	logPortEntityIDConstant = "Port entity ID"
	logTargetIDConstant     = "TargetID"
)

// NewController returns a new gNMI connection  controller
func NewController(topo topo.Store) *controller.Controller {
	c := controller.NewController("port")
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

func (r *Reconciler) Reconcile(id controller.ID) (controller.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	targetID := id.Value.(topoapi.ID)
	log.Infow("Reconciling Ports for target", logTargetIDConstant, targetID)
	_, err := r.topo.Get(ctx, targetID)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Errorw("Failed reconciling Ports for Target", logTargetIDConstant, targetID, "error", err)
			return controller.Result{}, err
		}
		return controller.Result{}, nil
	}

	opts, err := certs.HandleCertPaths("", "", "", true)
	if err != nil {
		return controller.Result{}, err
	}

	gnmiConn, err := grpc.Dial("onos-config:5150", opts...)
	if err != nil {
		log.Warnw("Failed reconciling Ports for Target", logTargetIDConstant, targetID, "error", err)
		return controller.Result{}, err
	}
	gnmiClient := gnmi.NewGNMIClient(gnmiConn)

	gnmiPath, err := gnmiutils.ToGNMIPath(interfacesPath)
	if err != nil {
		log.Errorw("Failed reconciling Ports for Target", logTargetIDConstant, targetID, "error", err)
		return controller.Result{}, err
	}

	var paths []*gnmi.Path
	gnmiPath.Target = string(targetID)
	paths = append(paths, gnmiPath)

	gnmiGetReq := &gnmi.GetRequest{
		Type:     gnmi.GetRequest_STATE,
		Encoding: gnmi.Encoding_PROTO,
		Path:     paths,
	}

	getResponse, err := gnmiClient.Get(ctx, gnmiGetReq)
	if err != nil {
		log.Warnw("Failed reconciling Ports for Target", logTargetIDConstant, targetID, "error", err)
		return controller.Result{}, err
	}

	port := &topoapi.PhyPort{
		TargetID: string(targetID),
	}
	for _, notification := range getResponse.Notification {
		for _, update := range notification.Update {
			elements := update.Path.Elem
			element := elements[len(elements)-1]
			if element.Name == "name" {
				port.DisplayName = update.GetVal().GetStringVal()

			}
			if element.Name == "port-speed" {
				port.Speed = update.GetVal().GetStringVal()
			}
		}
	}

	if ok, err := r.createPortEntity(ctx, targetID, port); err != nil {
		return controller.Result{}, err
	} else if ok {
		return controller.Result{}, nil
	}

	portEntityID := utils.GetPortID(string(targetID), port.DisplayName)

	if ok, err := r.createPortRelation(ctx, targetID, portEntityID); err != nil {
		return controller.Result{}, err
	} else if ok {
		return controller.Result{}, nil
	}

	return controller.Result{}, nil
}

func (r *Reconciler) createPortEntity(ctx context.Context, targetID topoapi.ID, port *topoapi.PhyPort) (bool, error) {
	portEntityID := utils.GetPortID(string(targetID), port.DisplayName)
	log.Infow("Creating port entity", logTargetIDConstant, targetID, logTargetIDConstant, portEntityID)
	object, err := r.topo.Get(ctx, portEntityID)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Warnw("Creating port entity failed", logTargetIDConstant, targetID, logPortEntityIDConstant, portEntityID)
			return false, err
		}

		portEntity := &topoapi.Object{
			ID:   portEntityID,
			Type: topoapi.Object_ENTITY,
			Obj: &topoapi.Object_Entity{
				Entity: &topoapi.Entity{
					KindID: topoapi.PortKind,
				},
			},
			Aspects: make(map[string]*gogotypes.Any),
			Labels:  map[string]string{},
		}

		err = portEntity.SetAspect(port)
		if err != nil {
			return false, err
		}

		err = r.topo.Create(ctx, portEntity)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				log.Warnw("Creating port entity failed", logTargetIDConstant, targetID, logPortEntityIDConstant, portEntityID, "error", err)
				return false, err
			}
			return false, nil
		}
		return true, nil
	}
	portAspect := &topoapi.PhyPort{}

	err = object.GetAspect(portAspect)
	if err == nil {
		log.Warnw("Port aspect is already set for port entity", logTargetIDConstant, targetID, logPortEntityIDConstant, portEntityID)
		return false, nil
	}
	log.Infow("Updating port aspect", logTargetIDConstant, targetID, logPortEntityIDConstant, portEntityID)
	err = object.SetAspect(portAspect)
	if err != nil {
		log.Warnw("Updating port aspect failed", logTargetIDConstant, targetID, logPortEntityIDConstant, portEntityID, "error", err)
		return false, err
	}
	err = r.topo.Update(ctx, object)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Warnw("Updating port entity failed", logTargetIDConstant, targetID, logPortEntityIDConstant, portEntityID, "error", err)
			return false, err
		}
		return false, nil
	}
	return true, nil
}

func (r *Reconciler) createPortRelation(ctx context.Context, targetID topoapi.ID, portEntityID topoapi.ID) (bool, error) {
	log.Infow("Creating contain port relation for target entity", logTargetIDConstant, targetID, logPortEntityIDConstant, portEntityID)

	portRelationID := utils.GetContainPortRelationID(targetID, portEntityID)
	_, err := r.topo.Get(ctx, portRelationID)

	if err != nil {
		if !errors.IsNotFound(err) {
			log.Warnw("Creating CONTAIN port relation for target entity failed", logTargetIDConstant, targetID, logPortEntityIDConstant, portEntityID, "error", err)
			return false, err
		}
		object := &topoapi.Object{
			ID:   portRelationID,
			Type: topoapi.Object_RELATION,
			Obj: &topoapi.Object_Relation{
				Relation: &topoapi.Relation{
					KindID:      topoapi.CONTAINS,
					SrcEntityID: targetID,
					TgtEntityID: portEntityID,
				},
			},
		}

		err := r.topo.Create(ctx, object)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				log.Warnw("Creating CONTAIN port relation for target entity failed", logTargetIDConstant, targetID, logPortEntityIDConstant, portEntityID, "error", err)
				return false, err
			}
			return false, nil
		}
		return true, nil
	}
	return false, nil

}
