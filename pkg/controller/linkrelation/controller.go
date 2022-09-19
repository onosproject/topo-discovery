// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package linkrelation

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
	defaultTimeout          = 30 * time.Second
	logLinkEntityID         = "link entity ID"
	logOriginatesRelationID = "ORIGINATES relation ID"
	logTerminatesRelationID = "TERMINATES relation ID"
)

// NewController returns a new gNMI connection  controller
func NewController(topo topo.Store) *controller.Controller {
	c := controller.NewController("link-entity-relations")
	c.Watch(&TopoWatcher{
		topo: topo,
	})
	c.Reconcile(&Reconciler{
		topo: topo,
	})
	return c
}

// Reconciler reconciles a link originates and terminates relations
type Reconciler struct {
	topo topo.Store
}

// Reconcile reconciles a link ORIGINATES and TERMINATES relations
func (r *Reconciler) Reconcile(id controller.ID) (controller.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	linkEntityID := id.Value.(topoapi.ID)
	log.Infow("Reconciling  Link entity Relations", logLinkEntityID, linkEntityID)
	_, err := r.topo.Get(ctx, linkEntityID)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Errorw("Failed reconciling  Link entity Relations", logLinkEntityID, linkEntityID, "error", err)

			return controller.Result{}, err
		}
		return controller.Result{}, nil
	}

	sourceInterfaceID, destInterfaceID, err := utils.GetInterfaceIDsFromLinkID(linkEntityID)
	if err != nil {
		return controller.Result{}, err
	}

	if ok, err := r.createLinkOriginatesRelation(ctx, sourceInterfaceID, linkEntityID); err != nil {
		return controller.Result{}, err
	} else if ok {
		return controller.Result{}, nil
	}
	if ok, err := r.createLinkTerminatesRelation(ctx, linkEntityID, destInterfaceID); err != nil {
		return controller.Result{}, err
	} else if ok {
		return controller.Result{}, nil
	}

	return controller.Result{}, nil
}

func (r *Reconciler) createLinkOriginatesRelation(ctx context.Context, sourceInterfaceID topoapi.ID, linkEntityID topoapi.ID) (bool, error) {
	originatesRelationID := utils.GetLinkOriginatesRelationID(sourceInterfaceID, linkEntityID)
	log.Infow("Creating ORIGINATES relation for link entity", logOriginatesRelationID, originatesRelationID)
	_, err := r.topo.Get(ctx, originatesRelationID)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Warnw("Creating ORIGINATES relation for link entity failed", logOriginatesRelationID, originatesRelationID, "error", err)

			return false, err
		}
		object := &topoapi.Object{
			ID:   originatesRelationID,
			Type: topoapi.Object_RELATION,
			Obj: &topoapi.Object_Relation{
				Relation: &topoapi.Relation{
					KindID:      topoapi.OriginatesKind,
					SrcEntityID: sourceInterfaceID,
					TgtEntityID: linkEntityID,
				},
			},
		}

		err := r.topo.Create(ctx, object)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				log.Warnw("Creating ORIGINATES relation for link entity failed", logOriginatesRelationID, originatesRelationID, "error", err)
				return false, err
			}
			return false, nil
		}
		return true, nil
	}
	return false, nil
}

func (r *Reconciler) createLinkTerminatesRelation(ctx context.Context, linkEntityID topoapi.ID, destInterfaceID topoapi.ID) (bool, error) {
	terminatesRelationID := utils.GetLinkTerminatesRelationID(linkEntityID, destInterfaceID)
	log.Infow("Creating TERMINATES relation for link entity", logTerminatesRelationID, terminatesRelationID)
	_, err := r.topo.Get(ctx, terminatesRelationID)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Warnw("Creating TERMINATES relation for link entity failed", logTerminatesRelationID, terminatesRelationID, "error", err)

			return false, err
		}
		object := &topoapi.Object{
			ID:   terminatesRelationID,
			Type: topoapi.Object_RELATION,
			Obj: &topoapi.Object_Relation{
				Relation: &topoapi.Relation{
					KindID:      topoapi.TerminatesKind,
					SrcEntityID: linkEntityID,
					TgtEntityID: destInterfaceID,
				},
			},
		}

		err := r.topo.Create(ctx, object)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				log.Warnw("Creating Terminates relation for link entity failed", logTerminatesRelationID, terminatesRelationID, "error", err)
				return false, err
			}
			return false, nil
		}
		return true, nil
	}
	return false, nil
}
