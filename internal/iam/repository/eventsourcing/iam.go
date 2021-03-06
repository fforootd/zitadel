package eventsourcing

import (
	"context"

	"github.com/caos/zitadel/internal/errors"
	es_models "github.com/caos/zitadel/internal/eventstore/models"
	"github.com/caos/zitadel/internal/iam/repository/eventsourcing/model"
)

func IAMByIDQuery(id string, latestSequence uint64) (*es_models.SearchQuery, error) {
	if id == "" {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-0soe4", "Errors.IAM.IDMissing")
	}
	return IAMQuery(latestSequence).
		AggregateIDFilter(id), nil
}

func IAMQuery(latestSequence uint64) *es_models.SearchQuery {
	return es_models.NewSearchQuery().
		AggregateTypeFilter(model.IAMAggregate).
		LatestSequenceFilter(latestSequence)
}

func IAMAggregate(ctx context.Context, aggCreator *es_models.AggregateCreator, iam *model.IAM) (*es_models.Aggregate, error) {
	if iam == nil {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-lo04e", "Errors.Internal")
	}
	return aggCreator.NewAggregate(ctx, iam.AggregateID, model.IAMAggregate, model.IAMVersion, iam.Sequence)
}

func IAMAggregateOverwriteContext(ctx context.Context, aggCreator *es_models.AggregateCreator, iam *model.IAM, resourceOwnerID string, userID string) (*es_models.Aggregate, error) {
	if iam == nil {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-dis83", "Errors.Internal")
	}

	return aggCreator.NewAggregate(ctx, iam.AggregateID, model.IAMAggregate, model.IAMVersion, iam.Sequence, es_models.OverwriteResourceOwner(resourceOwnerID), es_models.OverwriteEditorUser(userID))
}

func IAMSetupStartedAggregate(aggCreator *es_models.AggregateCreator, iam *model.IAM) func(ctx context.Context) (*es_models.Aggregate, error) {
	return func(ctx context.Context) (*es_models.Aggregate, error) {
		agg, err := IAMAggregate(ctx, aggCreator, iam)
		if err != nil {
			return nil, err
		}
		return agg.AppendEvent(model.IAMSetupStarted, nil)
	}
}

func IAMSetupDoneAggregate(aggCreator *es_models.AggregateCreator, iam *model.IAM) func(ctx context.Context) (*es_models.Aggregate, error) {
	return func(ctx context.Context) (*es_models.Aggregate, error) {
		agg, err := IAMAggregate(ctx, aggCreator, iam)
		if err != nil {
			return nil, err
		}

		return agg.AppendEvent(model.IAMSetupDone, nil)
	}
}

func IAMSetGlobalOrgAggregate(aggCreator *es_models.AggregateCreator, iam *model.IAM, globalOrg string) func(ctx context.Context) (*es_models.Aggregate, error) {
	return func(ctx context.Context) (*es_models.Aggregate, error) {
		if globalOrg == "" {
			return nil, errors.ThrowPreconditionFailed(nil, "EVENT-8siwa", "Errors.IAM.GlobalOrgMissing")
		}
		agg, err := IAMAggregate(ctx, aggCreator, iam)
		if err != nil {
			return nil, err
		}
		return agg.AppendEvent(model.GlobalOrgSet, &model.IAM{GlobalOrgID: globalOrg})
	}
}

func IAMSetIamProjectAggregate(aggCreator *es_models.AggregateCreator, iam *model.IAM, projectID string) func(ctx context.Context) (*es_models.Aggregate, error) {
	return func(ctx context.Context) (*es_models.Aggregate, error) {
		if projectID == "" {
			return nil, errors.ThrowPreconditionFailed(nil, "EVENT-sjuw3", "Errors.IAM.IamProjectIDMisisng")
		}
		agg, err := IAMAggregate(ctx, aggCreator, iam)
		if err != nil {
			return nil, err
		}
		return agg.AppendEvent(model.IAMProjectSet, &model.IAM{IAMProjectID: projectID})
	}
}

func IAMMemberAddedAggregate(aggCreator *es_models.AggregateCreator, existingIAM *model.IAM, member *model.IAMMember) func(ctx context.Context) (*es_models.Aggregate, error) {
	return func(ctx context.Context) (*es_models.Aggregate, error) {
		if member == nil {
			return nil, errors.ThrowPreconditionFailed(nil, "EVENT-9sope", "Errors.Internal")
		}
		agg, err := IAMAggregate(ctx, aggCreator, existingIAM)
		if err != nil {
			return nil, err
		}
		return agg.AppendEvent(model.IAMMemberAdded, member)
	}
}

func IAMMemberChangedAggregate(aggCreator *es_models.AggregateCreator, existingIAM *model.IAM, member *model.IAMMember) func(ctx context.Context) (*es_models.Aggregate, error) {
	return func(ctx context.Context) (*es_models.Aggregate, error) {
		if member == nil {
			return nil, errors.ThrowPreconditionFailed(nil, "EVENT-38skf", "Errors.Internal")
		}

		agg, err := IAMAggregate(ctx, aggCreator, existingIAM)
		if err != nil {
			return nil, err
		}
		return agg.AppendEvent(model.IAMMemberChanged, member)
	}
}

func IAMMemberRemovedAggregate(aggCreator *es_models.AggregateCreator, existingIAM *model.IAM, member *model.IAMMember) func(ctx context.Context) (*es_models.Aggregate, error) {
	return func(ctx context.Context) (*es_models.Aggregate, error) {
		if member == nil {
			return nil, errors.ThrowPreconditionFailed(nil, "EVENT-90lsw", "Errors.Internal")
		}
		agg, err := IAMAggregate(ctx, aggCreator, existingIAM)
		if err != nil {
			return nil, err
		}
		return agg.AppendEvent(model.IAMMemberRemoved, member)
	}
}

func IDPConfigAddedAggregate(aggCreator *es_models.AggregateCreator, existing *model.IAM, idp *model.IDPConfig) func(ctx context.Context) (*es_models.Aggregate, error) {
	return func(ctx context.Context) (*es_models.Aggregate, error) {
		if idp == nil {
			return nil, errors.ThrowPreconditionFailed(nil, "EVENT-MSn7d", "Errors.Internal")
		}
		agg, err := IAMAggregate(ctx, aggCreator, existing)
		if err != nil {
			return nil, err
		}
		agg, err = agg.AppendEvent(model.IDPConfigAdded, idp)
		if err != nil {
			return nil, err
		}
		if idp.OIDCIDPConfig != nil {
			return agg.AppendEvent(model.OIDCIDPConfigAdded, idp.OIDCIDPConfig)
		}
		return agg, nil
	}
}

func IDPConfigChangedAggregate(aggCreator *es_models.AggregateCreator, existing *model.IAM, idp *model.IDPConfig) func(ctx context.Context) (*es_models.Aggregate, error) {
	return func(ctx context.Context) (*es_models.Aggregate, error) {
		if idp == nil {
			return nil, errors.ThrowPreconditionFailed(nil, "EVENT-Amc7s", "Errors.Internal")
		}
		agg, err := IAMAggregate(ctx, aggCreator, existing)
		if err != nil {
			return nil, err
		}
		var changes map[string]interface{}
		for _, i := range existing.IDPs {
			if i.IDPConfigID == idp.IDPConfigID {
				changes = i.Changes(idp)
			}
		}
		return agg.AppendEvent(model.IDPConfigChanged, changes)
	}
}

func IDPConfigRemovedAggregate(ctx context.Context, aggCreator *es_models.AggregateCreator, existing *model.IAM, idp *model.IDPConfig, provider *model.IDPProvider) (*es_models.Aggregate, error) {
	if idp == nil {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-se23g", "Errors.Internal")
	}
	agg, err := IAMAggregate(ctx, aggCreator, existing)
	if err != nil {
		return nil, err
	}
	agg, err = agg.AppendEvent(model.IDPConfigRemoved, &model.IDPConfigID{IDPConfigID: idp.IDPConfigID})
	if err != nil {
		return nil, err
	}
	if provider != nil {
		return agg.AppendEvent(model.LoginPolicyIDPProviderCascadeRemoved, &model.IDPConfigID{IDPConfigID: idp.IDPConfigID})
	}
	return agg, nil
}

func IDPConfigDeactivatedAggregate(aggCreator *es_models.AggregateCreator, existing *model.IAM, idp *model.IDPConfig) func(ctx context.Context) (*es_models.Aggregate, error) {
	return func(ctx context.Context) (*es_models.Aggregate, error) {
		if idp == nil {
			return nil, errors.ThrowPreconditionFailed(nil, "EVENT-slfi3", "Errors.Internal")
		}
		agg, err := IAMAggregate(ctx, aggCreator, existing)
		if err != nil {
			return nil, err
		}
		return agg.AppendEvent(model.IDPConfigDeactivated, &model.IDPConfigID{IDPConfigID: idp.IDPConfigID})
	}
}

func IDPConfigReactivatedAggregate(aggCreator *es_models.AggregateCreator, existing *model.IAM, idp *model.IDPConfig) func(ctx context.Context) (*es_models.Aggregate, error) {
	return func(ctx context.Context) (*es_models.Aggregate, error) {
		if idp == nil {
			return nil, errors.ThrowPreconditionFailed(nil, "EVENT-slf32", "Errors.Internal")
		}
		agg, err := IAMAggregate(ctx, aggCreator, existing)
		if err != nil {
			return nil, err
		}
		return agg.AppendEvent(model.IDPConfigReactivated, &model.IDPConfigID{IDPConfigID: idp.IDPConfigID})
	}
}

func OIDCIDPConfigChangedAggregate(aggCreator *es_models.AggregateCreator, existing *model.IAM, config *model.OIDCIDPConfig) func(ctx context.Context) (*es_models.Aggregate, error) {
	return func(ctx context.Context) (*es_models.Aggregate, error) {
		if config == nil {
			return nil, errors.ThrowPreconditionFailed(nil, "EVENT-slf32", "Errors.Internal")
		}
		agg, err := IAMAggregate(ctx, aggCreator, existing)
		if err != nil {
			return nil, err
		}
		var changes map[string]interface{}
		for _, idp := range existing.IDPs {
			if idp.IDPConfigID == config.IDPConfigID && idp.OIDCIDPConfig != nil {
				changes = idp.OIDCIDPConfig.Changes(config)
			}
		}
		if len(changes) <= 1 {
			return nil, errors.ThrowPreconditionFailedf(nil, "EVENT-Cml9s", "Errors.NoChangesFound")
		}
		return agg.AppendEvent(model.OIDCIDPConfigChanged, changes)
	}
}

func LoginPolicyAddedAggregate(aggCreator *es_models.AggregateCreator, existing *model.IAM, policy *model.LoginPolicy) func(ctx context.Context) (*es_models.Aggregate, error) {
	return func(ctx context.Context) (*es_models.Aggregate, error) {
		if policy == nil {
			return nil, errors.ThrowPreconditionFailed(nil, "EVENT-Smla8", "Errors.Internal")
		}
		agg, err := IAMAggregate(ctx, aggCreator, existing)
		if err != nil {
			return nil, err
		}
		validationQuery := es_models.NewSearchQuery().
			AggregateTypeFilter(model.IAMAggregate).
			EventTypesFilter(model.LoginPolicyAdded).
			AggregateIDFilter(existing.AggregateID)

		validation := checkExistingLoginPolicyValidation()
		agg.SetPrecondition(validationQuery, validation)
		return agg.AppendEvent(model.LoginPolicyAdded, policy)
	}
}

func LoginPolicyChangedAggregate(aggCreator *es_models.AggregateCreator, existing *model.IAM, policy *model.LoginPolicy) func(ctx context.Context) (*es_models.Aggregate, error) {
	return func(ctx context.Context) (*es_models.Aggregate, error) {
		if policy == nil {
			return nil, errors.ThrowPreconditionFailed(nil, "EVENT-Mlco9", "Errors.Internal")
		}
		agg, err := IAMAggregate(ctx, aggCreator, existing)
		if err != nil {
			return nil, err
		}
		changes := existing.DefaultLoginPolicy.Changes(policy)
		if len(changes) == 0 {
			return nil, errors.ThrowPreconditionFailed(nil, "EVENT-Smk8d", "Errors.NoChangesFound")
		}
		return agg.AppendEvent(model.LoginPolicyChanged, changes)
	}
}

func LoginPolicyIDPProviderAddedAggregate(aggCreator *es_models.AggregateCreator, existing *model.IAM, provider *model.IDPProvider) func(ctx context.Context) (*es_models.Aggregate, error) {
	return func(ctx context.Context) (*es_models.Aggregate, error) {
		if provider == nil {
			return nil, errors.ThrowPreconditionFailed(nil, "EVENT-Sml9d", "Errors.Internal")
		}
		agg, err := IAMAggregate(ctx, aggCreator, existing)
		if err != nil {
			return nil, err
		}
		validationQuery := es_models.NewSearchQuery().
			AggregateTypeFilter(model.IAMAggregate).
			AggregateIDFilter(existing.AggregateID)

		validation := checkExistingLoginPolicyIDPProviderValidation(provider.IDPConfigID)
		agg.SetPrecondition(validationQuery, validation)
		return agg.AppendEvent(model.LoginPolicyIDPProviderAdded, provider)
	}
}

func LoginPolicyIDPProviderRemovedAggregate(ctx context.Context, aggCreator *es_models.AggregateCreator, existing *model.IAM, provider *model.IDPProviderID) (*es_models.Aggregate, error) {
	if provider == nil || existing == nil {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-Sml9d", "Errors.Internal")
	}
	agg, err := IAMAggregate(ctx, aggCreator, existing)
	if err != nil {
		return nil, err
	}
	return agg.AppendEvent(model.LoginPolicyIDPProviderRemoved, provider)
}

func checkExistingLoginPolicyValidation() func(...*es_models.Event) error {
	return func(events ...*es_models.Event) error {
		for _, event := range events {
			switch event.Type {
			case model.LoginPolicyAdded:
				return errors.ThrowPreconditionFailed(nil, "EVENT-Ski9d", "Errors.IAM.LoginPolicy.AlreadyExists")
			}
		}
		return nil
	}
}

func checkExistingLoginPolicyIDPProviderValidation(idpConfigID string) func(...*es_models.Event) error {
	return func(events ...*es_models.Event) error {
		idpConfigs := make([]*model.IDPConfig, 0)
		idps := make([]*model.IDPProvider, 0)
		for _, event := range events {
			switch event.Type {
			case model.IDPConfigAdded:
				config := new(model.IDPConfig)
				config.SetData(event)
				idpConfigs = append(idpConfigs, config)
			case model.IDPConfigRemoved:
				config := new(model.IDPConfig)
				config.SetData(event)
				for i, p := range idpConfigs {
					if p.IDPConfigID == config.IDPConfigID {
						idpConfigs[i] = idpConfigs[len(idpConfigs)-1]
						idpConfigs[len(idpConfigs)-1] = nil
						idpConfigs = idpConfigs[:len(idpConfigs)-1]
					}
				}
			case model.LoginPolicyIDPProviderAdded:
				idp := new(model.IDPProvider)
				idp.SetData(event)
				idps = append(idps, idp)
			case model.LoginPolicyIDPProviderRemoved:
				idp := new(model.IDPProvider)
				idp.SetData(event)
				for i, p := range idps {
					if p.IDPConfigID == idp.IDPConfigID {
						idps[i] = idps[len(idps)-1]
						idps[len(idps)-1] = nil
						idps = idps[:len(idps)-1]
					}
				}
			}
		}
		exists := false
		for _, p := range idpConfigs {
			if p.IDPConfigID == idpConfigID {
				exists = true
			}
		}
		if !exists {
			return errors.ThrowPreconditionFailed(nil, "EVENT-Djlo9", "Errors.IAM.IdpNotExisting")
		}
		for _, p := range idps {
			if p.IDPConfigID == idpConfigID {
				return errors.ThrowPreconditionFailed(nil, "EVENT-us5Zw", "Errors.IAM.LoginPolicy.IdpProviderAlreadyExisting")
			}
		}
		return nil
	}
}
