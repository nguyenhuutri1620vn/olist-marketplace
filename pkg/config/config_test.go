package config_test

import (
	"strings"
	"testing"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/config"
)

func TestRequireReturnsValues(t *testing.T) {
	t.Setenv("CFG_A", "a")
	t.Setenv("CFG_B", "b")

	got, err := config.Require("CFG_A", "CFG_B")
	if err != nil {
		t.Fatal(err)
	}
	if got["CFG_A"] != "a" || got["CFG_B"] != "b" {
		t.Errorf("Require = %v, want CFG_A=a CFG_B=b", got)
	}
}

func TestRequireNamesEveryMissingVariable(t *testing.T) {
	t.Setenv("CFG_SET", "x")
	t.Setenv("CFG_EMPTY", "")

	_, err := config.Require("CFG_SET", "CFG_EMPTY", "CFG_UNSET_FOR_TEST")
	if err == nil {
		t.Fatal("Require succeeded, want error for missing variables")
	}
	for _, name := range []string{"CFG_EMPTY", "CFG_UNSET_FOR_TEST"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not name %s", err, name)
		}
	}
	if strings.Contains(err.Error(), "CFG_SET") {
		t.Errorf("error %q names CFG_SET, which is set", err)
	}
}
