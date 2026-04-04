package pki

import (
	"context"
	"fmt"

	vault "github.com/hashicorp/vault/api"
)

type IssuedCertificate struct {
	Certificate string
	PrivateKey  string
	SerialNumber string
}

type VaultPKI struct {
	client *vault.Client
	role   string
}

func NewVaultPKI(addr, token, role string) (*VaultPKI, error) {
	cfg := vault.DefaultConfig()
	cfg.Address = addr

	client, err := vault.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("pki: create vault client: %w", err)
	}
	client.SetToken(token)

	return &VaultPKI{client: client, role: role}, nil
}

func (v *VaultPKI) Issue(ctx context.Context, commonName string) (*IssuedCertificate, error) {
	secret, err := v.client.Logical().WriteWithContext(ctx,
		fmt.Sprintf("pki/issue/%s", v.role),
		map[string]any{"common_name": commonName},
	)
	if err != nil {
		return nil, fmt.Errorf("pki: issue cert: %w", err)
	}

	cert, ok := secret.Data["certificate"].(string)
	if !ok {
		return nil, fmt.Errorf("pki: missing certificate in response")
	}
	key, ok := secret.Data["private_key"].(string)
	if !ok {
		return nil, fmt.Errorf("pki: missing private_key in response")
	}
	sn, ok := secret.Data["serial_number"].(string)
	if !ok {
		return nil, fmt.Errorf("pki: missing serial_number in response")
	}

	return &IssuedCertificate{
		Certificate:  cert,
		PrivateKey:   key,
		SerialNumber: sn,
	}, nil
}

func (v *VaultPKI) FetchCACert(ctx context.Context) (string, error) {
	secret, err := v.client.Logical().ReadWithContext(ctx, "pki/cert/ca")
	if err != nil {
		return "", fmt.Errorf("pki: fetch ca cert: %w", err)
	}
	cert, ok := secret.Data["certificate"].(string)
	if !ok {
		return "", fmt.Errorf("pki: missing certificate in ca response")
	}
	return cert, nil
}

func (v *VaultPKI) Revoke(ctx context.Context, serialNumber string) error {
	_, err := v.client.Logical().WriteWithContext(ctx,
		"pki/revoke",
		map[string]any{"serial_number": serialNumber},
	)
	if err != nil {
		return fmt.Errorf("pki: revoke cert: %w", err)
	}
	return nil
}
