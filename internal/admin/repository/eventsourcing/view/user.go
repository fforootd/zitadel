package view

import (
	usr_model "github.com/caos/zitadel/internal/user/model"
	"github.com/caos/zitadel/internal/user/repository/view"
	"github.com/caos/zitadel/internal/user/repository/view/model"
	"github.com/caos/zitadel/internal/view/repository"
)

const (
	userTable = "adminapi.users"
)

func (v *View) UserByID(userID string) (*model.UserView, error) {
	return view.UserByID(v.Db, userTable, userID)
}

func (v *View) SearchUsers(request *usr_model.UserSearchRequest) ([]*model.UserView, uint64, error) {
	return view.SearchUsers(v.Db, userTable, request)
}

func (v *View) GetGlobalUserByLoginName(loginName string) (*model.UserView, error) {
	return view.GetGlobalUserByLoginName(v.Db, userTable, loginName)
}

func (v *View) UsersByOrgID(orgID string) ([]*model.UserView, error) {
	return view.UsersByOrgID(v.Db, userTable, orgID)
}

func (v *View) UserIDsByDomain(domain string) ([]string, error) {
	return view.UserIDsByDomain(v.Db, userTable, domain)
}

func (v *View) IsUserUnique(userName, email string) (bool, error) {
	return view.IsUserUnique(v.Db, userTable, userName, email)
}

func (v *View) UserMfas(userID string) ([]*usr_model.MultiFactor, error) {
	return view.UserMfas(v.Db, userTable, userID)
}

func (v *View) PutUsers(user []*model.UserView, sequence uint64) error {
	err := view.PutUsers(v.Db, userTable, user...)
	if err != nil {
		return err
	}
	return v.ProcessedUserSequence(sequence)
}

func (v *View) PutUser(user *model.UserView, sequence uint64) error {
	err := view.PutUser(v.Db, userTable, user)
	if err != nil {
		return err
	}
	if sequence != 0 {
		return v.ProcessedUserSequence(sequence)
	}
	return nil
}

func (v *View) DeleteUser(userID string, eventSequence uint64) error {
	err := view.DeleteUser(v.Db, userTable, userID)
	if err != nil {
		return nil
	}
	return v.ProcessedUserSequence(eventSequence)
}

func (v *View) GetLatestUserSequence() (*repository.CurrentSequence, error) {
	return v.latestSequence(userTable)
}

func (v *View) ProcessedUserSequence(eventSequence uint64) error {
	return v.saveCurrentSequence(userTable, eventSequence)
}

func (v *View) GetLatestUserFailedEvent(sequence uint64) (*repository.FailedEvent, error) {
	return v.latestFailedEvent(userTable, sequence)
}

func (v *View) ProcessedUserFailedEvent(failedEvent *repository.FailedEvent) error {
	return v.saveFailedEvent(failedEvent)
}
