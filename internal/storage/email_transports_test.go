package storage

import (
	"reflect"
	"testing"

	"github.com/nodelistdb/internal/database"
)

func TestOtherTransports(t *testing.T) {
	ibn := &database.InternetConfiguration{
		Protocols: database.ProtocolMap{"IBN": {{Port: 24554}}, "INA": {{}}},
	}
	inaOnly := &database.InternetConfiguration{
		Protocols: database.ProtocolMap{"INA": {{}}},
	}

	for _, tc := range []struct {
		name  string
		flags []string
		ic    *database.InternetConfiguration
		phone string
		want  []string
	}{
		{"email only", []string{"CM"}, nil, "-Unpublished-", nil},
		// INA is an address, not a way in.
		{"INA is not a transport", nil, inaOnly, "-Unpublished-", nil},
		{"IBN from internet_config", nil, ibn, "-Unpublished-", []string{"IBN"}},
		// Older rows kept some flags verbatim in the flags array.
		{"flags array, bare and addressed", []string{"ITN", "IFT:ftp.example"}, nil, "", []string{"ITN", "IFT"}},
		{"dialable phone", nil, nil, "7-495-123-4567", []string{"PSTN"}},
		{"both, IP flags first", nil, ibn, "49-40-98670308", []string{"IBN", "PSTN"}},
		{"zero placeholder", nil, nil, "000-000-000-000", nil},
		{"short zero placeholder", nil, nil, "0-0-0-0", nil},
		{"empty phone", nil, nil, "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := otherTransports(tc.flags, tc.ic, tc.phone)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("otherTransports = %v, want %v", got, tc.want)
			}
		})
	}
}
