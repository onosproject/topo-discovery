// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package phyinterfacerelation

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
	defaultTimeout                = 30 * time.Second
	logInterfaceEntityID          = "phy interface entity ID"
	logInterfaceContainRelationID = "phy interface CONTAIN Relation ID"
)

// NewController returns a new phy interface relation controller
func NewController(topo topo.Store) *controller.Controller {
	c := controller.NewController("phy-interface-relation")
	c.Watch(&TopoWatcher{
		topo: topo,
	})
	c.Reconcile(&Reconciler{
		topo: topo,
	})
	return c
}

// Reconciler reconciles phy interface entity CONTAIN relation
type Reconciler struct {
	topo topo.Store
}

// Reconcile reconciles phy interface CONTAIN relations with programmable entity
func (r *Reconciler) Reconcile(id controller.ID) (controller.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	phyInterfaceEntityID := id.Value.(topoapi.ID)
	log.Infow("Reconciling phy interface CONTAIN Relation", logInterfaceEntityID, phyInterfaceEntityID)
	phyInterfaceEntity, err := r.topo.Get(ctx, phyInterfaceEntityID)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Errorw("Failed reconciling phy interface CONTAIN relation", logInterfaceEntityID, phyInterfaceEntityID, "error", err)

			return controller.Result{}, err
		}
		return controller.Result{}, nil
	}

	phyInterfaceAspect := &topoapi.PhyInterface{}
	err = phyInterfaceEntity.GetAspect(phyInterfaceAspect)
	if err != nil {
		log.Warnw("Failed reconciling phy interface CONTAIN relation", logInterfaceEntityID, phyInterfaceEntityID, "error", err)
		return controller.Result{}, err
	}

	_, err = r.topo.Get(ctx, topoapi.ID(phyInterfaceAspect.TargetID))
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Warnw("Failed reconciling phy interface CONTAIN relation", logInterfaceEntityID, phyInterfaceEntityID, "error", err)
			return controller.Result{}, err
		}
		if ok, err := r.deletePhyInterfaceEntity(ctx, phyInterfaceEntity); err != nil {
			return controller.Result{}, err
		} else if ok {
			return controller.Result{}, nil
		}
	}

	if ok, err := r.createPhyInterfaceContainRelation(ctx, topoapi.ID(phyInterfaceAspect.TargetID), phyInterfaceEntityID); err != nil {
		return controller.Result{}, err
	} else if ok {
		return controller.Result{}, nil
	}

	return controller.Result{}, nil
}
func (r *Reconciler) deletePhyInterfaceEntity(ctx context.Context, phyInterfaceEntity *topoapi.Object) (bool, error) {
	err := r.topo.Delete(ctx, phyInterfaceEntity)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Errorw("Failed deleting phy interface entity", logInterfaceEntityID, phyInterfaceEntity.ID, "error", err)
			return false, err
		}
		log.Warnf("Failed deleting phy interface  entity", logInterfaceEntityID, phyInterfaceEntity.ID, "error", err)

		return false, nil
	}
	return true, nil
}

func (r *Reconciler) createPhyInterfaceContainRelation(ctx context.Context, targetID topoapi.ID, phyInterfaceEntityID topoapi.ID) (bool, error) {
	phyInterfaceRelationID := utils.GetContainPhyInterfaceRelationID(targetID, phyInterfaceEntityID)
	log.Infow("Creating contain phy interface relation for target entity", logInterfaceContainRelationID, phyInterfaceRelationID)
	_, err := r.topo.Get(ctx, phyInterfaceRelationID)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Warnw("Creating phy interface CONTAIN  relation for target entity failed", logInterfaceContainRelationID, phyInterfaceRelationID, "error", err)

			return false, err
		}
		object := &topoapi.Object{
			ID:   phyInterfaceRelationID,
			Type: topoapi.Object_RELATION,
			Obj: &topoapi.Object_Relation{
				Relation: &topoapi.Relation{
					KindID:      topoapi.CONTAINS,
					SrcEntityID: targetID,
					TgtEntityID: phyInterfaceEntityID,
				},
			},
		}

		err := r.topo.Create(ctx, object)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				log.Warnw("Creating phy interface  CONTAIN  relation for target entity failed", logInterfaceContainRelationID, phyInterfaceRelationID, "error", err)
				return false, err
			}
			return false, nil
		}
		return true, nil
	}
	return false, nil
}
