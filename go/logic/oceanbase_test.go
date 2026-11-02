package logic

import (
	"github.com/github/gh-ost/go/base"
	"testing"
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
