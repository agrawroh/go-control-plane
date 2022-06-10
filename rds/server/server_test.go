package server

import (
	"crypto/tls"
	"testing"

	"github.com/envoyproxy/go-control-plane/rds/env"
)

func TestStringToTLSVersion(t *testing.T) {
	expected := map[string]uint16{
		"TLSv1.0": tls.VersionTLS10,
		"TLSv1.1": tls.VersionTLS11,
		"TLSv1.2": tls.VersionTLS12,
		"TLSv1.3": tls.VersionTLS13,
		"":        0,
	}

	for k, v := range expected {
		actual, err := stringToTLSVersion(k)
		if err != nil {
			t.Errorf("expected %d for %s, got error %s", v, k, err)
		}
		if actual != v {
			t.Errorf("expected %d for %s, got %d", v, k, actual)
		}
	}

	_, err := stringToTLSVersion("invalid tls version")
	if err == nil {
		t.Errorf("expected error from invalid TLS versoin, got nil")
	}
}

func TestCreateTLSConfigPanic(t *testing.T) {
	tests := []struct {
		name     string
		settings *env.Settings
	}{
		{"Invalid min TLS version", &env.Settings{MinTLSVersion: "invalid_version"}},
		{"Invalid max TLS version", &env.Settings{MaxTLSVersion: "invalid_version"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() { recover() }()

			createTLSConfig(tt.settings)

			// createTLSConfig() should panic, code should never reach here.
			t.Errorf("Expected panic(), but didn't with test case \"%s\"", tt.name)
		})
	}
}

func TestCreateTLSConfigReturnNil(t *testing.T) {
	s := &env.Settings{
		MinTLSVersion: "",
		MaxTLSVersion: "TLSv1.1",
		ServerTLS:     false,
	}

	if createTLSConfig(s) != nil {
		t.Errorf("Expected nil, got something")
	}
}

func TestCreateTLSConfig(t *testing.T) {
	tests := []struct {
		name        string
		settings    *env.Settings
		nilCa       bool
		shouldPanic bool
		success     bool
	}{
		{
			name:        "Non-existent cert path",
			settings:    &env.Settings{ServerCaPath: "/non_existent_path"},
			nilCa:       false,
			shouldPanic: true,
			success:     false,
		},
		{
			name:        "Non-cert pem file",
			settings:    &env.Settings{ServerCaPath: "testdata/ca-key.pem"},
			nilCa:       false,
			shouldPanic: true,
			success:     false,
		},
		{
			name: "Non-existent server cert file",
			settings: &env.Settings{
				ServerCaPath:          "testdata/ca-cert.pem",
				ServerKeyPath:         "testdata/server-key.pem",
				ServerCertificatePath: "non_existent_path"},
			nilCa:       false,
			shouldPanic: true,
			success:     false,
		},
		{
			name: "Non-cert server pem file",
			settings: &env.Settings{
				ServerCaPath:          "testdata/ca-cert.pem",
				ServerKeyPath:         "testdata/server-key.pem",
				ServerCertificatePath: "testdata/server-key.pem"},
			nilCa:       false,
			shouldPanic: true,
			success:     false,
		},
		{
			name: "Success with client cert required",
			settings: &env.Settings{
				RequireClientCert:     true,
				ServerCaPath:          "testdata/ca-cert.pem",
				ServerCertificatePath: "testdata/server-cert.pem",
				ServerKeyPath:         "testdata/server-key.pem"},
			nilCa:       false,
			shouldPanic: false,
			success:     true,
		},
		{
			name: "Success without client cert required",
			settings: &env.Settings{
				RequireClientCert:     false,
				ServerCaPath:          "testdata/ca-cert.pem",
				ServerCertificatePath: "testdata/server-cert.pem",
				ServerKeyPath:         "testdata/server-key.pem"},
			nilCa:       false,
			shouldPanic: false,
			success:     true,
		},
		{
			name: "Success with nil CA",
			settings: &env.Settings{
				ServerCaPath:          "",
				ServerCertificatePath: "testdata/server-cert.pem",
				ServerKeyPath:         "testdata/server-key.pem"},
			nilCa:       true,
			shouldPanic: false,
			success:     true,
		},
	}
	for _, tt := range tests {
		tt.settings.MinTLSVersion = "TLSv1.1"
		tt.settings.MaxTLSVersion = "TLSv1.2"
		tt.settings.ServerTLS = true

		t.Run(tt.name, func(t *testing.T) {
			defer func() { recover() }()

			c := createTLSConfig(tt.settings)

			// createTLSConfig() should panic, code should never reach here.
			if tt.shouldPanic {
				t.Errorf("Expected panic(), but didn't with test case \"%s\"", tt.name)
			}
			if tt.success {
				if ev, _ := stringToTLSVersion(tt.settings.MinTLSVersion); c.MinVersion != ev {
					t.Errorf("Expected TLS MinVersion %d, got %d", ev, c.MinVersion)
				}
				if ev, _ := stringToTLSVersion(tt.settings.MaxTLSVersion); c.MaxVersion != ev {
					t.Errorf("Expected TLS MaxVersion %d, got %d", ev, c.MaxVersion)
				}
				if len(c.Certificates) == 0 {
					t.Errorf("Expected some certs, got none")
				}
				if tt.settings.RequireClientCert != (c.ClientAuth == tls.RequireAndVerifyClientCert) {
					t.Errorf("Expected ClientAuth %t, got %s", tt.settings.RequireClientCert, c.ClientAuth)
				}
				if tt.nilCa != (c.ClientCAs == nil) {
					if tt.nilCa {
						t.Errorf("Expected nil CA, got something")
					} else {
						t.Errorf("Expected non-nil CA, got nil")
					}
				}
			}
		})
	}
}
