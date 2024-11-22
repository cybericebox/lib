package appError

import "github.com/cybericebox/lib/pkg/err"

var (
	ErrIPAM             = err.ErrInternal.WithObjectCode(ipamObjectCode)
	ErrIPAMCIDRRequired = err.ErrInvalidData.WithObjectCode(ipamObjectCode).WithMessage("CIDR is required").WithDetailCode(1)
)
