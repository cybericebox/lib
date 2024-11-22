package appError

import "github.com/cybericebox/lib/pkg/err"

var (
	ErrPassword                    = err.ErrInternal.WithObjectCode(passwordObjectCode)
	ErrPasswordInvalidHashPassword = err.ErrInvalidData.WithObjectCode(passwordObjectCode).WithMessage("Invalid hash password").WithDetailCode(1)
	ErrorInvalidPasswordComplexity = err.ErrInvalidData.WithObjectCode(passwordObjectCode).WithMessage("Invalid password complexity").WithDetailCode(2)
)
