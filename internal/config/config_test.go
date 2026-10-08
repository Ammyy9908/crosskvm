package config

import (
	"errors"
	"testing"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
		errIs   error
	}{
		{
			name:    "valid default config",
			cfg:     DefaultConfig(),
			wantErr: false,
		},
		{
			name: "valid connect config",
			cfg: Config{
				Mode: ModeConnect,
				Addr: "192.168.1.50:4545",
			},
			wantErr: false,
		},
		{
			name: "invalid mode",
			cfg: Config{
				Mode: "unknown",
				Addr: ":4545",
			},
			wantErr: true,
			errIs:   ErrInvalidMode,
		},
		{
			name: "empty addr",
			cfg: Config{
				Mode: ModeListen,
				Addr: "   ",
			},
			wantErr: true,
			errIs:   ErrMissingAddr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.errIs != nil && !errors.Is(err, tt.errIs) {
				t.Errorf("Validate() error = %v, want error containing %v", err, tt.errIs)
			}
		})
	}
}
