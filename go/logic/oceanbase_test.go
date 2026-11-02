package logic

import (
	"testing"

	"github.com/github/gh-ost/go/base"
)

func TestOceanBaseMetadataInstrumentation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ob      bool
		cutover base.CutOver
		want    bool
	}{
		{"mysql atomic", false, base.CutOverAtomic, true},
		{"mysql two-step", false, base.CutOverTwoStep, true},
		{"oceanbase atomic", true, base.CutOverAtomic, false},
		{"oceanbase two-step", true, base.CutOverTwoStep, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := base.NewMigrationContext()
			ctx.OceanBase = tc.ob
			ctx.CutOverType = tc.cutover
			m := &Migrator{migrationContext: ctx}
			if got := m.requiresMetadataLockInstrumentation(); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestOceanBaseAllowOnMaster(t *testing.T) {
	for _, ob := range []bool{false, true} {
		for _, sameTarget := range []bool{false, true} {
			for _, allow := range []bool{false, true} {
				ctx := base.NewMigrationContext()
				ctx.OceanBase = ob
				ctx.AllowedRunningOnMaster = allow
				ctx.InspectorConnectionConfig.Key.Hostname = "inspector"
				ctx.ApplierConnectionConfig.Key.Hostname = "applier"
				if sameTarget {
					ctx.ApplierConnectionConfig.Key.Hostname = "inspector"
				}
				m := &Migrator{migrationContext: ctx}
				want := !ob && sameTarget && !allow
				if got := m.requiresAllowOnMaster(); got != want {
					t.Errorf("ob=%v sameTarget=%v allow=%v: got %v, want %v", ob, sameTarget, allow, got, want)
				}
			}
		}
	}
}
