package appError

import "github.com/cybericebox/lib/pkg/err"

var (
	ErrToken                     = err.ErrInternal.WithObjectCode(tokenObjectCode)
	ErrTokenEmptySignature       = err.ErrInvalidData.WithObjectCode(tokenObjectCode).WithMessage("Token signature is empty").WithDetailCode(1)
	ErrTokenEmptyIssuer          = err.ErrInvalidData.WithObjectCode(tokenObjectCode).WithMessage("Token issuer is empty").WithDetailCode(2)
	ErrTokenTTLMustBeNotNegative = err.ErrInvalidData.WithObjectCode(tokenObjectCode).WithMessage("Token TTL must be not negative").WithDetailCode(3)
	ErrTokenInvalidJWTToken      = err.ErrInvalidData.WithObjectCode(tokenObjectCode).WithMessage("Invalid JWT Token").WithDetailCode(4)
)
