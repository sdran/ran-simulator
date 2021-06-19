// SPDX-FileCopyrightText: 2020-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: LicenseRef-ONF-Member-1.0

package mobility

import (
	"context"
	"github.com/onosproject/onos-api/go/onos/ransim/types"
	e2sm_mho "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_mho/v1/e2sm-mho"
	"github.com/onosproject/ran-simulator/pkg/model"
	"math/rand"
)

// RrcCtrl is the RRC controller
type RrcCtrl struct {
	RrcUpdateChan chan model.UE
}

// NewRrcCtrl returns a new RRC Controller
func (d *driver) NewRrcCtrl() *RrcCtrl {
	return &RrcCtrl{}
}

func (d *driver) updateRrc(ctx context.Context, imsi types.IMSI, probabilityOfRrcStateChange float64) {
	var randomBoolean = rand.Float64() < probabilityOfRrcStateChange

	if randomBoolean {
		ue, err := d.ueStore.Get(ctx, imsi)
		if err != nil {
			log.Warn("Unable to find UE %d", imsi)
			return
		}

		if ue.RrcState == e2sm_mho.Rrcstatus_RRCSTATUS_IDLE {
			log.Debugf("RRC state change imsi:%d from IDLE to CONNECTED", imsi)
			ue.RrcState = e2sm_mho.Rrcstatus_RRCSTATUS_CONNECTED
		} else if ue.RrcState == e2sm_mho.Rrcstatus_RRCSTATUS_CONNECTED {
			log.Debugf("RRC state change imsi:%d from CONNECTED to IDLE", imsi)
			ue.RrcState = e2sm_mho.Rrcstatus_RRCSTATUS_IDLE
		} else {
			log.Warnf("Invalid RrcState %v for imsi %d", ue.RrcState, imsi)
			return
		}

		d.rrcCtrl.RrcUpdateChan <- *ue

	}
}
