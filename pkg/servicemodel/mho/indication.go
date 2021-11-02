// SPDX-FileCopyrightText: 2020-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package mho

import (
	"context"
	"encoding/binary"
	subutils "github.com/onosproject/ran-simulator/pkg/utils/e2ap/subscription"
	"strconv"
	"time"

	e2smtypes "github.com/onosproject/onos-api/go/onos/e2t/e2sm"
	ransimtypes "github.com/onosproject/onos-api/go/onos/ransim/types"
	e2sm_mho "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_mho/v1/e2sm-mho"
	"github.com/onosproject/ran-simulator/pkg/model"
	"github.com/onosproject/ran-simulator/pkg/store/subscriptions"
	e2apIndicationUtils "github.com/onosproject/ran-simulator/pkg/utils/e2ap/indication"
	indHdr "github.com/onosproject/ran-simulator/pkg/utils/e2sm/mho/indication/header"
	indMsgFmt1 "github.com/onosproject/ran-simulator/pkg/utils/e2sm/mho/indication/message_format1"
	indMsgFmt2 "github.com/onosproject/ran-simulator/pkg/utils/e2sm/mho/indication/message_format2"
)

func (m *Mho) sendRicIndication(ctx context.Context, subscription *subutils.Subscription) error {
	node := m.ServiceModel.Node
	// Creates and sends an indication message for each cell in the node
	for _, ncgi := range node.Cells {
		log.Debugf("Send MHO indications for cell ncgi:%d", ncgi)
		for _, ue := range m.ServiceModel.UEs.ListUEs(ctx, ncgi) {
			// Ignore idle UEs
			if ue.RrcState == e2sm_mho.Rrcstatus_RRCSTATUS_IDLE {
				continue
			}
			log.Debugf("Send MHO indications for cell ncgi:%d, IMSI:%d", ncgi, ue.IMSI)
			err := m.sendRicIndicationFormat1(ctx, ncgi, ue, subscription)
			if err != nil {
				log.Warn(err)
				continue
			}
		}
	}
	return nil
}

func (m *Mho) sendRicIndicationFormat1(ctx context.Context, ncgi ransimtypes.NCGI, ue *model.UE, subscription *subutils.Subscription) error {
	subID := subscriptions.NewID(subscription.GetRicInstanceID(), subscription.GetReqID(), subscription.GetRanFuncID())
	sub, err := m.ServiceModel.Subscriptions.Get(subID)
	if err != nil {
		return err
	}

	indicationHeaderBytes, err := m.createIndicationHeaderBytes(ctx, ncgi)
	if err != nil {
		return err
	}

	indicationMessageBytes, err := m.createIndicationMsgFormat1(ue)
	if err != nil {
		return err
	}
	if indicationMessageBytes == nil {
		return nil
	}

	indication := e2apIndicationUtils.NewIndication(
		e2apIndicationUtils.WithRicInstanceID(subscription.GetRicInstanceID()),
		e2apIndicationUtils.WithRanFuncID(subscription.GetRanFuncID()),
		e2apIndicationUtils.WithRequestID(subscription.GetReqID()),
		e2apIndicationUtils.WithIndicationHeader(indicationHeaderBytes),
		e2apIndicationUtils.WithIndicationMessage(indicationMessageBytes))

	ricIndication, err := indication.Build()
	if err != nil {
		return err
	}

	err = sub.E2Channel.RICIndication(ctx, ricIndication)
	if err != nil {
		return err
	}

	return nil
}

func (m *Mho) sendRicIndicationFormat2(ctx context.Context, ncgi ransimtypes.NCGI, ue *model.UE, subscription *subutils.Subscription) error {
	subID := subscriptions.NewID(subscription.GetRicInstanceID(), subscription.GetReqID(), subscription.GetRanFuncID())
	sub, err := m.ServiceModel.Subscriptions.Get(subID)
	if err != nil {
		return err
	}

	indicationHeaderBytes, err := m.createIndicationHeaderBytes(ctx, ncgi)
	if err != nil {
		return err
	}

	indicationMessageBytes, err := m.createIndicationMsgFormat2(ue)
	if err != nil {
		return err
	}
	if indicationMessageBytes == nil {
		return nil
	}

	indication := e2apIndicationUtils.NewIndication(
		e2apIndicationUtils.WithRicInstanceID(subscription.GetRicInstanceID()),
		e2apIndicationUtils.WithRanFuncID(subscription.GetRanFuncID()),
		e2apIndicationUtils.WithRequestID(subscription.GetReqID()),
		e2apIndicationUtils.WithIndicationHeader(indicationHeaderBytes),
		e2apIndicationUtils.WithIndicationMessage(indicationMessageBytes))

	ricIndication, err := indication.Build()
	if err != nil {
		return err
	}

	err = sub.E2Channel.RICIndication(ctx, ricIndication)
	if err != nil {
		return err
	}

	return nil
}

func (m *Mho) createIndicationHeaderBytes(ctx context.Context, ncgi ransimtypes.NCGI) ([]byte, error) {

	cell, _ := m.ServiceModel.CellStore.Get(ctx, ncgi)
	plmnID := ransimtypes.NewUint24(uint32(m.ServiceModel.Model.PlmnID))
	timestamp := make([]byte, 4)
	binary.BigEndian.PutUint32(timestamp, uint32(time.Now().Unix()))
	header := indHdr.NewIndicationHeader(
		indHdr.WithPlmnID(*plmnID),
		indHdr.WithNrcellIdentity(uint64(ransimtypes.GetNCI(cell.NCGI))))

	mhoModelPlugin, err := m.ServiceModel.ModelPluginRegistry.GetPlugin(e2smtypes.OID(m.ServiceModel.OID))
	if err != nil {
		return nil, err
	}

	indicationHeaderAsn1Bytes, err := header.MhoToAsn1Bytes(mhoModelPlugin)
	if err != nil {
		return nil, err
	}

	return indicationHeaderAsn1Bytes, nil
}

func (m *Mho) createIndicationMsgFormat1(ue *model.UE) ([]byte, error) {
	log.Debugf("Create MHO Indication message ueID: %d", ue.IMSI)

	plmnID := ransimtypes.NewUint24(uint32(m.ServiceModel.Model.PlmnID))
	measReport := make([]*e2sm_mho.E2SmMhoMeasurementReportItem, 0)

	if len(ue.Cells) == 0 {
		log.Infof("no neighbor cells found for ueID:%d", ue.IMSI)
		return nil, nil
	}

	// add serving cell to measReport
	measReport = append(measReport, &e2sm_mho.E2SmMhoMeasurementReportItem{
		Cgi: &e2sm_mho.CellGlobalId{
			CellGlobalId: &e2sm_mho.CellGlobalId_NrCgi{
				NrCgi: &e2sm_mho.Nrcgi{
					PLmnIdentity: &e2sm_mho.PlmnIdentity{
						Value: plmnID.ToBytes(),
					},
					NRcellIdentity: &e2sm_mho.NrcellIdentity{
						Value: &e2sm_mho.BitString{
							Value: uint64(ransimtypes.GetNCI(ue.Cell.NCGI)),
							Len:   36,
						},
					},
				},
			},
		},
		Rsrp: &e2sm_mho.Rsrp{
			Value: int32(ue.Cell.Strength),
		},
	})

	for _, cell := range ue.Cells {
		measReport = append(measReport, &e2sm_mho.E2SmMhoMeasurementReportItem{
			Cgi: &e2sm_mho.CellGlobalId{
				CellGlobalId: &e2sm_mho.CellGlobalId_NrCgi{
					NrCgi: &e2sm_mho.Nrcgi{
						PLmnIdentity: &e2sm_mho.PlmnIdentity{
							Value: plmnID.ToBytes(),
						},
						NRcellIdentity: &e2sm_mho.NrcellIdentity{
							Value: &e2sm_mho.BitString{
								Value: uint64(ransimtypes.GetNCI(cell.NCGI)),
								Len:   36,
							},
						},
					},
				},
			},
			Rsrp: &e2sm_mho.Rsrp{
				Value: int32(cell.Strength),
			},
		})
	}

	ueID := strconv.Itoa(int(ue.IMSI))

	log.Debugf("MHO measurement report for ueID %s: %v", ueID, measReport)

	indicationMessage := indMsgFmt1.NewIndicationMessage(
		indMsgFmt1.WithUeID(ueID),
		indMsgFmt1.WithMeasReport(measReport))

	log.Debugf("MHO measurement report indication message for ueID %s: %v", ueID, indicationMessage)

	mhoModelPlugin, err := m.ServiceModel.ModelPluginRegistry.GetPlugin(e2smtypes.OID(m.ServiceModel.OID))
	if err != nil {
		return nil, err
	}
	indicationMessageBytes, err := indicationMessage.ToAsn1Bytes(mhoModelPlugin)
	if err != nil {
		log.Warn(err)
		return nil, err
	}

	return indicationMessageBytes, nil
}

func (m *Mho) createIndicationMsgFormat2(ue *model.UE) ([]byte, error) {
	log.Debugf("Create MHO RRC indication message ueID: %d", ue.IMSI)

	ueID := strconv.Itoa(int(ue.IMSI))

	indicationMessage := indMsgFmt2.NewIndicationMessage(
		indMsgFmt2.WithUeID(ueID),
		indMsgFmt2.WithRrcStatus(ue.RrcState))

	log.Debugf("MHO RRC state indication message for ueID %s: %v", ueID, indicationMessage)

	mhoModelPlugin, err := m.ServiceModel.ModelPluginRegistry.GetPlugin(e2smtypes.OID(m.ServiceModel.OID))
	if err != nil {
		return nil, err
	}
	indicationMessageBytes, err := indicationMessage.ToAsn1Bytes(mhoModelPlugin)
	if err != nil {
		log.Warn(err)
		return nil, err
	}

	return indicationMessageBytes, nil
}
