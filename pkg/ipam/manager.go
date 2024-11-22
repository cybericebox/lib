package ipam

import (
	"context"
	"encoding/binary"
	"errors"
	"github.com/cybericebox/lib/pkg/appError"
	goipam "github.com/metal-stack/go-ipam"
	"net"
	"net/netip"
)

type (
	IPAManager struct {
		ipaManager goipam.Ipamer
		cidr       string
	}

	PostgresConfig struct {
		Host     string
		Port     string
		Username string
		Password string
		Database string
		SSLMode  string
	}

	Dependencies struct {
		PostgresConfig PostgresConfig
		CIDR           string
	}
)

func NewIPAManager(deps Dependencies) (*IPAManager, error) {
	storage, err := goipam.NewPostgresStorage(
		deps.PostgresConfig.Host,
		deps.PostgresConfig.Port,
		deps.PostgresConfig.Username,
		deps.PostgresConfig.Password,
		deps.PostgresConfig.Database,
		goipam.SSLMode(deps.PostgresConfig.SSLMode),
	)

	if err != nil {
		return nil, appError.ErrIPAM.WithError(err).WithMessage("Failed to create new postgres storage").Err()
	}
	ctx := context.Background()

	ipaManager := goipam.NewWithStorage(storage)

	if deps.CIDR == "" {
		return nil, appError.ErrIPAMCIDRRequired.Err()
	}

	_, err = ipaManager.PrefixFrom(ctx, deps.CIDR)

	if err != nil {
		if !errors.Is(err, goipam.ErrNotFound) {
			return nil, appError.ErrIPAM.WithError(err).WithMessage("Failed to get prefix").Err()
		}
		if _, err = ipaManager.NewPrefix(ctx, deps.CIDR); err != nil {
			return nil, appError.ErrIPAM.WithError(err).WithMessage("Failed to create new prefix").Err()
		}
	}

	return &IPAManager{
		ipaManager: ipaManager,
		cidr:       deps.CIDR,
	}, nil
}

func (m *IPAManager) AcquireChildCIDR(ctx context.Context, blockSize uint32) (*IPAManager, error) {
	prefix, err := m.ipaManager.AcquireChildPrefix(ctx, m.cidr, uint8(blockSize))
	if err != nil {
		return nil, appError.ErrIPAM.WithError(err).WithMessage("Failed to acquire child prefix").Err()
	}
	return &IPAManager{
		ipaManager: m.ipaManager,
		cidr:       prefix.Cidr,
	}, nil
}

func (m *IPAManager) GetChildCIDR(ctx context.Context, cidr string) (*IPAManager, error) {
	prefix, err := m.ipaManager.PrefixFrom(ctx, cidr)
	if err != nil {
		return nil, appError.ErrIPAM.WithError(err).WithMessage("Failed to get prefix").Err()
	}
	return &IPAManager{
		ipaManager: m.ipaManager,
		cidr:       prefix.Cidr,
	}, nil
}

func (m *IPAManager) ReleaseChildCIDR(ctx context.Context, childCIDR string) error {
	// release all IPs in the CIDR
	prefix, err := m.ipaManager.PrefixFrom(ctx, childCIDR)
	if err != nil {
		return appError.ErrIPAM.WithError(err).WithMessage("Failed to get prefix").Err()
	}

	addr, netCIDR, err := net.ParseCIDR(prefix.Cidr)
	if err != nil {
		return appError.ErrIPAM.WithError(err).WithMessage("Failed to parse CIDR").Err()
	}

	ip := addr.String()

	for {
		if err = m.ipaManager.ReleaseIPFromPrefix(ctx, prefix.Cidr, ip); err != nil {
			if !errors.Is(err, goipam.ErrNotFound) {
				return appError.ErrIPAM.WithError(err).WithMessage("Failed to release IP").Err()
			}
		}
		// get next IP
		ip = netip.MustParseAddr(ip).Next().String()

		if !netCIDR.Contains(net.ParseIP(ip)) {
			break
		}
	}

	prefix, err = m.ipaManager.PrefixFrom(ctx, childCIDR)
	if err != nil {
		return appError.ErrIPAM.WithError(err).WithMessage("Failed to get prefix").Err()
	}

	if err = m.ipaManager.ReleaseChildPrefix(ctx, prefix); err != nil {
		return appError.ErrIPAM.WithError(err).WithMessage("Failed to release child prefix").Err()
	}

	return nil
}

func (m *IPAManager) AcquireSingleIP(ctx context.Context, specificIP ...string) (string, error) {
	if len(specificIP) > 0 {
		_, err := m.ipaManager.AcquireSpecificIP(ctx, m.cidr, specificIP[0])
		if err != nil && !errors.Is(err, goipam.ErrAlreadyAllocated) {
			return "", appError.ErrIPAM.WithError(err).WithMessage("Failed to acquire specific IP").Err()
		}
		return specificIP[0], nil
	}

	ip, err := m.ipaManager.AcquireIP(ctx, m.cidr)
	if err != nil {
		return "", appError.ErrIPAM.WithError(err).WithMessage("Failed to acquire IP").Err()
	}
	return ip.IP.String(), nil
}

func (m *IPAManager) ReleaseSingleIP(ctx context.Context, ip string) error {
	if err := m.ipaManager.ReleaseIPFromPrefix(ctx, m.cidr, ip); err != nil {
		return appError.ErrIPAM.WithError(err).WithMessage("Failed to release IP").Err()
	}

	return nil
}

func (m *IPAManager) GetFirstIP() (string, error) {
	ip, err := GetFirstCIDRIP(m.cidr)
	if err != nil {
		return "", appError.ErrIPAM.WithError(err).WithMessage("Failed to get first IP").Err()
	}

	return ip, nil
}

func (m *IPAManager) GetCIDR() string {
	return m.cidr
}

func GetFirstCIDRIP(cidr string) (string, error) {
	_, parsedCIDR, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", appError.ErrIPAM.WithError(err).WithMessage("Failed to parse CIDR").Err()

	}
	return int2ip(ip2int(parsedCIDR.IP) + uint32(1)).String(), nil
}

func ip2int(ip net.IP) uint32 {
	return binary.BigEndian.Uint32(ip)
}

func int2ip(nn uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, nn)
	return ip
}
