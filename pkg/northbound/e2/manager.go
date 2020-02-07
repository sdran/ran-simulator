// Copyright 2020-present Open Networking Foundation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2

import (
	"fmt"
	"github.com/onosproject/ran-simulator/api/trafficsim"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/onosproject/ran-simulator/api/e2"
	"github.com/onosproject/ran-simulator/api/types"
	"github.com/onosproject/ran-simulator/pkg/manager"
	"github.com/prometheus/common/log"
)

// TestPlmnID - https://en.wikipedia.org/wiki/Mobile_country_code#Test_networks
const TestPlmnID = "001001"
const e2Manager = "e2Manager"

var mgr Manager

// Manager single point of entry for the trafficsim system.
type Manager struct {
}

// NewManager ...
func NewManager() (*Manager, error) {
	return &Manager{}, nil
}

// Run ...
func (m *Manager) Run(towerParams types.TowersParams) error {
	trafficSimMgr := manager.GetManager()
	for _, tower := range trafficSimMgr.Towers {
		tower.PlmnID = TestPlmnID
		tower.EcID = makeEci(tower.Name)
		tower.MaxUEs = towerParams.MaxUEs
		tower.Neighbors = makeNeighbors(tower.Name, towerParams)
		log.Infof("Neighbors of %s - %s", tower.Name, strings.Join(tower.Neighbors, ", "))
	}
	for _, ue := range trafficSimMgr.UserEquipments {
		ue.Crnti = makeCrnti(ue.Name)
	}
	ueChangeChannel, err := trafficSimMgr.Dispatcher.RegisterUeListener(e2Manager)
	if err != nil {
		return err
	}
	go func() {
		for ueUpdate := range ueChangeChannel {
			if ueUpdate.Type == trafficsim.Type_UPDATED && ueUpdate.UpdateType == trafficsim.UpdateType_TOWER {
				ue, ok := ueUpdate.Object.(*types.Ue)
				if !ok {
					log.Fatalf("Object %v could not be converted to UE", ueUpdate)
				}
				log.Infof("UE %s changed. Serving: %s (%f), 2nd: %s (%f), 3rd: %s (%f).",
					ue.Name, ue.Tower, ue.TowerDist, ue.Tower2, ue.Tower2Dist, ue.Tower3, ue.Tower3Dist)
			}
		}
	}()
	return nil
}

//Close kills the channels and manager related objects
func (m *Manager) Close() {
	manager.GetManager().Dispatcher.UnregisterUeListener(e2Manager)
	log.Info("Closing Manager")
}

// GetManager returns the initialized and running instance of manager.
// Should be called only after NewManager and Run are done.
func GetManager() *Manager {
	return &mgr
}

// Min ...
func Min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

// Max ...
func Max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

func makeNeighbors(towerName string, towerParams types.TowersParams) []string {
	neighbors := make([]string, 0, 8)
	re := regexp.MustCompile("[0-9]+")
	id, _ := strconv.Atoi(re.FindAllString(towerName, 1)[0])
	id--

	nrows := int(towerParams.TowerRows)
	ncols := int(towerParams.TowerCols)

	i := id / nrows
	j := id % ncols

	for x := Max(0, i-1); x <= Min(i+1, nrows-1); x++ {
		for y := Max(0, j-1); y <= Min(j+1, ncols-1); y++ {
			if (x == i && y == j-1) || (x == i && y == j+1) || (x == i-1 && y == j) || (x == i+1 && y == j) {
				towerNum := x*nrows + y + 1
				towerName := fmt.Sprintf("Tower-%d", towerNum)
				neighbors = append(neighbors, towerName)
			}
		}
	}
	return neighbors
}

func makeEci(towerName string) string {
	re := regexp.MustCompile("[0-9]+")
	id, _ := strconv.Atoi(re.FindAllString(towerName, 1)[0])
	return fmt.Sprintf("%07X", id)
}

func makeCrnti(ueName string) string {
	re := regexp.MustCompile("[0-9]+")
	id, _ := strconv.Atoi(re.FindAllString(ueName, 1)[0])
	return fmt.Sprintf("%04X", id+1)
}

func (m *Manager) runControl(stream e2.InterfaceService_SendControlServer) error {
	c := make(chan e2.ControlUpdate)
	go mgr.recvLoop(stream, c)
	return mgr.sendLoop(stream, c)
}

func (m *Manager) sendLoop(stream e2.InterfaceService_SendControlServer, c chan e2.ControlUpdate) error {
	for {
		select {
		case msg := <-c:
			if err := stream.Send(&msg); err != nil {
				log.Infof("send error %v", err)
				return err
			}
		case <-stream.Context().Done():
			log.Infof("Controller has disconnected")
			return nil
		}
	}
}

func (m *Manager) recvLoop(stream e2.InterfaceService_SendControlServer, c chan e2.ControlUpdate) {
	for {
		in, err := stream.Recv()
		if err == io.EOF || err != nil {
			return
		}
		log.Infof("Recv messageType %d", in.MessageType)
		switch x := in.S.(type) {
		case *e2.ControlResponse_CellConfigRequest:
			mgr.handleCellConfigRequest(stream, x.CellConfigRequest, c)
		default:
			log.Errorf("ControlResponse has unexpected type %T", x)
		}
	}
}

func (m *Manager) handleCellConfigRequest(stream e2.InterfaceService_SendControlServer, req *e2.CellConfigRequest, c chan e2.ControlUpdate) {
	log.Infof("handleCellConfigRequest")

	trafficSimMgr := manager.GetManager()

	for _, tower := range trafficSimMgr.Towers {
		cells := make([]*e2.CandScell, 0, 8)
		for _, neighbor := range tower.Neighbors {
			t := trafficSimMgr.Towers[neighbor]
			cell := e2.CandScell{
				Ecgi: &e2.ECGI{
					PlmnId: t.PlmnID,
					Ecid:   t.EcID,
				}}
			cells = append(cells, &cell)
		}
		cellConfigReport := e2.ControlUpdate{
			MessageType: e2.MessageType_CELL_CONFIG_REPORT,
			S: &e2.ControlUpdate_CellConfigReport{
				CellConfigReport: &e2.CellConfigReport{
					Ecgi: &e2.ECGI{
						PlmnId: tower.PlmnID,
						Ecid:   tower.EcID,
					},
					MaxNumConnectedUes: tower.MaxUEs,
					CandScells:         cells,
				},
			},
		}

		c <- cellConfigReport
		log.Infof("handleCellConfigReport eci: %s", tower.EcID)
	}

	// Initate UE admissions
	for _, ue := range trafficSimMgr.UserEquipments {
		eci := trafficSimMgr.GetTowerByName(ue.Tower).EcID
		ueAdmReq := e2.ControlUpdate{
			MessageType: e2.MessageType_UE_ADMISSION_REQUEST,
			S: &e2.ControlUpdate_UEAdmissionRequest{
				UEAdmissionRequest: &e2.UEAdmissionRequest{
					Ecgi: &e2.ECGI{
						PlmnId: TestPlmnID,
						Ecid:   eci,
					},
					Crnti:             ue.Crnti,
					AdmissionEstCause: e2.AdmEstCause_MO_SIGNALLING,
				},
			},
		}
		c <- ueAdmReq
		log.Infof("ueAdmissionRequest eci:%s crnti:%s", eci, ue.Crnti)
	}
}
