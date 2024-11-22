package appError

import "github.com/cybericebox/lib/pkg/err"

var (
	ErrToken                         = err.ErrInternal.WithObjectCode(tokenObjectCode)
	ErrTokenEmptySignature           = err.ErrInvalidData.WithObjectCode(tokenObjectCode).WithMessage("Token signature is empty").WithDetailCode(1)
	ErrTokenTTLMustBeGreaterThanZero = err.ErrInvalidData.WithObjectCode(tokenObjectCode).WithMessage("TTL must be greater than 0").WithDetailCode(2)
	ErrTokenInvalidJWTToken          = err.ErrInvalidData.WithObjectCode(tokenObjectCode).WithMessage("Invalid JWT Token").WithDetailCode(3)
)
