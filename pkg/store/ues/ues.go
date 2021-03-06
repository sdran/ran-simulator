// SPDX-FileCopyrightText: 2020-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package ues

import (
	"context"
	"math/rand"
	"sync"

	"github.com/google/uuid"
	"github.com/onosproject/ran-simulator/pkg/store/watcher"

	"github.com/onosproject/ran-simulator/pkg/store/event"

	"github.com/onosproject/onos-api/go/onos/ransim/types"
	"github.com/onosproject/onos-lib-go/pkg/errors"
	liblog "github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/ran-simulator/pkg/model"
	"github.com/onosproject/ran-simulator/pkg/store/cells"
)

const (
	minIMSI = 1000000
	maxIMSI = 9999999
)

var log = liblog.GetLogger("store", "ues")

// Store tracks inventory of user-equipment for the simulation
type Store interface {
	// SetUECount updates the UE count and creates or deletes new UEs as needed
	SetUECount(ctx context.Context, count uint)

	// Len returns the number of active UEs
	Len(ctx context.Context) int

	// CreateUEs creates the specified number of UEs
	CreateUEs(ctx context.Context, count uint)

	// Get retrieves the UE with the specified IMSI
	Get(ctx context.Context, imsi types.IMSI) (*model.UE, error)

	// Delete destroy the specified UE
	Delete(ctx context.Context, imsi types.IMSI) (*model.UE, error)

	// MoveToCell update the cell affiliation of the specified UE
	MoveToCell(ctx context.Context, imsi types.IMSI, ecgi types.ECGI, strength float64) error

	// MoveToCoordinate updates the UEs geo location and compass heading
	MoveToCoordinate(ctx context.Context, imsi types.IMSI, location model.Coordinate, heading uint32) error

	// ListAllUEs returns an array of all UEs
	ListAllUEs(ctx context.Context) []*model.UE

	// ListUEs returns an array of all UEs associated with the specified cell
	ListUEs(ctx context.Context, ecgi types.ECGI) []*model.UE

	// Watch watches the UE inventory events using the supplied channel
	Watch(ctx context.Context, ch chan<- event.Event, options ...WatchOptions) error
}

// WatchOptions allows tailoring the WatchNodes behaviour
type WatchOptions struct {
	Replay  bool
	Monitor bool
}

type store struct {
	mu        sync.RWMutex
	ues       map[types.IMSI]*model.UE
	cellStore cells.Store
	watchers  *watcher.Watchers
}

// NewUERegistry creates a new user-equipment registry primed with the specified number of UEs to start.
// UEs will be semi-randomly distributed between the specified cells
func NewUERegistry(count uint, cellStore cells.Store) Store {
	log.Infof("Creating registry from model with %d UEs", count)
	watchers := watcher.NewWatchers()
	store := &store{
		mu:        sync.RWMutex{},
		ues:       make(map[types.IMSI]*model.UE),
		cellStore: cellStore,
		watchers:  watchers,
	}
	ctx := context.Background()
	store.CreateUEs(ctx, count)
	log.Infof("Created registry primed with %d UEs", len(store.ues))
	return store
}

func (s *store) SetUECount(ctx context.Context, count uint) {
	delta := len(s.ues) - int(count)
	if delta < 0 {
		s.CreateUEs(ctx, uint(-delta))
	} else if delta > 0 {
		s.removeSomeUEs(ctx, delta)
	}
}

func (s *store) Len(ctx context.Context) int {
	return len(s.ues)
}

func (s *store) removeSomeUEs(ctx context.Context, count int) {
	c := count
	for imsi := range s.ues {
		if c == 0 {
			break
		}
		_, _ = s.Delete(ctx, imsi)
		c = c - 1
	}
}

func (s *store) CreateUEs(ctx context.Context, count uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := uint(0); i < count; i++ {
		imsi := types.IMSI(rand.Int63n(maxIMSI-minIMSI) + minIMSI)
		if _, ok := s.ues[imsi]; ok {
			// FIXME: more robust check for duplicates
			imsi = types.IMSI(rand.Int63n(maxIMSI-minIMSI) + minIMSI)
		}

		randomCell, err := s.cellStore.GetRandomCell()
		if err != nil {
			log.Error(err)
		}
		ecgi := randomCell.ECGI
		ue := &model.UE{
			IMSI:     imsi,
			Type:     "phone",
			Location: model.Coordinate{Lat: 0, Lng: 0},
			Heading:  0,
			Cell: &model.UECell{
				ID:       types.GEnbID(ecgi), // placeholder
				ECGI:     ecgi,
				Strength: rand.Float64() * 100,
			},
			CRNTI:      types.CRNTI(90125 + i),
			Cells:      nil,
			IsAdmitted: false,
		}
		s.ues[ue.IMSI] = ue
	}
}

// Get gets a UE based on a given imsi
func (s *store) Get(ctx context.Context, imsi types.IMSI) (*model.UE, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if node, ok := s.ues[imsi]; ok {
		return node, nil
	}

	return nil, errors.New(errors.NotFound, "UE not found")
}

// Delete deletes a UE based on a given imsi
func (s *store) Delete(ctx context.Context, imsi types.IMSI) (*model.UE, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ue, ok := s.ues[imsi]; ok {
		delete(s.ues, imsi)
		deleteEvent := event.Event{
			Key:   imsi,
			Value: ue,
			Type:  Deleted,
		}
		s.watchers.Send(deleteEvent)
		return ue, nil
	}
	return nil, errors.New(errors.NotFound, "UE not found")
}

func (s *store) ListAllUEs(ctx context.Context) []*model.UE {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]*model.UE, 0, len(s.ues))
	for _, ue := range s.ues {
		list = append(list, ue)
	}
	return list
}

func (s *store) MoveToCell(ctx context.Context, imsi types.IMSI, ecgi types.ECGI, strength float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ue, ok := s.ues[imsi]; ok {
		ue.Cell.ECGI = ecgi
		ue.Cell.Strength = strength
		updateEvent := event.Event{
			Key:   ue.IMSI,
			Value: ue,
			Type:  Updated,
		}
		s.watchers.Send(updateEvent)
		return nil
	}
	return errors.New(errors.NotFound, "UE not found")
}

func (s *store) MoveToCoordinate(ctx context.Context, imsi types.IMSI, location model.Coordinate, heading uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ue, ok := s.ues[imsi]; ok {
		ue.Location = location
		ue.Heading = heading
		updateEvent := event.Event{
			Key:   ue.IMSI,
			Value: ue,
			Type:  Updated,
		}
		s.watchers.Send(updateEvent)
		return nil
	}
	return errors.New(errors.NotFound, "UE not found")
}

func (s *store) ListUEs(ctx context.Context, ecgi types.ECGI) []*model.UE {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]*model.UE, 0, len(s.ues))
	for _, ue := range s.ues {
		if ue.Cell.ECGI == ecgi {
			list = append(list, ue)
		}
	}
	return list
}

func (s *store) Watch(ctx context.Context, ch chan<- event.Event, options ...WatchOptions) error {
	log.Debug("Watching ue changes")
	replay := len(options) > 0 && options[0].Replay

	id := uuid.New()
	err := s.watchers.AddWatcher(id, ch)
	if err != nil {
		log.Error(err)
		close(ch)
		return err
	}
	go func() {
		<-ctx.Done()
		err = s.watchers.RemoveWatcher(id)
		if err != nil {
			log.Error(err)
		}
		close(ch)
	}()

	if replay {
		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, ue := range s.ues {
				ch <- event.Event{
					Key:   ue.IMSI,
					Value: ue,
					Type:  None,
				}
			}
		}()
	}

	return nil
}
