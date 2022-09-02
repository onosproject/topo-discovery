// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package port

import (
	"context"
	"encoding/json"
	gogotypes "github.com/gogo/protobuf/types"
	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-lib-go/pkg/certs"
	"github.com/onosproject/onos-lib-go/pkg/controller"
	"github.com/onosproject/onos-lib-go/pkg/errors"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/topo-discovery/pkg/controller/utils"
	"github.com/onosproject/topo-discovery/pkg/store/topo"
	"github.com/openconfig/gnmi/proto/gnmi"

	"google.golang.org/grpc"
	"time"
)

var log = logging.GetLogger()

const (
	defaultTimeout  = 30 * time.Second
	logPortEntityID = "Port entity ID"
	logTargetID     = "TargetID"
	interfacesPath  = "openconfig-interfaces:interfaces"
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

func (r *Reconciler) extractPorts(notification []*gnmi.Notification, targetID topoapi.ID) ([]*topoapi.PhyPort, error) {
	var ports []*topoapi.PhyPort
	notificationMap := make(map[string]interface{})

	err := json.Unmarshal(notification[0].Update[0].Val.GetJsonIetfVal(), &notificationMap)
	if err != nil {
		return nil, err
	}

	interfacesMap := notificationMap[interfacesPath].(map[string]interface{})
	interfaces := interfacesMap["interface"].([]interface{})
	for _, v := range interfaces {
		mapValue := v.(map[string]interface{})
		port := &topoapi.PhyPort{}
		for key, value := range mapValue {
			if key == "name" {
				port.DisplayName = value.(string)
				port.TargetID = string(targetID)
			}
		}
		ports = append(ports, port)
	}

	return ports, nil
}

// Reconcile reconciles port entities for a programmable target
func (r *Reconciler) Reconcile(id controller.ID) (controller.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	targetID := id.Value.(topoapi.ID)
	log.Infow("Reconciling Ports for target", logTargetID, targetID)
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
		Encoding: gnmi.Encoding_JSON,
		Path:     paths,
	}

	getResponse, err := gnmiClient.Get(ctx, gnmiGetReq)
	if err != nil {
		log.Warnw("Failed reconciling Ports for Target", logTargetID, targetID, "error", err)
		return controller.Result{}, err
	}

	ports, err := r.extractPorts(getResponse.Notification, targetID)
	if err != nil {
		return controller.Result{}, err
	}

	if ok, err := r.createPortEntities(ctx, ports); err != nil {
		return controller.Result{}, err
	} else if ok {
		return controller.Result{}, nil
	}
	return controller.Result{}, nil
}

func (r *Reconciler) createPortEntities(ctx context.Context, ports []*topoapi.PhyPort) (bool, error) {
	for _, port := range ports {
		if _, err := r.createPortEntity(ctx, port); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (r *Reconciler) createPortEntity(ctx context.Context, port *topoapi.PhyPort) (bool, error) {
	targetID := port.TargetID
	portEntityID := utils.GetPortID(targetID, port.DisplayName)
	log.Infow("Creating port entity", logTargetID, targetID, logTargetID, portEntityID)
	object, err := r.topo.Get(ctx, portEntityID)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Warnw("Creating port entity failed", logTargetID, targetID, logPortEntityID, portEntityID)
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
				log.Warnw("Creating port entity failed", logTargetID, targetID, logPortEntityID, portEntityID, "error", err)
				return false, err
			}
			return false, nil
		}
		return true, nil
	}

	portAspect := &topoapi.PhyPort{}
	err = object.GetAspect(portAspect)
	if err == nil {
		log.Debugf("Port aspect is already set", portAspect)
		return false, nil
	}

	log.Debugw("Updating port aspect", logTargetID, targetID, logPortEntityID, portEntityID)
	err = object.SetAspect(port)
	if err != nil {
		log.Warnw("Updating port aspect failed", logTargetID, targetID, logPortEntityID, portEntityID, "error", err)
		return false, err
	}
	err = r.topo.Update(ctx, object)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Warnw("Updating port entity failed", logTargetID, targetID, logPortEntityID, portEntityID, "error", err)
			return false, err
		}
		return false, nil
	}
	return true, nil

}
