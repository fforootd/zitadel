package eventsourcing

import (
	"context"
	"fmt"
	"time"

	"github.com/caos/logging"
	"github.com/golang/protobuf/ptypes"

	"github.com/caos/zitadel/internal/errors"
	"github.com/caos/zitadel/internal/id"
	org_model "github.com/caos/zitadel/internal/org/model"
	policy_model "github.com/caos/zitadel/internal/policy/model"

	"github.com/pquerna/otp/totp"

	req_model "github.com/caos/zitadel/internal/auth_request/model"
	"github.com/caos/zitadel/internal/cache/config"
	sd "github.com/caos/zitadel/internal/config/systemdefaults"
	"github.com/caos/zitadel/internal/crypto"
	caos_errs "github.com/caos/zitadel/internal/errors"
	es_int "github.com/caos/zitadel/internal/eventstore"
	es_models "github.com/caos/zitadel/internal/eventstore/models"
	es_sdk "github.com/caos/zitadel/internal/eventstore/sdk"
	global_model "github.com/caos/zitadel/internal/model"
	usr_model "github.com/caos/zitadel/internal/user/model"
	"github.com/caos/zitadel/internal/user/repository/eventsourcing/model"
)

const (
	yearLayout            = "2006-01-02"
	defaultExpirationDate = "9999-01-01"
)

type UserEventstore struct {
	es_int.Eventstore
	userCache                *UserCache
	idGenerator              id.Generator
	domain                   string
	PasswordAlg              crypto.HashAlgorithm
	InitializeUserCode       crypto.Generator
	EmailVerificationCode    crypto.Generator
	PhoneVerificationCode    crypto.Generator
	PasswordVerificationCode crypto.Generator
	MachineKeyAlg            crypto.EncryptionAlgorithm
	MachineKeySize           int
	Multifactors             global_model.Multifactors
	validateTOTP             func(string, string) bool
}

type UserConfig struct {
	es_int.Eventstore
	Cache            *config.CacheConfig
	PasswordSaltCost int
}

func StartUser(conf UserConfig, systemDefaults sd.SystemDefaults) (*UserEventstore, error) {
	userCache, err := StartCache(conf.Cache)
	if err != nil {
		return nil, err
	}
	aesCrypto, err := crypto.NewAESCrypto(systemDefaults.UserVerificationKey)
	if err != nil {
		return nil, err
	}
	initCodeGen := crypto.NewEncryptionGenerator(systemDefaults.SecretGenerators.InitializeUserCode, aesCrypto)
	emailVerificationCode := crypto.NewEncryptionGenerator(systemDefaults.SecretGenerators.EmailVerificationCode, aesCrypto)
	phoneVerificationCode := crypto.NewEncryptionGenerator(systemDefaults.SecretGenerators.PhoneVerificationCode, aesCrypto)
	passwordVerificationCode := crypto.NewEncryptionGenerator(systemDefaults.SecretGenerators.PasswordVerificationCode, aesCrypto)
	aesOtpCrypto, err := crypto.NewAESCrypto(systemDefaults.Multifactors.OTP.VerificationKey)
	passwordAlg := crypto.NewBCrypt(systemDefaults.SecretGenerators.PasswordSaltCost)

	return &UserEventstore{
		Eventstore:               conf.Eventstore,
		userCache:                userCache,
		idGenerator:              id.SonyFlakeGenerator,
		domain:                   systemDefaults.Domain,
		InitializeUserCode:       initCodeGen,
		EmailVerificationCode:    emailVerificationCode,
		PhoneVerificationCode:    phoneVerificationCode,
		PasswordVerificationCode: passwordVerificationCode,
		Multifactors: global_model.Multifactors{
			OTP: global_model.OTP{
				CryptoMFA: aesOtpCrypto,
				Issuer:    systemDefaults.Multifactors.OTP.Issuer,
			},
		},
		PasswordAlg:    passwordAlg,
		validateTOTP:   totp.Validate,
		MachineKeyAlg:  aesCrypto,
		MachineKeySize: int(systemDefaults.SecretGenerators.MachineKeySize),
	}, nil
}

func (es *UserEventstore) UserByID(ctx context.Context, id string) (*usr_model.User, error) {
	user := es.userCache.getUser(id)

	query, err := UserByIDQuery(user.AggregateID, user.Sequence)
	if err != nil {
		return nil, err
	}
	err = es_sdk.Filter(ctx, es.FilterEvents, user.AppendEvents, query)
	if err != nil && caos_errs.IsNotFound(err) && user.Sequence == 0 {
		return nil, err
	}
	es.userCache.cacheUser(user)
	return model.UserToModel(user), nil
}

func (es *UserEventstore) UserEventsByID(ctx context.Context, id string, sequence uint64) ([]*es_models.Event, error) {
	query, err := UserByIDQuery(id, sequence)
	if err != nil {
		return nil, err
	}
	return es.FilterEvents(ctx, query)
}

func (es *UserEventstore) prepareCreateMachine(ctx context.Context, user *usr_model.User, orgIamPolicy *org_model.OrgIAMPolicy, resourceOwner string) (*model.User, []*es_models.Aggregate, error) {
	machine := model.UserFromModel(user)

	if !orgIamPolicy.UserLoginMustBeDomain {
		return nil, nil, errors.ThrowPreconditionFailed(nil, "EVENT-cJlnI", "Errors.User.Invalid")
	}

	createAggregates, err := MachineCreateAggregate(ctx, es.AggregateCreator(), machine, resourceOwner, true)

	return machine, createAggregates, err
}

func (es *UserEventstore) prepareCreateHuman(ctx context.Context, user *usr_model.User, pwPolicy *policy_model.PasswordComplexityPolicy, orgIAMPolicy *org_model.OrgIAMPolicy, resourceOwner string) (*model.User, []*es_models.Aggregate, error) {
	err := user.CheckOrgIAMPolicy(orgIAMPolicy)
	if err != nil {
		return nil, nil, err
	}
	user.SetNamesAsDisplayname()
	if !user.IsValid() {
		return nil, nil, caos_errs.ThrowPreconditionFailed(nil, "EVENT-LoIxJ", "Errors.User.Invalid")
	}

	err = user.HashPasswordIfExisting(pwPolicy, es.PasswordAlg, true)
	if err != nil {
		return nil, nil, err
	}
	err = user.GenerateInitCodeIfNeeded(es.InitializeUserCode)
	if err != nil {
		return nil, nil, err
	}
	err = user.GeneratePhoneCodeIfNeeded(es.PhoneVerificationCode)
	if err != nil {
		return nil, nil, err
	}

	repoUser := model.UserFromModel(user)
	repoInitCode := model.InitCodeFromModel(user.InitCode)
	repoPhoneCode := model.PhoneCodeFromModel(user.PhoneCode)

	createAggregates, err := HumanCreateAggregate(ctx, es.AggregateCreator(), repoUser, repoInitCode, repoPhoneCode, resourceOwner, orgIAMPolicy.UserLoginMustBeDomain)

	return repoUser, createAggregates, err
}

func (es *UserEventstore) PrepareCreateUser(ctx context.Context, user *usr_model.User, pwPolicy *policy_model.PasswordComplexityPolicy, orgIAMPolicy *org_model.OrgIAMPolicy, resourceOwner string) (*model.User, []*es_models.Aggregate, error) {
	id, err := es.idGenerator.Next()
	if err != nil {
		return nil, nil, err
	}
	user.AggregateID = id

	if user.Human != nil {
		return es.prepareCreateHuman(ctx, user, pwPolicy, orgIAMPolicy, resourceOwner)
	} else if user.Machine != nil {
		return es.prepareCreateMachine(ctx, user, orgIAMPolicy, resourceOwner)
	}
	return nil, nil, errors.ThrowInvalidArgument(nil, "EVENT-Q29tp", "Errors.User.TypeUndefined")
}

func (es *UserEventstore) CreateUser(ctx context.Context, user *usr_model.User, pwPolicy *policy_model.PasswordComplexityPolicy, orgIAMPolicy *org_model.OrgIAMPolicy) (*usr_model.User, error) {
	repoUser, aggregates, err := es.PrepareCreateUser(ctx, user, pwPolicy, orgIAMPolicy, "")
	if err != nil {
		return nil, err
	}

	err = es_sdk.PushAggregates(ctx, es.PushAggregates, repoUser.AppendEvents, aggregates...)
	if err != nil {
		return nil, err
	}

	es.userCache.cacheUser(repoUser)
	return model.UserToModel(repoUser), nil
}

func (es *UserEventstore) PrepareRegisterUser(ctx context.Context, user *usr_model.User, externalIDP *usr_model.ExternalIDP, policy *policy_model.PasswordComplexityPolicy, orgIAMPolicy *org_model.OrgIAMPolicy, resourceOwner string) (*model.User, []*es_models.Aggregate, error) {
	if user.Human == nil {
		return nil, nil, caos_errs.ThrowInvalidArgument(nil, "EVENT-ht8Ux", "Errors.User.Invalid")
	}

	err := user.CheckOrgIAMPolicy(orgIAMPolicy)
	if err != nil {
		return nil, nil, err
	}
	user.SetNamesAsDisplayname()
	if !user.IsValid() || externalIDP == nil && (user.Password == nil || user.SecretString == "") {
		return nil, nil, caos_errs.ThrowPreconditionFailed(nil, "EVENT-9dk45", "Errors.User.Invalid")
	}
	id, err := es.idGenerator.Next()
	if err != nil {
		return nil, nil, err
	}
	user.AggregateID = id
	if externalIDP != nil {
		externalIDP.AggregateID = id
		if !externalIDP.IsValid() {
			return nil, nil, errors.ThrowPreconditionFailed(nil, "EVENT-4Dj9s", "Errors.User.ExternalIDP.Invalid")
		}
		user.ExternalIDPs = append(user.ExternalIDPs, externalIDP)
	}
	err = user.HashPasswordIfExisting(policy, es.PasswordAlg, false)
	if err != nil {
		return nil, nil, err
	}
	err = user.GenerateInitCodeIfNeeded(es.InitializeUserCode)
	if err != nil {
		return nil, nil, err
	}

	repoUser := model.UserFromModel(user)
	repoExternalIDP := model.ExternalIDPFromModel(externalIDP)
	repoInitCode := model.InitCodeFromModel(user.InitCode)

	aggregates, err := UserRegisterAggregate(ctx, es.AggregateCreator(), repoUser, repoExternalIDP, resourceOwner, repoInitCode, orgIAMPolicy.UserLoginMustBeDomain)
	return repoUser, aggregates, err
}

func (es *UserEventstore) RegisterUser(ctx context.Context, user *usr_model.User, pwPolicy *policy_model.PasswordComplexityPolicy, orgIAMPolicy *org_model.OrgIAMPolicy, resourceOwner string) (*usr_model.User, error) {
	repoUser, createAggregates, err := es.PrepareRegisterUser(ctx, user, nil, pwPolicy, orgIAMPolicy, resourceOwner)
	if err != nil {
		return nil, err
	}

	err = es_sdk.PushAggregates(ctx, es.PushAggregates, repoUser.AppendEvents, createAggregates...)
	if err != nil {
		return nil, err
	}

	es.userCache.cacheUser(repoUser)
	return model.UserToModel(repoUser), nil
}

func (es *UserEventstore) DeactivateUser(ctx context.Context, id string) (*usr_model.User, error) {
	user, err := es.UserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if user.IsInactive() {
		return nil, caos_errs.ThrowPreconditionFailed(nil, "EVENT-die45", "Errors.User.AlreadyInactive")
	}

	repoUser := model.UserFromModel(user)
	aggregate := UserDeactivateAggregate(es.AggregateCreator(), repoUser)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, aggregate)
	if err != nil {
		return nil, err
	}
	es.userCache.cacheUser(repoUser)
	return model.UserToModel(repoUser), nil
}

func (es *UserEventstore) ReactivateUser(ctx context.Context, id string) (*usr_model.User, error) {
	user, err := es.UserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !user.IsInactive() {
		return nil, caos_errs.ThrowPreconditionFailed(nil, "EVENT-do94s", "Errors.User.NotInactive")
	}

	repoUser := model.UserFromModel(user)
	aggregate := UserReactivateAggregate(es.AggregateCreator(), repoUser)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, aggregate)
	if err != nil {
		return nil, err
	}
	es.userCache.cacheUser(repoUser)
	return model.UserToModel(repoUser), nil
}

func (es *UserEventstore) LockUser(ctx context.Context, id string) (*usr_model.User, error) {
	user, err := es.UserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !user.IsActive() && !user.IsInitial() {
		return nil, caos_errs.ThrowPreconditionFailed(nil, "EVENT-di83s", "Errors.User.ShouldBeActiveOrInitial")
	}

	repoUser := model.UserFromModel(user)
	aggregate := UserLockAggregate(es.AggregateCreator(), repoUser)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, aggregate)
	if err != nil {
		return nil, err
	}
	es.userCache.cacheUser(repoUser)
	return model.UserToModel(repoUser), nil
}

func (es *UserEventstore) UnlockUser(ctx context.Context, id string) (*usr_model.User, error) {
	user, err := es.UserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !user.IsLocked() {
		return nil, caos_errs.ThrowPreconditionFailed(nil, "EVENT-dks83", "Errors.User.NotLocked")
	}

	repoUser := model.UserFromModel(user)
	aggregate := UserUnlockAggregate(es.AggregateCreator(), repoUser)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, aggregate)
	if err != nil {
		return nil, err
	}
	es.userCache.cacheUser(repoUser)
	return model.UserToModel(repoUser), nil
}

func (es *UserEventstore) UserChanges(ctx context.Context, id string, lastSequence uint64, limit uint64, sortAscending bool) (*usr_model.UserChanges, error) {
	query := ChangesQuery(id, lastSequence, limit, sortAscending)

	events, err := es.Eventstore.FilterEvents(context.Background(), query)
	if err != nil {
		logging.Log("EVENT-g9HCv").WithError(err).Warn("eventstore unavailable")
		return nil, errors.ThrowInternal(err, "EVENT-htuG9", "Errors.Internal")
	}
	if len(events) == 0 {
		return nil, caos_errs.ThrowNotFound(nil, "EVENT-6cAxe", "Errors.User.NoChanges")
	}

	result := make([]*usr_model.UserChange, len(events))

	for i, event := range events {
		creationDate, err := ptypes.TimestampProto(event.CreationDate)
		logging.Log("EVENT-8GTGS").OnError(err).Debug("unable to parse timestamp")
		change := &usr_model.UserChange{
			ChangeDate: creationDate,
			EventType:  event.Type.String(),
			ModifierID: event.EditorUser,
			Sequence:   event.Sequence,
		}

		//TODO: now all types should be unmarshalled, e.g. password
		// if len(event.Data) != 0 {
		// 	user := new(model.User)
		// 	err := json.Unmarshal(event.Data, user)
		// 	logging.Log("EVENT-Rkg7X").OnError(err).Debug("unable to unmarshal data")
		// 	change.Data = user
		// }

		result[i] = change
		if lastSequence < event.Sequence {
			lastSequence = event.Sequence
		}
	}

	return &usr_model.UserChanges{
		Changes:      result,
		LastSequence: lastSequence,
	}, nil
}

func ChangesQuery(userID string, latestSequence, limit uint64, sortAscending bool) *es_models.SearchQuery {
	query := es_models.NewSearchQuery().
		AggregateTypeFilter(model.UserAggregate)
	if !sortAscending {
		query.OrderDesc()
	}

	query.LatestSequenceFilter(latestSequence).
		AggregateIDFilter(userID).
		SetLimit(limit)
	return query
}

func (es *UserEventstore) InitializeUserCodeByID(ctx context.Context, userID string) (*usr_model.InitUserCode, error) {
	if userID == "" {
		return nil, caos_errs.ThrowPreconditionFailed(nil, "EVENT-d8diw", "Errors.User.UserIDMissing")
	}
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Human == nil {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-mDPtj", "Errors.User.NotHuman")
	}

	if user.InitCode != nil {
		return user.InitCode, nil
	}
	return nil, caos_errs.ThrowNotFound(nil, "EVENT-d8e2", "Erorrs.User.InitCodeNotFound")
}

func (es *UserEventstore) CreateInitializeUserCodeByID(ctx context.Context, userID string) (*usr_model.InitUserCode, error) {
	if userID == "" {
		return nil, caos_errs.ThrowPreconditionFailed(nil, "EVENT-dic8s", "Errors.User.UserIDMissing")
	}
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Human == nil {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-9bbXj", "Errors.User.NotHuman")
	}

	initCode := new(usr_model.InitUserCode)
	err = initCode.GenerateInitUserCode(es.InitializeUserCode)
	if err != nil {
		return nil, err
	}

	repoUser := model.UserFromModel(user)
	repoInitCode := model.InitCodeFromModel(initCode)

	agg := UserInitCodeAggregate(es.AggregateCreator(), repoUser, repoInitCode)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, agg)
	if err != nil {
		return nil, err
	}
	es.userCache.cacheUser(repoUser)
	return model.InitCodeToModel(repoUser.InitCode), nil
}

func (es *UserEventstore) InitCodeSent(ctx context.Context, userID string) error {
	if userID == "" {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-0posw", "Errors.User.UserIDMissing")
	}
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Human == nil {
		return errors.ThrowPreconditionFailed(nil, "EVENT-SvPa6", "Errors.User.NotHuman")
	}

	repoUser := model.UserFromModel(user)
	agg := UserInitCodeSentAggregate(es.AggregateCreator(), repoUser)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, agg)
	if err != nil {
		return err
	}
	es.userCache.cacheUser(repoUser)
	return nil
}

func (es *UserEventstore) VerifyInitCode(ctx context.Context, policy *policy_model.PasswordComplexityPolicy, userID, verificationCode, password string) error {
	if userID == "" {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-lo9fd", "Errors.User.UserIDMissing")
	}
	if verificationCode == "" {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-lo9fd", "Errors.User.Code.Empty")
	}
	pw := &usr_model.Password{SecretString: password}
	err := pw.HashPasswordIfExisting(policy, es.PasswordAlg, false)
	if err != nil {
		return err
	}
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Human == nil {
		return errors.ThrowPreconditionFailed(nil, "EVENT-b3xda", "Errors.User.NotHuman")
	}
	if user.InitCode == nil {
		return caos_errs.ThrowNotFound(nil, "EVENT-spo9W", "Errors.User.Code.NotFound")
	}
	repoPassword := model.PasswordFromModel(pw)
	repoUser := model.UserFromModel(user)
	var updateAggregate func(ctx context.Context) (*es_models.Aggregate, error)
	if err := crypto.VerifyCode(user.InitCode.CreationDate, user.InitCode.Expiry, user.InitCode.Code, verificationCode, es.InitializeUserCode); err != nil {
		updateAggregate = InitCodeCheckFailedAggregate(es.AggregateCreator(), repoUser)
		es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, updateAggregate)
		return err
	} else {
		updateAggregate = InitCodeVerifiedAggregate(es.AggregateCreator(), repoUser, repoPassword)
	}
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, updateAggregate)
	if err != nil {
		return err
	}

	es.userCache.cacheUser(repoUser)
	return nil
}

func (es *UserEventstore) SkipMfaInit(ctx context.Context, userID string) error {
	if userID == "" {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-dic8s", "Errors.User.UserIDMissing")
	}
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Human == nil {
		return errors.ThrowPreconditionFailed(nil, "EVENT-S1tdl", "Errors.User.NotHuman")
	}

	repoUser := model.UserFromModel(user)
	agg := SkipMfaAggregate(es.AggregateCreator(), repoUser)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, agg)
	if err != nil {
		return err
	}
	es.userCache.cacheUser(repoUser)
	return nil
}

func (es *UserEventstore) UserPasswordByID(ctx context.Context, userID string) (*usr_model.Password, error) {
	if userID == "" {
		return nil, caos_errs.ThrowPreconditionFailed(nil, "EVENT-di834", "Errors.User.UserIDMissing")
	}
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Human == nil {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-jLHYG", "Errors.User.NotHuman")
	}

	if user.Password != nil {
		return user.Password, nil
	}
	return nil, caos_errs.ThrowNotFound(nil, "EVENT-d8e2", "Errors.User.Password.NotFound")
}

func (es *UserEventstore) CheckPassword(ctx context.Context, userID, password string, authRequest *req_model.AuthRequest) error {
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Human == nil {
		return errors.ThrowPreconditionFailed(nil, "EVENT-HxcAx", "Errors.User.NotHuman")
	}
	if user.Password == nil {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-s35Fa", "Errors.User.Password.Empty")
	}
	if err := crypto.CompareHash(user.Password.SecretCrypto, []byte(password), es.PasswordAlg); err == nil {
		return es.setPasswordCheckResult(ctx, user, authRequest, PasswordCheckSucceededAggregate)
	}
	if err := es.setPasswordCheckResult(ctx, user, authRequest, PasswordCheckFailedAggregate); err != nil {
		return err
	}
	return caos_errs.ThrowInvalidArgument(nil, "EVENT-452ad", "Errors.User.Password.Invalid")
}

func (es *UserEventstore) setPasswordCheckResult(ctx context.Context, user *usr_model.User, authRequest *req_model.AuthRequest, check func(*es_models.AggregateCreator, *model.User, *model.AuthRequest) es_sdk.AggregateFunc) error {
	repoUser := model.UserFromModel(user)
	repoAuthRequest := model.AuthRequestFromModel(authRequest)
	agg := check(es.AggregateCreator(), repoUser, repoAuthRequest)
	err := es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, agg)
	if err != nil {
		return err
	}
	es.userCache.cacheUser(repoUser)
	return nil
}

func (es *UserEventstore) SetOneTimePassword(ctx context.Context, policy *policy_model.PasswordComplexityPolicy, password *usr_model.Password) (*usr_model.Password, error) {
	user, err := es.UserByID(ctx, password.AggregateID)
	if err != nil {
		return nil, err
	}
	if user.Human == nil {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-PjDfJ", "Errors.User.NotHuman")
	}
	return es.changedPassword(ctx, user, policy, password.SecretString, true)
}

func (es *UserEventstore) SetPassword(ctx context.Context, policy *policy_model.PasswordComplexityPolicy, userID, code, password string) error {
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Human == nil {
		return errors.ThrowPreconditionFailed(nil, "EVENT-pHkAQ", "Errors.User.NotHuman")
	}
	if user.PasswordCode == nil {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-65sdr", "Errors.User.Code.NotFound")
	}
	if err := crypto.VerifyCode(user.PasswordCode.CreationDate, user.PasswordCode.Expiry, user.PasswordCode.Code, code, es.PasswordVerificationCode); err != nil {
		return err
	}
	_, err = es.changedPassword(ctx, user, policy, password, false)
	return err
}

func (es *UserEventstore) ChangeMachine(ctx context.Context, machine *usr_model.Machine) (*usr_model.Machine, error) {
	user, err := es.UserByID(ctx, machine.AggregateID)
	if err != nil {
		return nil, err
	}
	if user.Machine == nil {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-OGUoz", "Errors.User.NotMachine")
	}

	repoUser := model.UserFromModel(user)
	repoMachine := model.MachineFromModel(machine)

	updateAggregate := MachineChangeAggregate(es.AggregateCreator(), repoUser, repoMachine)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, updateAggregate)
	if err != nil {
		return nil, err
	}

	es.userCache.cacheUser(repoUser)
	return model.MachineToModel(repoUser.Machine), nil
}

func (es *UserEventstore) ChangePassword(ctx context.Context, policy *policy_model.PasswordComplexityPolicy, userID, old, new string) (*usr_model.Password, error) {
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Human == nil {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-9AuLE", "Errors.User.NotHuman")
	}
	if user.Password == nil {
		return nil, caos_errs.ThrowPreconditionFailed(nil, "EVENT-Fds3s", "Errors.User.Password.Empty")
	}
	if err := crypto.CompareHash(user.Password.SecretCrypto, []byte(old), es.PasswordAlg); err != nil {
		return nil, caos_errs.ThrowInvalidArgument(nil, "EVENT-s56a3", "Errors.User.Password.Invalid")
	}
	return es.changedPassword(ctx, user, policy, new, false)
}

func (es *UserEventstore) changedPassword(ctx context.Context, user *usr_model.User, policy *policy_model.PasswordComplexityPolicy, password string, onetime bool) (*usr_model.Password, error) {
	pw := &usr_model.Password{SecretString: password}
	err := pw.HashPasswordIfExisting(policy, es.PasswordAlg, onetime)
	if err != nil {
		return nil, err
	}
	repoPassword := model.PasswordFromModel(pw)
	repoUser := model.UserFromModel(user)
	agg := PasswordChangeAggregate(es.AggregateCreator(), repoUser, repoPassword)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, agg)
	if err != nil {
		return nil, err
	}
	es.userCache.cacheUser(repoUser)

	return model.PasswordToModel(repoUser.Password), nil
}

func (es *UserEventstore) RequestSetPassword(ctx context.Context, userID string, notifyType usr_model.NotificationType) error {
	if userID == "" {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-dic8s", "Errors.User.UserIDMissing")
	}
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Human == nil {
		return errors.ThrowPreconditionFailed(nil, "EVENT-33ywz", "Errors.User.NotHuman")
	}

	passwordCode := new(model.PasswordCode)
	err = es.generatePasswordCode(passwordCode, notifyType)
	if err != nil {
		return err
	}

	repoUser := model.UserFromModel(user)
	agg := RequestSetPassword(es.AggregateCreator(), repoUser, passwordCode)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, agg)
	if err != nil {
		return err
	}
	es.userCache.cacheUser(repoUser)
	return nil
}

func (es *UserEventstore) PasswordCodeSent(ctx context.Context, userID string) error {
	if userID == "" {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-s09ow", "Errors.User.UserIDMissing")
	}
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Human == nil {
		return errors.ThrowPreconditionFailed(nil, "EVENT-tbVAo", "Errors.User.NotHuman")
	}

	repoUser := model.UserFromModel(user)
	agg := PasswordCodeSentAggregate(es.AggregateCreator(), repoUser)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, agg)
	if err != nil {
		return err
	}
	es.userCache.cacheUser(repoUser)
	return nil
}

func (es *UserEventstore) AddExternalIDP(ctx context.Context, externalIDP *usr_model.ExternalIDP) (*usr_model.ExternalIDP, error) {
	if externalIDP == nil || !externalIDP.IsValid() {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-Ek9s", "Errors.User.ExternalIDP.Invalid")
	}
	existingUser, err := es.UserByID(ctx, externalIDP.AggregateID)
	if err != nil {
		return nil, err
	}
	if existingUser.Human == nil {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-Cnk8s", "Errors.User.NotHuman")
	}
	repoUser := model.UserFromModel(existingUser)
	repoExternalIDP := model.ExternalIDPFromModel(externalIDP)
	aggregates, err := ExternalIDPAddedAggregate(ctx, es.Eventstore.AggregateCreator(), repoUser, repoExternalIDP)
	if err != nil {
		return nil, err
	}
	err = es_sdk.PushAggregates(ctx, es.PushAggregates, repoUser.AppendEvents, aggregates...)
	if err != nil {
		return nil, err
	}

	es.userCache.cacheUser(repoUser)
	if _, idp := model.GetExternalIDP(repoUser.ExternalIDPs, externalIDP.UserID); idp != nil {
		return model.ExternalIDPToModel(idp), nil
	}
	return nil, errors.ThrowInternal(nil, "EVENT-Msi9d", "Errors.Internal")
}

func (es *UserEventstore) BulkAddExternalIDPs(ctx context.Context, userID string, externalIDPs []*usr_model.ExternalIDP) error {
	if externalIDPs == nil || len(externalIDPs) == 0 {
		return errors.ThrowPreconditionFailed(nil, "EVENT-Ek9s", "Errors.User.ExternalIDP.MinimumExternalIDPNeeded")
	}
	for _, externalIDP := range externalIDPs {
		if !externalIDP.IsValid() {
			return caos_errs.ThrowPreconditionFailed(nil, "EVENT-idue3", "Errors.User.ExternalIDP.Invalid")
		}
	}
	existingUser, err := es.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if existingUser.Human == nil {
		return errors.ThrowPreconditionFailed(nil, "EVENT-Cnk8s", "Errors.User.NotHuman")
	}
	repoUser := model.UserFromModel(existingUser)
	repoExternalIDPs := model.ExternalIDPsFromModel(externalIDPs)
	aggregates, err := ExternalIDPAddedAggregate(ctx, es.Eventstore.AggregateCreator(), repoUser, repoExternalIDPs...)
	if err != nil {
		return err
	}
	err = es_sdk.PushAggregates(ctx, es.PushAggregates, repoUser.AppendEvents, aggregates...)
	if err != nil {
		return err
	}

	es.userCache.cacheUser(repoUser)
	return nil
}

func (es *UserEventstore) PrepareRemoveExternalIDP(ctx context.Context, externalIDP *usr_model.ExternalIDP, cascade bool) (*model.User, []*es_models.Aggregate, error) {
	if externalIDP == nil || !externalIDP.IsValid() {
		return nil, nil, errors.ThrowPreconditionFailed(nil, "EVENT-Cm8sj", "Errors.User.ExternalIDP.Invalid")
	}
	existingUser, err := es.UserByID(ctx, externalIDP.AggregateID)
	if err != nil {
		return nil, nil, err
	}
	if existingUser.Human == nil {
		return nil, nil, errors.ThrowPreconditionFailed(nil, "EVENT-E8iod", "Errors.User.NotHuman")
	}
	_, existingIDP := existingUser.GetExternalIDP(externalIDP)
	if existingIDP == nil {
		return nil, nil, errors.ThrowPreconditionFailed(nil, "EVENT-3Dh7s", "Errors.User.ExternalIDP.NotOnUser")
	}
	repoUser := model.UserFromModel(existingUser)
	repoExternalIDP := model.ExternalIDPFromModel(externalIDP)
	agg, err := ExternalIDPRemovedAggregate(ctx, es.Eventstore.AggregateCreator(), repoUser, repoExternalIDP, cascade)
	if err != nil {
		return nil, nil, err
	}
	return repoUser, agg, err
}

func (es *UserEventstore) RemoveExternalIDP(ctx context.Context, externalIDP *usr_model.ExternalIDP) error {
	repoUser, aggregates, err := es.PrepareRemoveExternalIDP(ctx, externalIDP, false)
	if err != nil {
		return err
	}
	err = es_sdk.PushAggregates(ctx, es.PushAggregates, repoUser.AppendEvents, aggregates...)
	if err != nil {
		return err
	}
	es.userCache.cacheUser(repoUser)
	return nil
}

func (es *UserEventstore) ProfileByID(ctx context.Context, userID string) (*usr_model.Profile, error) {
	if userID == "" {
		return nil, caos_errs.ThrowPreconditionFailed(nil, "EVENT-di834", "Errors.User.UserIDMissing")
	}
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Human == nil {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-BaE4M", "Errors.User.NotHuman")
	}

	if user.Profile != nil {
		return user.Profile, nil
	}
	return nil, caos_errs.ThrowNotFound(nil, "EVENT-dk23f", "Errors.User.ProfileNotFound")
}

func (es *UserEventstore) ChangeProfile(ctx context.Context, profile *usr_model.Profile) (*usr_model.Profile, error) {
	profile.SetNamesAsDisplayname()
	if !profile.IsValid() {
		return nil, caos_errs.ThrowPreconditionFailed(nil, "EVENT-d82i3", "Errors.User.ProfileInvalid")
	}
	user, err := es.UserByID(ctx, profile.AggregateID)
	if err != nil {
		return nil, err
	}
	if user.Human == nil {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-Xhw8Y", "Errors.User.NotHuman")
	}

	repoUser := model.UserFromModel(user)
	repoProfile := model.ProfileFromModel(profile)

	updateAggregate := ProfileChangeAggregate(es.AggregateCreator(), repoUser, repoProfile)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, updateAggregate)
	if err != nil {
		return nil, err
	}

	es.userCache.cacheUser(repoUser)
	return model.ProfileToModel(repoUser.Profile), nil
}

func (es *UserEventstore) EmailByID(ctx context.Context, userID string) (*usr_model.Email, error) {
	if userID == "" {
		return nil, caos_errs.ThrowPreconditionFailed(nil, "EVENT-di834", "Errors.User.UserIDMissing")
	}
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Human == nil {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-zHtOg", "Errors.User.NotHuman")
	}

	if user.Email != nil {
		return user.Email, nil
	}
	return nil, caos_errs.ThrowNotFound(nil, "EVENT-dki89", "Errors.User.EmailNotFound")
}

func (es *UserEventstore) ChangeEmail(ctx context.Context, email *usr_model.Email) (*usr_model.Email, error) {
	if !email.IsValid() {
		return nil, caos_errs.ThrowPreconditionFailed(nil, "EVENT-lco09", "Errors.User.EmailInvalid")
	}
	user, err := es.UserByID(ctx, email.AggregateID)
	if err != nil {
		return nil, err
	}
	if user.Human == nil {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-tgBdL", "Errors.User.NotHuman")
	}

	emailCode, err := email.GenerateEmailCodeIfNeeded(es.EmailVerificationCode)
	if err != nil {
		return nil, err
	}

	repoUser := model.UserFromModel(user)
	repoEmail := model.EmailFromModel(email)
	repoEmailCode := model.EmailCodeFromModel(emailCode)

	updateAggregate, err := EmailChangeAggregate(ctx, es.AggregateCreator(), repoUser, repoEmail, repoEmailCode)
	if err != nil {
		return nil, err
	}
	err = es_sdk.PushAggregates(ctx, es.PushAggregates, repoUser.AppendEvents, updateAggregate)
	if err != nil {
		return nil, err
	}

	es.userCache.cacheUser(repoUser)
	return model.EmailToModel(repoUser.Email), nil
}

func (es *UserEventstore) VerifyEmail(ctx context.Context, userID, verificationCode string) error {
	if userID == "" {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-lo9fd", "Errors.User.UserIDMissing")
	}
	if verificationCode == "" {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-skDws", "Errors.User.Code.Empty")
	}
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Human == nil {
		return errors.ThrowPreconditionFailed(nil, "EVENT-YgXu6", "Errors.User.NotHuman")
	}
	if user.EmailCode == nil {
		return caos_errs.ThrowNotFound(nil, "EVENT-lso9w", "Errors.User.Code.NotFound")
	}

	err = crypto.VerifyCode(user.EmailCode.CreationDate, user.EmailCode.Expiry, user.EmailCode.Code, verificationCode, es.EmailVerificationCode)
	if err == nil {
		return es.setEmailVerifyResult(ctx, user, EmailVerifiedAggregate)
	}
	if err := es.setEmailVerifyResult(ctx, user, EmailVerificationFailedAggregate); err != nil {
		return err
	}
	return caos_errs.ThrowInvalidArgument(err, "EVENT-dtGaa", "Errors.User.Code.Invalid")
}

func (es *UserEventstore) setEmailVerifyResult(ctx context.Context, user *usr_model.User, check func(aggCreator *es_models.AggregateCreator, user *model.User) es_sdk.AggregateFunc) error {
	repoUser := model.UserFromModel(user)
	err := es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, check(es.AggregateCreator(), repoUser))
	if err != nil {
		return err
	}
	es.userCache.cacheUser(repoUser)
	return nil
}

func (es *UserEventstore) CreateEmailVerificationCode(ctx context.Context, userID string) error {
	if userID == "" {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-lco09", "Errors.User.UserIDMissing")
	}
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Human == nil {
		return errors.ThrowPreconditionFailed(nil, "EVENT-hqUZP", "Errors.User.NotHuman")
	}
	if user.Email == nil {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-pdo9s", "Errors.User.EmailNotFound")
	}
	if user.IsEmailVerified {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-pdo9s", "Errors.User.EmailAlreadyVerified")
	}

	emailCode := new(usr_model.EmailCode)
	err = emailCode.GenerateEmailCode(es.EmailVerificationCode)
	if err != nil {
		return err
	}

	repoUser := model.UserFromModel(user)
	repoEmailCode := model.EmailCodeFromModel(emailCode)
	updateAggregate := EmailVerificationCodeAggregate(es.AggregateCreator(), repoUser, repoEmailCode)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, updateAggregate)
	if err != nil {
		return err
	}

	es.userCache.cacheUser(repoUser)
	return nil
}

func (es *UserEventstore) EmailVerificationCodeSent(ctx context.Context, userID string) error {
	if userID == "" {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-spo0w", "Errors.User.UserIDMissing")
	}
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Human == nil {
		return errors.ThrowPreconditionFailed(nil, "EVENT-BcFVd", "Errors.User.NotHuman")
	}

	repoUser := model.UserFromModel(user)
	agg := EmailCodeSentAggregate(es.AggregateCreator(), repoUser)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, agg)
	if err != nil {
		return err
	}
	es.userCache.cacheUser(repoUser)
	return nil
}

func (es *UserEventstore) PhoneByID(ctx context.Context, userID string) (*usr_model.Phone, error) {
	if userID == "" {
		return nil, caos_errs.ThrowPreconditionFailed(nil, "EVENT-do9se", "Errors.User.UserIDMissing")
	}
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Human == nil {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-LwQeA", "Errors.User.NotHuman")
	}

	if user.Phone != nil {
		return user.Phone, nil
	}
	return nil, caos_errs.ThrowNotFound(nil, "EVENT-pos9e", "Errors.User.PhoneNotFound")
}

func (es *UserEventstore) ChangePhone(ctx context.Context, phone *usr_model.Phone) (*usr_model.Phone, error) {
	if !phone.IsValid() {
		return nil, caos_errs.ThrowPreconditionFailed(nil, "EVENT-do9s4", "Errors.User.PhoneInvalid")
	}
	user, err := es.UserByID(ctx, phone.AggregateID)
	if err != nil {
		return nil, err
	}
	if user.Human == nil {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-oREkn", "Errors.User.NotHuman")
	}

	phoneCode, err := phone.GeneratePhoneCodeIfNeeded(es.PhoneVerificationCode)
	if err != nil {
		return nil, err
	}

	repoUser := model.UserFromModel(user)
	repoPhone := model.PhoneFromModel(phone)
	repoPhoneCode := model.PhoneCodeFromModel(phoneCode)

	updateAggregate := PhoneChangeAggregate(es.AggregateCreator(), repoUser, repoPhone, repoPhoneCode)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, updateAggregate)
	if err != nil {
		return nil, err
	}

	es.userCache.cacheUser(repoUser)
	return model.PhoneToModel(repoUser.Phone), nil
}

func (es *UserEventstore) VerifyPhone(ctx context.Context, userID, verificationCode string) error {
	if userID == "" || verificationCode == "" {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-dsi8s", "Errors.User.UserIDMissing")
	}
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Human == nil {
		return errors.ThrowPreconditionFailed(nil, "EVENT-UspdK", "Errors.User.NotHuman")
	}
	if user.PhoneCode == nil {
		return caos_errs.ThrowNotFound(nil, "EVENT-slp0s", "Errors.User.Code.NotFound")
	}

	err = crypto.VerifyCode(user.PhoneCode.CreationDate, user.PhoneCode.Expiry, user.PhoneCode.Code, verificationCode, es.PhoneVerificationCode)
	if err == nil {
		return es.setPhoneVerifyResult(ctx, user, PhoneVerifiedAggregate)
	}
	if err := es.setPhoneVerifyResult(ctx, user, PhoneVerificationFailedAggregate); err != nil {
		return err
	}
	return caos_errs.ThrowInvalidArgument(err, "EVENT-dsf4G", "Errors.User.Code.Invalid")
}

func (es *UserEventstore) setPhoneVerifyResult(ctx context.Context, user *usr_model.User, check func(aggCreator *es_models.AggregateCreator, user *model.User) es_sdk.AggregateFunc) error {
	repoUser := model.UserFromModel(user)
	err := es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, check(es.AggregateCreator(), repoUser))
	if err != nil {
		return err
	}
	es.userCache.cacheUser(repoUser)
	return nil
}

func (es *UserEventstore) CreatePhoneVerificationCode(ctx context.Context, userID string) error {
	if userID == "" {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-do9sw", "Errors.User.UserIDMissing")
	}
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Human == nil {
		return errors.ThrowPreconditionFailed(nil, "EVENT-eEi05", "Errors.User.NotHuman")
	}
	if user.Phone == nil {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-sp9fs", "Errors.User.PhoneNotFound")
	}
	if user.IsPhoneVerified {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-sleis", "Errors.User.PhoneAlreadyVerified")
	}

	phoneCode := new(usr_model.PhoneCode)
	err = phoneCode.GeneratePhoneCode(es.PhoneVerificationCode)
	if err != nil {
		return err
	}

	repoUser := model.UserFromModel(user)
	repoPhoneCode := model.PhoneCodeFromModel(phoneCode)
	updateAggregate := PhoneVerificationCodeAggregate(es.AggregateCreator(), repoUser, repoPhoneCode)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, updateAggregate)
	if err != nil {
		return err
	}

	es.userCache.cacheUser(repoUser)
	return nil
}

func (es *UserEventstore) PhoneVerificationCodeSent(ctx context.Context, userID string) error {
	if userID == "" {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-sp0wa", "Errors.User.UserIDMissing")
	}
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Human == nil {
		return errors.ThrowPreconditionFailed(nil, "EVENT-5bhOP", "Errors.User.NotHuman")
	}

	repoUser := model.UserFromModel(user)
	agg := PhoneCodeSentAggregate(es.AggregateCreator(), repoUser)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, agg)
	if err != nil {
		return err
	}
	es.userCache.cacheUser(repoUser)
	return nil
}

func (es *UserEventstore) RemovePhone(ctx context.Context, userID string) error {
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Human == nil {
		return errors.ThrowPreconditionFailed(nil, "EVENT-Satfl", "Errors.User.NotHuman")
	}
	repoUser := model.UserFromModel(user)
	removeAggregate := PhoneRemovedAggregate(es.AggregateCreator(), repoUser)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, removeAggregate)
	if err != nil {
		return err
	}

	es.userCache.cacheUser(repoUser)
	return nil
}

func (es *UserEventstore) AddressByID(ctx context.Context, userID string) (*usr_model.Address, error) {
	if userID == "" {
		return nil, caos_errs.ThrowPreconditionFailed(nil, "EVENT-di8ws", "Errors.User.UserIDMissing")
	}
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Human == nil {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-pHrLu", "Errors.User.NotHuman")
	}

	if user.Address != nil {
		return user.Address, nil
	}
	return nil, caos_errs.ThrowNotFound(nil, "EVENT-so9wa", "Errors.User.AddressNotFound")
}

func (es *UserEventstore) ChangeAddress(ctx context.Context, address *usr_model.Address) (*usr_model.Address, error) {
	user, err := es.UserByID(ctx, address.AggregateID)
	if err != nil {
		return nil, err
	}
	if user.Human == nil {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-crpHD", "Errors.User.NotHuman")
	}
	repoUser := model.UserFromModel(user)
	repoAddress := model.AddressFromModel(address)

	updateAggregate := AddressChangeAggregate(es.AggregateCreator(), repoUser, repoAddress)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, updateAggregate)
	if err != nil {
		return nil, err
	}

	es.userCache.cacheUser(repoUser)
	return model.AddressToModel(repoUser.Address), nil
}

func (es *UserEventstore) AddOTP(ctx context.Context, userID, accountName string) (*usr_model.OTP, error) {
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Human == nil {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-XJvu3", "Errors.User.NotHuman")
	}
	if user.IsOTPReady() {
		return nil, caos_errs.ThrowAlreadyExists(nil, "EVENT-do9se", "Errors.User.Mfa.Otp.AlreadyReady")
	}
	if accountName == "" {
		accountName = user.UserName
		if user.Email != nil {
			accountName = user.EmailAddress
		}
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: es.Multifactors.OTP.Issuer, AccountName: accountName})
	if err != nil {
		return nil, err
	}
	encryptedSecret, err := crypto.Encrypt([]byte(key.Secret()), es.Multifactors.OTP.CryptoMFA)
	if err != nil {
		return nil, err
	}
	repoOTP := &model.OTP{Secret: encryptedSecret}
	repoUser := model.UserFromModel(user)
	updateAggregate := MFAOTPAddAggregate(es.AggregateCreator(), repoUser, repoOTP)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, updateAggregate)
	if err != nil {
		return nil, err
	}

	es.userCache.cacheUser(repoUser)
	otp := model.OTPToModel(repoUser.OTP)
	otp.Url = key.URL()
	otp.SecretString = key.Secret()
	return otp, nil
}

func (es *UserEventstore) RemoveOTP(ctx context.Context, userID string) error {
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Human == nil {
		return errors.ThrowPreconditionFailed(nil, "EVENT-WsBv9", "Errors.User.NotHuman")
	}
	if user.OTP == nil {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-sp0de", "Errors.User.Mfa.Otp.NotExisting")
	}
	repoUser := model.UserFromModel(user)
	updateAggregate := MFAOTPRemoveAggregate(es.AggregateCreator(), repoUser)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, updateAggregate)
	if err != nil {
		return err
	}

	es.userCache.cacheUser(repoUser)
	return nil
}

func (es *UserEventstore) CheckMfaOTPSetup(ctx context.Context, userID, code string) error {
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Human == nil {
		return errors.ThrowPreconditionFailed(nil, "EVENT-7zRQM", "Errors.User.NotHuman")
	}
	if user.OTP == nil {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-yERHV", "Errors.Users.Mfa.Otp.NotExisting")
	}
	if user.IsOTPReady() {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-qx4ls", "Errors.Users.Mfa.Otp.AlreadyReady")
	}
	if err := es.verifyMfaOTP(user.OTP, code); err != nil {
		return err
	}
	repoUser := model.UserFromModel(user)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, MFAOTPVerifyAggregate(es.AggregateCreator(), repoUser))
	if err != nil {
		return err
	}

	es.userCache.cacheUser(repoUser)
	return nil
}

func (es *UserEventstore) CheckMfaOTP(ctx context.Context, userID, code string, authRequest *req_model.AuthRequest) error {
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Human == nil {
		return errors.ThrowPreconditionFailed(nil, "EVENT-ckqn5", "Errors.User.NotHuman")
	}
	if !user.IsOTPReady() {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-sd5NJ", "Errors.User.Mfa.Otp.NotReady")
	}

	repoUser := model.UserFromModel(user)
	repoAuthReq := model.AuthRequestFromModel(authRequest)
	var aggregate func(*es_models.AggregateCreator, *model.User, *model.AuthRequest) es_sdk.AggregateFunc
	var checkErr error
	if checkErr = es.verifyMfaOTP(user.OTP, code); checkErr != nil {
		aggregate = MFAOTPCheckFailedAggregate
	} else {
		aggregate = MFAOTPCheckSucceededAggregate
	}
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, aggregate(es.AggregateCreator(), repoUser, repoAuthReq))
	if checkErr != nil {
		return checkErr
	}
	if err != nil {
		return err
	}

	es.userCache.cacheUser(repoUser)
	return nil
}

func (es *UserEventstore) verifyMfaOTP(otp *usr_model.OTP, code string) error {
	decrypt, err := crypto.DecryptString(otp.Secret, es.Multifactors.OTP.CryptoMFA)
	if err != nil {
		return err
	}

	valid := es.validateTOTP(code, decrypt)
	if !valid {
		return caos_errs.ThrowInvalidArgument(nil, "EVENT-8isk2", "Errors.User.Mfa.Otp.InvalidCode")
	}
	return nil
}

func (es *UserEventstore) SignOut(ctx context.Context, agentID string, userIDs []string) error {
	users := make([]*model.User, len(userIDs))
	for i, id := range userIDs {
		user, err := es.UserByID(ctx, id)
		if err != nil {
			return err
		}
		users[i] = model.UserFromModel(user)
	}

	aggFunc := SignOutAggregates(es.AggregateCreator(), users, agentID)
	aggregates, err := aggFunc(ctx)
	if err != nil {
		return err
	}
	err = es_sdk.PushAggregates(ctx, es.PushAggregates, nil, aggregates...)
	if err != nil {
		return err
	}
	return nil
}

func (es *UserEventstore) PrepareDomainClaimed(ctx context.Context, userIDs []string) ([]*es_models.Aggregate, error) {
	aggregates := make([]*es_models.Aggregate, 0)
	for _, userID := range userIDs {
		user, err := es.UserByID(ctx, userID)
		if err != nil {
			return nil, err
		}
		repoUser := model.UserFromModel(user)
		name, err := es.generateTemporaryLoginName()
		if err != nil {
			return nil, err
		}
		userAgg, err := DomainClaimedAggregate(ctx, es.AggregateCreator(), repoUser, name)
		if err != nil {
			return nil, err
		}
		aggregates = append(aggregates, userAgg...)
	}
	return aggregates, nil
}

func (es *UserEventstore) DomainClaimedSent(ctx context.Context, userID string) error {
	if userID == "" {
		return caos_errs.ThrowPreconditionFailed(nil, "EVENT-0posw", "Errors.User.UserIDMissing")
	}
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return err
	}

	repoUser := model.UserFromModel(user)
	agg := DomainClaimedSentAggregate(es.AggregateCreator(), repoUser)
	err = es_sdk.Push(ctx, es.PushAggregates, repoUser.AppendEvents, agg)
	if err != nil {
		return err
	}
	es.userCache.cacheUser(repoUser)
	return nil
}

func (es *UserEventstore) ChangeUsername(ctx context.Context, userID, username string, orgIamPolicy *org_model.OrgIAMPolicy) error {
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	oldUsername := user.UserName
	user.UserName = username
	if err := user.CheckOrgIAMPolicy(orgIamPolicy); err != nil {
		return err
	}
	repoUser := model.UserFromModel(user)
	aggregates, err := UsernameChangedAggregates(ctx, es.AggregateCreator(), repoUser, oldUsername, orgIamPolicy.UserLoginMustBeDomain)
	if err != nil {
		return err
	}
	err = es_sdk.PushAggregates(ctx, es.PushAggregates, repoUser.AppendEvents, aggregates...)
	if err != nil {
		return err
	}
	es.userCache.cacheUser(repoUser)
	return nil
}

func (es *UserEventstore) generateTemporaryLoginName() (string, error) {
	id, err := es.idGenerator.Next()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s@temporary.%s", id, es.domain), nil
}

func (es *UserEventstore) AddMachineKey(ctx context.Context, key *usr_model.MachineKey) (*usr_model.MachineKey, error) {
	user, err := es.UserByID(ctx, key.AggregateID)
	if err != nil {
		return nil, err
	}
	if user.Machine == nil {
		return nil, errors.ThrowPreconditionFailed(nil, "EVENT-5ROh4", "Errors.User.NotMachine")
	}

	id, err := es.idGenerator.Next()
	if err != nil {
		return nil, err
	}
	key.KeyID = id

	if key.ExpirationDate.IsZero() {
		key.ExpirationDate, err = time.Parse(yearLayout, defaultExpirationDate)
		if err != nil {
			logging.Log("EVENT-vzibi").WithError(err).Warn("unable to set default date")
			return nil, errors.ThrowInternal(err, "EVENT-j68fg", "Errors.Internal")
		}
	}
	if key.ExpirationDate.Before(time.Now()) {
		return nil, errors.ThrowInvalidArgument(nil, "EVENT-C6YV5", "Errors.MachineKey.ExpireBeforeNow")
	}

	repoUser := model.UserFromModel(user)
	repoKey := model.MachineKeyFromModel(key)
	err = repoKey.GenerateMachineKeyPair(es.MachineKeySize, es.MachineKeyAlg)
	if err != nil {
		return nil, err
	}

	userAggregate, err := UserAggregate(ctx, es.AggregateCreator(), repoUser)
	if err != nil {
		return nil, err
	}
	keyAggregate, err := userAggregate.AppendEvent(model.MachineKeyAdded, repoKey)
	if err != nil {
		return nil, err
	}
	err = es_sdk.PushAggregates(ctx, es.PushAggregates, repoKey.AppendEvents, keyAggregate)
	if err != nil {
		return nil, err
	}

	return model.MachineKeyToModel(repoKey), nil
}

func (es *UserEventstore) RemoveMachineKey(ctx context.Context, userID, keyID string) error {
	user, err := es.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Machine == nil {
		return errors.ThrowPreconditionFailed(nil, "EVENT-h5Qtd", "Errors.User.NotMachine")
	}

	repoUser := model.UserFromModel(user)
	userAggregate, err := UserAggregate(ctx, es.AggregateCreator(), repoUser)
	if err != nil {
		return err
	}

	keyIDPayload := struct {
		KeyID string `json:"keyId"`
	}{KeyID: keyID}

	keyAggregate, err := userAggregate.AppendEvent(model.MachineKeyRemoved, &keyIDPayload)
	if err != nil {
		return err
	}
	return es.PushAggregates(ctx, keyAggregate)
}
