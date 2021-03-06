package model

import (
	"testing"
)

func TestCheckPasswordComplexityPolicy(t *testing.T) {
	type args struct {
		policy   *PasswordComplexityPolicy
		password string
	}
	tests := []struct {
		name     string
		args     args
		hasError bool
	}{
		{
			name: "has minlength ok",
			args: args{
				policy: &PasswordComplexityPolicy{
					HasLowercase: false,
					HasUppercase: false,
					HasSymbol:    false,
					HasNumber:    false,
					MinLength:    10,
				},
				password: "password12",
			},
			hasError: false,
		},
		{
			name: "has minlength not ok",
			args: args{
				policy: &PasswordComplexityPolicy{
					HasLowercase: false,
					HasUppercase: false,
					HasSymbol:    false,
					HasNumber:    false,
					MinLength:    10,
				},
				password: "password",
			},
			hasError: true,
		},
		{
			name: "has lowercase ok",
			args: args{
				policy: &PasswordComplexityPolicy{
					HasLowercase: true,
					HasUppercase: false,
					HasSymbol:    false,
					HasNumber:    false,
					MinLength:    0,
				},
				password: "password",
			},
			hasError: false,
		},
		{
			name: "has lowercase not ok",
			args: args{
				policy: &PasswordComplexityPolicy{
					HasLowercase: true,
					HasUppercase: false,
					HasSymbol:    false,
					HasNumber:    false,
					MinLength:    0,
				},
				password: "PASSWORD",
			},
			hasError: true,
		},
		{
			name: "has uppercase ok",
			args: args{
				policy: &PasswordComplexityPolicy{
					HasLowercase: false,
					HasUppercase: true,
					HasSymbol:    false,
					HasNumber:    false,
					MinLength:    0,
				},
				password: "PASSWORD",
			},
			hasError: false,
		},
		{
			name: "has uppercase not ok",
			args: args{
				policy: &PasswordComplexityPolicy{
					HasLowercase: false,
					HasUppercase: true,
					HasSymbol:    false,
					HasNumber:    false,
					MinLength:    0,
				},
				password: "password",
			},
			hasError: true,
		},
		{
			name: "has symbol ok",
			args: args{
				policy: &PasswordComplexityPolicy{
					HasLowercase: false,
					HasUppercase: false,
					HasSymbol:    true,
					HasNumber:    false,
					MinLength:    0,
				},
				password: "!G$",
			},
			hasError: false,
		},
		{
			name: "has symbol not ok",
			args: args{
				policy: &PasswordComplexityPolicy{
					HasLowercase: false,
					HasUppercase: false,
					HasSymbol:    true,
					HasNumber:    false,
					MinLength:    0,
				},
				password: "PASSWORD",
			},
			hasError: true,
		},
		{
			name: "has number ok",
			args: args{
				policy: &PasswordComplexityPolicy{
					HasLowercase: false,
					HasUppercase: false,
					HasSymbol:    false,
					HasNumber:    true,
					MinLength:    0,
				},
				password: "123456",
			},
			hasError: false,
		},
		{
			name: "has number not ok",
			args: args{
				policy: &PasswordComplexityPolicy{
					HasLowercase: false,
					HasUppercase: false,
					HasSymbol:    false,
					HasNumber:    true,
					MinLength:    0,
				},
				password: "PASSWORD",
			},
			hasError: true,
		},
		{
			name: "has everything ok",
			args: args{
				policy: &PasswordComplexityPolicy{
					HasLowercase: true,
					HasUppercase: true,
					HasSymbol:    true,
					HasNumber:    true,
					MinLength:    10,
				},
				password: "Password1!",
			},
			hasError: false,
		},
		{
			name: "has everything not ok",
			args: args{
				policy: &PasswordComplexityPolicy{
					HasLowercase: true,
					HasUppercase: true,
					HasSymbol:    true,
					HasNumber:    true,
					MinLength:    10,
				},
				password: "password",
			},
			hasError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.args.policy.Check(tt.args.password)
			if !tt.hasError && err != nil {
				t.Errorf("should not get err: %v", err)
			}
			if tt.hasError && err == nil {
				t.Errorf("should have error: %v", err)
			}
		})
	}
}
