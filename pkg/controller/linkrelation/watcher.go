// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package linkrelation

import (
	"context"
	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-lib-go/pkg/controller"
	"github.com/onosproject/topo-discovery/pkg/store/topo"
	"sync"
)

const queueSize = 100

// TopoWatcher is a topology watcher
type TopoWatcher struct {
	topo   topo.Store
	cancel context.CancelFunc
	mu     sync.Mutex
}

// Start starts the topo store watcher
func (w *TopoWatcher) Start(ch chan<- controller.ID) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		return nil
	}

	eventCh := make(chan topoapi.Event, queueSize)
	ctx, cancel := context.WithCancel(context.Background())

	err := w.topo.Watch(ctx, eventCh, nil)
	if err != nil {
		cancel()
		return err
	}
	w.cancel = cancel
	go func() {
		for event := range eventCh {
			if entity, ok := event.Object.Obj.(*topoapi.Object_Entity); ok {
				if entity.Entity.KindID == topoapi.LinkKind {
					log.Info("Received logical interface entity event")
					if err == nil {
						if err == nil {
							ch <- controller.NewID(event.Object.ID)
						}
					}
				}
			}
			if relation, ok := event.Object.Obj.(*topoapi.Object_Relation); ok {
				if relation.Relation.KindID == topoapi.OriginatesKind {
					targetEntity, err := w.topo.Get(ctx, relation.Relation.TgtEntityID)
					if err != nil {
						log.Warn(err)
					} else if targetEntity.GetEntity().KindID == topoapi.LinkKind {
						ch <- controller.NewID(relation.Relation.TgtEntityID)
					}
				}
			}
			if relation, ok := event.Object.Obj.(*topoapi.Object_Relation); ok {
				if relation.Relation.KindID == topoapi.TerminatesKind {
					targetEntity, err := w.topo.Get(ctx, relation.Relation.SrcEntityID)
					if err != nil {
						log.Warn(err)
					} else if targetEntity.GetEntity().KindID == topoapi.LinkKind {
						ch <- controller.NewID(relation.Relation.SrcEntityID)
					}
				}
			}

		}
	}()
	return nil
}

// Stop stops the topology watcher
func (w *TopoWatcher) Stop() {
	w.mu.Lock()
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	w.mu.Unlock()
}
