package handler

import (
	"github.com/caos/logging"
	"github.com/caos/zitadel/internal/eventstore/models"
	es_models "github.com/caos/zitadel/internal/eventstore/models"
	"github.com/caos/zitadel/internal/eventstore/spooler"
	iam_model "github.com/caos/zitadel/internal/iam/model"
	iam_es_model "github.com/caos/zitadel/internal/iam/repository/eventsourcing/model"
	iam_view_model "github.com/caos/zitadel/internal/iam/repository/view/model"
	"github.com/caos/zitadel/internal/org/repository/eventsourcing/model"
)

type IDPConfig struct {
	handler
}

const (
	idpConfigTable = "management.idp_configs"
)

func (m *IDPConfig) ViewModel() string {
	return idpConfigTable
}

func (m *IDPConfig) EventQuery() (*models.SearchQuery, error) {
	sequence, err := m.view.GetLatestIDPConfigSequence()
	if err != nil {
		return nil, err
	}
	return es_models.NewSearchQuery().
		AggregateTypeFilter(model.OrgAggregate, iam_es_model.IAMAggregate).
		LatestSequenceFilter(sequence.CurrentSequence), nil
}

func (m *IDPConfig) Reduce(event *models.Event) (err error) {
	switch event.AggregateType {
	case model.OrgAggregate:
		err = m.processIdpConfig(iam_model.IDPProviderTypeOrg, event)
	case iam_es_model.IAMAggregate:
		err = m.processIdpConfig(iam_model.IDPProviderTypeSystem, event)
	}
	return err
}

func (m *IDPConfig) processIdpConfig(providerType iam_model.IDPProviderType, event *models.Event) (err error) {
	idp := new(iam_view_model.IDPConfigView)
	switch event.Type {
	case model.IDPConfigAdded,
		iam_es_model.IDPConfigAdded:
		err = idp.AppendEvent(providerType, event)
	case model.IDPConfigChanged, iam_es_model.IDPConfigChanged,
		model.OIDCIDPConfigAdded, iam_es_model.OIDCIDPConfigAdded,
		model.OIDCIDPConfigChanged, iam_es_model.OIDCIDPConfigChanged:
		err = idp.SetData(event)
		if err != nil {
			return err
		}
		idp, err = m.view.IDPConfigByID(idp.IDPConfigID)
		if err != nil {
			return err
		}
		err = idp.AppendEvent(providerType, event)
	case model.IDPConfigRemoved, iam_es_model.IDPConfigRemoved:
		err = idp.SetData(event)
		if err != nil {
			return err
		}
		return m.view.DeleteIDPConfig(idp.IDPConfigID, event.Sequence)
	default:
		return m.view.ProcessedIDPConfigSequence(event.Sequence)
	}
	if err != nil {
		return err
	}
	return m.view.PutIDPConfig(idp, idp.Sequence)
}

func (m *IDPConfig) OnError(event *models.Event, err error) error {
	logging.LogWithFields("SPOOL-Nxu8s", "id", event.AggregateID).WithError(err).Warn("something went wrong in idp config handler")
	return spooler.HandleError(event, err, m.view.GetLatestIDPConfigFailedEvent, m.view.ProcessedIDPConfigFailedEvent, m.view.ProcessedIDPConfigSequence, m.errorCountUntilSkip)
}
