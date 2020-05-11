package eventstore

import (
	"context"

	"github.com/caos/zitadel/internal/errors"
	"github.com/caos/zitadel/internal/eventstore/internal/repository"
	"github.com/caos/zitadel/internal/eventstore/models"
)

type Eventstore interface {
	AggregateCreator() *models.AggregateCreator
	Health(ctx context.Context) error
	PushAggregates(ctx context.Context, aggregates ...*models.Aggregate) error
	FilterEvents(ctx context.Context, searchQuery *models.SearchQuery) (events []*models.Event, err error)

	PushAggregateStructs(ctx context.Context, aggregates []models.AggregateStruct) (err error)
}

var _ Eventstore = (*eventstore)(nil)

type eventstore struct {
	repo             repository.Repository
	aggregateCreator *models.AggregateCreator
}

func (es *eventstore) AggregateCreator() *models.AggregateCreator {
	return es.aggregateCreator
}

func (es *eventstore) PushAggregates(ctx context.Context, aggregates ...*models.Aggregate) (err error) {
	for _, aggregate := range aggregates {
		if len(aggregate.Events) == 0 {
			return errors.ThrowInvalidArgument(nil, "EVENT-cNhIj", "no events in aggregate")
		}
		for _, event := range aggregate.Events {
			if err = event.Validate(); err != nil {
				return errors.ThrowInvalidArgument(err, "EVENT-tzIhl", "validate event failed")
			}
		}
	}
	err = es.repo.PushAggregates(ctx, aggregates...)
	if err != nil {
		return err
	}

	return nil
}

func (es *eventstore) PushAggregateStructs(ctx context.Context, aggregates []models.AggregateStruct) (err error) {
	if len(aggregates) == 0 {
		return errors.ThrowInvalidArgument(nil, "EVENT-eTPXl", "no aggregates")
	}
	return es.repo.PushAggregateStructs(ctx, aggregates)
}

func (es *eventstore) FilterEvents(ctx context.Context, searchQuery *models.SearchQuery) ([]*models.Event, error) {
	if err := searchQuery.Validate(); err != nil {
		return nil, err
	}
	return es.repo.Filter(ctx, searchQuery)
}

func (es *eventstore) Health(ctx context.Context) error {
	return es.repo.Health(ctx)
}
