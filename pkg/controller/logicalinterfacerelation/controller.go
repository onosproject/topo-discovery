// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package logicalinterfacerelation

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
	defaultTimeout              = 30 * time.Second
	logLogicalInterfaceEntityID = "logical interface entity ID"
)

// NewController returns a new gNMI connection  controller
func NewController(topo topo.Store) *controller.Controller {
	c := controller.NewController("logical-interface-relation")
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

// Reconcile reconciles logical interface CONTAIN relations with programmable entity
func (r *Reconciler) Reconcile(id controller.ID) (controller.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	logicalInterfaceEntityID := id.Value.(topoapi.ID)
	log.Infow("Reconciling logical interface CONTAIN Relation", logLogicalInterfaceEntityID, logicalInterfaceEntityID)
	logicalInterfaceEntity, err := r.topo.Get(ctx, logicalInterfaceEntityID)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Errorw("Failed reconciling logical interface CONTAIN relation", logLogicalInterfaceEntityID, logicalInterfaceEntityID, "error", err)

			return controller.Result{}, err
		}
		return controller.Result{}, nil
	}

	logicalInterfaceAspect := &topoapi.LogicalInterface{}
	err = logicalInterfaceEntity.GetAspect(logicalInterfaceAspect)
	if err != nil {
		log.Warnw("Failed reconciling logical interface CONTAIN relation", logLogicalInterfaceEntityID, logicalInterfaceEntityID, "error", err)
		return controller.Result{}, err
	}

	_, err = r.topo.Get(ctx, topoapi.ID(logicalInterfaceAspect.TargetID))
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Warnw("Failed reconciling logical interface CONTAIN relation", logLogicalInterfaceEntityID, logicalInterfaceEntityID, "error", err)
			return controller.Result{}, err
		}
		if ok, err := r.deleteLogicalInterfaceEntity(ctx, logicalInterfaceEntity); err != nil {
			return controller.Result{}, err
		} else if ok {
			return controller.Result{}, nil
		}
	}

	if ok, err := r.createLogicalInterfaceContainRelation(ctx, topoapi.ID(logicalInterfaceAspect.TargetID), logicalInterfaceEntityID); err != nil {
		return controller.Result{}, err
	} else if ok {
		return controller.Result{}, nil
	}

	return controller.Result{}, nil
}
func (r *Reconciler) deleteLogicalInterfaceEntity(ctx context.Context, logicalInterfaceEntity *topoapi.Object) (bool, error) {
	err := r.topo.Delete(ctx, logicalInterfaceEntity)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Errorw("Failed deleting logical interface entity", logLogicalInterfaceEntityID, logicalInterfaceEntity.ID, "error", err)
			return false, err
		}
		log.Warnf("Failed deleting logical interface  entity", logLogicalInterfaceEntityID, logicalInterfaceEntity.ID, "error", err)

		return false, nil
	}
	return true, nil
}

func (r *Reconciler) createLogicalInterfaceContainRelation(ctx context.Context, targetID topoapi.ID, logicalInterfaceEntityID topoapi.ID) (bool, error) {
	logicalInterfaceRelationID := utils.GetContainLogicalInterfaceRelationID(targetID, logLogicalInterfaceEntityID)
	log.Infow("Creating contain logical interface relation for target entity", "logical interface CONTAIN Relation ID", logicalInterfaceRelationID)
	_, err := r.topo.Get(ctx, logicalInterfaceRelationID)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Warnw("Creating logical interface CONTAIN  relation for target entity failed", "logical interface CONTAIN Relation ID", logicalInterfaceRelationID, "error", err)

			return false, err
		}
		object := &topoapi.Object{
			ID:   logicalInterfaceRelationID,
			Type: topoapi.Object_RELATION,
			Obj: &topoapi.Object_Relation{
				Relation: &topoapi.Relation{
					KindID:      topoapi.CONTAINS,
					SrcEntityID: targetID,
					TgtEntityID: logicalInterfaceEntityID,
				},
			},
		}

		err := r.topo.Create(ctx, object)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				log.Warnw("Creating logical interface  CONTAIN  relation for target entity failed", "logical interface CONTAIN Relation ID", logicalInterfaceRelationID, "error", err)
				return false, err
			}
			return false, nil
		}
		return true, nil
	}
	return false, nil
}
