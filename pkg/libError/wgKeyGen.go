package libError

import "github.com/cybericebox/lib/pkg/err"

var (
	ErrWgKeyGen = err.ErrInternal.WithObjectCode(wgKeyGenObjectCode)
)
