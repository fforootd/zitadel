package management

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"

	"github.com/caos/zitadel/internal/errors"
	"github.com/caos/zitadel/pkg/grpc/management"
)

func (s *Server) CreatePasswordComplexityPolicy(ctx context.Context, policy *management.PasswordComplexityPolicyCreate) (*management.PasswordComplexityPolicy, error) {
	policyresp, err := s.policy.CreatePasswordComplexityPolicy(ctx, passwordComplexityPolicyCreateToModel(policy))
	if err != nil {
		return nil, err
	}

	return passwordComplexityPolicyFromModel(policyresp), nil
}

func (s *Server) GetPasswordComplexityPolicy(ctx context.Context, _ *empty.Empty) (*management.PasswordComplexityPolicy, error) {
	policy, err := s.policy.GetPasswordComplexityPolicy(ctx)
	if err != nil {
		return nil, err
	}

	return passwordComplexityPolicyFromModel(policy), nil
}

func (s *Server) GetDefaultPasswordComplexityPolicy(ctx context.Context, _ *empty.Empty) (*management.PasswordComplexityPolicy, error) {
	policy, err := s.policy.GetDefaultPasswordComplexityPolicy(ctx)
	if err != nil {
		return nil, err
	}

	return passwordComplexityPolicyFromModel(policy), nil
}

func (s *Server) UpdatePasswordComplexityPolicy(ctx context.Context, policy *management.PasswordComplexityPolicyUpdate) (*management.PasswordComplexityPolicy, error) {
	policyresp, err := s.policy.UpdatePasswordComplexityPolicy(ctx, passwordComplexityPolicyUpdateToModel(policy))
	if err != nil {
		return nil, err
	}

	return passwordComplexityPolicyFromModel(policyresp), nil
}

func (s *Server) DeletePasswordComplexityPolicy(ctx context.Context, ID *management.PasswordComplexityPolicyID) (*empty.Empty, error) {
	return nil, errors.ThrowUnimplemented(nil, "GRPC-skw3f", "Not implemented")
}

func (s *Server) CreatePasswordAgePolicy(ctx context.Context, policy *management.PasswordAgePolicyCreate) (*management.PasswordAgePolicy, error) {
	policyresp, err := s.policy.CreatePasswordAgePolicy(ctx, passwordAgePolicyCreateToModel(policy))
	if err != nil {
		return nil, err
	}

	return passwordAgePolicyFromModel(policyresp), nil
}

func (s *Server) GetPasswordAgePolicy(ctx context.Context, _ *empty.Empty) (*management.PasswordAgePolicy, error) {
	policy, err := s.policy.GetPasswordAgePolicy(ctx)
	if err != nil {
		return nil, err
	}

	return passwordAgePolicyFromModel(policy), nil
}

func (s *Server) UpdatePasswordAgePolicy(ctx context.Context, policy *management.PasswordAgePolicyUpdate) (*management.PasswordAgePolicy, error) {
	policyresp, err := s.policy.UpdatePasswordAgePolicy(ctx, passwordAgePolicyUpdateToModel(policy))
	if err != nil {
		return nil, err
	}

	return passwordAgePolicyFromModel(policyresp), nil
}

func (s *Server) DeletePasswordAgePolicy(ctx context.Context, ID *management.PasswordAgePolicyID) (*empty.Empty, error) {
	return nil, errors.ThrowUnimplemented(nil, "GRPC-plo67", "Not implemented")
}

func (s *Server) CreatePasswordLockoutPolicy(ctx context.Context, policy *management.PasswordLockoutPolicyCreate) (*management.PasswordLockoutPolicy, error) {
	policyresp, err := s.policy.CreatePasswordLockoutPolicy(ctx, passwordLockoutPolicyCreateToModel(policy))
	if err != nil {
		return nil, err
	}

	return passwordLockoutPolicyFromModel(policyresp), nil
}

func (s *Server) GetPasswordLockoutPolicy(ctx context.Context, _ *empty.Empty) (*management.PasswordLockoutPolicy, error) {
	policy, err := s.policy.GetPasswordLockoutPolicy(ctx)
	if err != nil {
		return nil, err
	}

	return passwordLockoutPolicyFromModel(policy), nil
}

func (s *Server) UpdatePasswordLockoutPolicy(ctx context.Context, policy *management.PasswordLockoutPolicyUpdate) (*management.PasswordLockoutPolicy, error) {
	policyresp, err := s.policy.UpdatePasswordLockoutPolicy(ctx, passwordLockoutPolicyUpdateToModel(policy))
	if err != nil {
		return nil, err
	}

	return passwordLockoutPolicyFromModel(policyresp), nil
}

func (s *Server) DeletePasswordLockoutPolicy(ctx context.Context, ID *management.PasswordLockoutPolicyID) (*empty.Empty, error) {
	return nil, errors.ThrowUnimplemented(nil, "GRPC-GHkd9", "Not implemented")
}
