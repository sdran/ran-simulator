// SPDX-FileCopyrightText: 2020-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
//

package honeycomb

import (
	"fmt"
	"github.com/onosproject/onos-api/go/onos/ransim/types"
	"github.com/onosproject/ran-simulator/pkg/model"
	"github.com/onosproject/ran-simulator/pkg/utils"
	"github.com/pmcxs/hexgrid"
	"math"
	"strconv"
	"strings"
)

// GenerateHoneycombTopology generates a set of simulated nodes and cells organized in a honeycomb
// outward from the specified center.
func GenerateHoneycombTopology(mapCenter model.Coordinate, numTowers uint, sectorsPerTower uint, plmnID types.PlmnID,
	enbStart uint32, pitch float32, maxDistance float64, maxNeighbors int,
	controllerAddresses []string, serviceModels []string, singleNode bool) (*model.Model, error) {

	m := &model.Model{
		PlmnID:        plmnID,
		MapLayout:     model.MapLayout{Center: mapCenter, LocationsScale: 1.25},
		Cells:         make(map[string]model.Cell),
		Nodes:         make(map[string]model.Node),
		Controllers:   generateControllers(controllerAddresses),
		ServiceModels: generateServiceModels(serviceModels),
	}

	aspectRatio := utils.AspectRatio(mapCenter.Lat)
	points := hexMesh(float64(pitch), numTowers)
	arc := int32(360.0 / sectorsPerTower)

	controllers := make([]string, 0, len(controllerAddresses))
	for name := range m.Controllers {
		controllers = append(controllers, name)
	}

	models := make([]string, 0, len(serviceModels))
	for name := range m.ServiceModels {
		models = append(models, name)
	}

	var t, s uint
	var enbID types.EnbID
	var nodeName string
	var node model.Node
	for t = 0; t < numTowers; t++ {
		var azOffset int32 = 0
		if sectorsPerTower == 6 {
			azOffset = int32(math.Mod(float64(t), 2) * 30)
		}

		if !singleNode || t == 0 {
			enbID = types.EnbID(enbStart + uint32(t+1))
			nodeName = fmt.Sprintf("node%d", t+1)

			node = model.Node{
				EnbID:         enbID,
				Controllers:   controllers,
				ServiceModels: models,
				Cells:         make([]types.ECGI, 0, sectorsPerTower),
				Status:        "stopped",
			}
		}

		for s = 0; s < sectorsPerTower; s++ {
			cellID := types.CellID(s + 1)
			if singleNode && sectorsPerTower == 1 {
				cellID = types.CellID(t + 1)
			}
			cellName := fmt.Sprintf("cell%d", (t*sectorsPerTower)+s+1)

			azimuth := azOffset
			if s > 0 {
				azimuth = int32(360.0*s/sectorsPerTower + uint(azOffset))
			}

			cell := model.Cell{
				ECGI: types.ToECGI(plmnID, types.ToECI(enbID, cellID)),
				Sector: model.Sector{
					Center: model.Coordinate{
						Lat: mapCenter.Lat + points[t].Lat,
						Lng: mapCenter.Lng + points[t].Lng/aspectRatio},
					Azimuth: azimuth,
					Arc:     arc},
				Color:     "green",
				MaxUEs:    99999,
				Neighbors: make([]types.ECGI, 0, sectorsPerTower),
				TxPowerDB: 11,
			}

			m.Cells[cellName] = cell
			node.Cells = append(node.Cells, cell.ECGI)
		}

		m.Nodes[nodeName] = node
	}

	// Add cells neighbors
	for cellName, cell := range m.Cells {
		for _, other := range m.Cells {
			if cell.ECGI != other.ECGI && isNeighbor(cell, other, maxDistance, sectorsPerTower == 1) && len(cell.Neighbors) < maxNeighbors {
				cell.Neighbors = append(cell.Neighbors, other.ECGI)
			}
		}
		m.Cells[cellName] = cell
	}

	return m, nil
}

func generateControllers(addresses []string) map[string]model.Controller {
	controllers := make(map[string]model.Controller)
	for i, address := range addresses {
		name := fmt.Sprintf("e2t-%d", i+1)
		controllers[name] = model.Controller{ID: name, Address: address, Port: 36421}
	}
	return controllers
}

func generateServiceModels(namesAndIDs []string) map[string]model.ServiceModel {
	models := make(map[string]model.ServiceModel)
	for i, nameAndID := range namesAndIDs {
		fields := strings.Split(nameAndID, "/")
		id := int64(i)
		if len(fields) > 1 {
			id, _ = strconv.ParseInt(fields[1], 10, 32)
		}
		models[fields[0]] = model.ServiceModel{ID: int(id), Version: "1.0.0", Description: fields[0] + " service model"}
	}
	return models
}

// Cells are neighbors if their sectors have the same coordinates or if their center arc vectors fall within a distance/2
func isNeighbor(cell model.Cell, other model.Cell, maxDistance float64, onlyDistance bool) bool {
	return (cell.Sector.Center.Lat == other.Sector.Center.Lat && cell.Sector.Center.Lng == other.Sector.Center.Lng) ||
		(onlyDistance && distance(cell.Sector.Center, other.Sector.Center) <= maxDistance) ||
		distance(reachPoint(cell.Sector, maxDistance), reachPoint(other.Sector, maxDistance)) <= maxDistance/2
}

// Calculate the end-point of the center arc vector a distance from the sector center
func reachPoint(sector model.Sector, distance float64) model.Coordinate {
	return targetPoint(sector.Center, float64((sector.Azimuth+sector.Arc/2)%360), distance)
}

// Earth radius in meters
const earthRadius = 6378100

// http://en.wikipedia.org/wiki/Haversine_formula
func distance(c1 model.Coordinate, c2 model.Coordinate) float64 {
	var la1, lo1, la2, lo2 float64
	la1 = c1.Lat * math.Pi / 180
	lo1 = c1.Lng * math.Pi / 180
	la2 = c2.Lat * math.Pi / 180
	lo2 = c2.Lng * math.Pi / 180

	h := hsin(la2-la1) + math.Cos(la1)*math.Cos(la2)*hsin(lo2-lo1)

	return 2 * earthRadius * math.Asin(math.Sqrt(h))
}

func targetPoint(c model.Coordinate, azimuth float64, dist float64) model.Coordinate {
	var la1, lo1, la2, lo2, d float64
	la1 = c.Lat * math.Pi / 180
	lo1 = c.Lng * math.Pi / 180
	d = dist / earthRadius

	la2 = math.Asin(math.Sin(la1)*math.Cos(d) + math.Cos(la1)*math.Sin(d)*math.Cos(azimuth))
	lo2 = lo1 + math.Atan2(math.Sin(azimuth)*math.Sin(d)*math.Cos(la1), math.Cos(d)-math.Sin(la1)*math.Sin(la2))

	tp := model.Coordinate{Lat: la2 * 180 / math.Pi, Lng: lo2 * 180 / math.Pi}

	//fmt.Printf("s: %.4f,%.4f\tt: %.4f,%.4f\ta: %.2f\td: %.2f\tcd: %.2f\n",
	//	c.Lng, c.Lat, tp.Lng, tp.Lat, azimuth, dist, distance(c, tp))

	return tp
}

func hsin(theta float64) float64 {
	return math.Pow(math.Sin(theta/2), 2)
}

func hexMesh(pitch float64, numTowers uint) []*model.Coordinate {
	rings, _ := numRings(numTowers)
	points := make([]*model.Coordinate, 0)
	hexArray := hexgrid.HexRange(hexgrid.NewHex(0, 0), int(rings))

	for _, h := range hexArray {
		x, y := hexgrid.Point(hexgrid.HexToPixel(hexgrid.LayoutPointY00(pitch, pitch), h))
		points = append(points, &model.Coordinate{Lat: x, Lng: y})
	}
	return points
}

// Number of cells in the hexagon layout 3x^2+9x+7
func numRings(numTowers uint) (uint, error) {
	switch n := numTowers; {
	case n <= 7:
		return 1, nil
	case n <= 19:
		return 2, nil
	case n <= 37:
		return 3, nil
	case n <= 61:
		return 4, nil
	case n <= 91:
		return 5, nil
	case n <= 127:
		return 6, nil
	case n <= 169:
		return 7, nil
	case n <= 217:
		return 8, nil
	case n <= 271:
		return 9, nil
	case n <= 331:
		return 10, nil
	case n <= 469:
		return 11, nil
	default:
		return 0, fmt.Errorf(">469 not handled %d", numTowers)
	}

}
