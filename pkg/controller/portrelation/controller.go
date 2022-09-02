// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package portrelation

import (
	"context"
	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-lib-go/pkg/controller"
	"github.com/onosproject/onos-lib-go/pkg/errors"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/topo-discovery/pkg/controller/utils"
	"github.com/onosproject/topo-discovery/pkg/store/topo"
	"time"
)

var log = logging.GetLogger()

const (
	defaultTimeout  = 30 * time.Second
	logPortEntityID = "Port entity ID"
)

// NewController returns a new gNMI connection  controller
func NewController(topo topo.Store) *controller.Controller {
	c := controller.NewController("port-relation")
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

// Reconcile reconciles port CONTAIN relations with programmable entity
func (r *Reconciler) Reconcile(id controller.ID) (controller.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	portEntityID := id.Value.(topoapi.ID)
	log.Infow("Reconciling port CONTAIN Relation", logPortEntityID, portEntityID)
	portEntity, err := r.topo.Get(ctx, portEntityID)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Errorw("Failed reconciling port CONTAIN relation", logPortEntityID, portEntityID, "error", err)
			return controller.Result{}, err
		}
		return controller.Result{}, nil
	}

	portAspect := &topoapi.PhyPort{}
	err = portEntity.GetAspect(portAspect)
	if err != nil {
		log.Warnw("Failed reconciling port CONTAIN relation", logPortEntityID, portEntityID, "error", err)
		return controller.Result{}, err
	}

	_, err = r.topo.Get(ctx, topoapi.ID(portAspect.TargetID))
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Warnw("Failed reconciling port CONTAIN relation", logPortEntityID, portEntityID, "error", err)
			return controller.Result{}, err
		}
		if ok, err := r.deletePortEntity(ctx, portEntity); err != nil {
			return controller.Result{}, err
		} else if ok {
			return controller.Result{}, nil
		}
	}

	if ok, err := r.createPortRelation(ctx, topoapi.ID(portAspect.TargetID), portEntityID); err != nil {
		return controller.Result{}, err
	} else if ok {
		return controller.Result{}, nil
	}

	return controller.Result{}, nil
}
func (r *Reconciler) deletePortEntity(ctx context.Context, portEntity *topoapi.Object) (bool, error) {
	err := r.topo.Delete(ctx, portEntity)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Errorw("Failed deleting port entity", logPortEntityID, portEntity.ID, "error", err)
			return false, err
		}
		log.Warnf("Failed deleting port entity", logPortEntityID, portEntity.ID, "error", err)
		return false, nil
	}
	return true, nil
}

func (r *Reconciler) createPortRelation(ctx context.Context, targetID topoapi.ID, portEntityID topoapi.ID) (bool, error) {
	portRelationID := utils.GetContainPortRelationID(targetID, portEntityID)
	log.Infow("Creating contain port relation for target entity", "Port Relation ID", portRelationID)
	_, err := r.topo.Get(ctx, portRelationID)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Warnw("Creating CONTAIN port relation for target entity failed", "Port Relation ID", portRelationID, "error", err)
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
				log.Warnw("Creating CONTAIN port relation for target entity failed", "Port Relation ID", portRelationID, "error", err)
				return false, err
			}
			return false, nil
		}
		return true, nil
	}
	return false, nil
}
