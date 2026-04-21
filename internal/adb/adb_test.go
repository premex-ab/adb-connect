package adb_test

import (
	"context"
	"testing"

	"github.com/premex-ab/adb-connect/internal/adb"
	"github.com/premex-ab/adb-connect/internal/testutil"
)

func TestPairSuccess(t *testing.T) {
	testutil.FakeBinary(t, "adb", `echo "Successfully paired"`)
	r, err := adb.Pair(context.Background(), "1.2.3.4", 5555, "123456")
	if err != nil {
		t.Fatal(err)
	}
	if !r.OK || !contains(r.Stdout, "Successfully paired") {
		t.Fatalf("got %+v", r)
	}
}

func TestDevicesParsesList(t *testing.T) {
	testutil.FakeBinary(t, "adb", `printf "List of devices attached\nemulator-5554\tdevice\n1.2.3.4:5555\tdevice\n"`)
	ds, err := adb.Devices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 2 || ds[0].Serial != "emulator-5554" || ds[1].Serial != "1.2.3.4:5555" {
		t.Fatalf("got %+v", ds)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
