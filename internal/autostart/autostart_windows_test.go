//go:build windows

package autostart

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestSetEnabledRoundTrip(t *testing.T) {
	const testPath = `Software\FeisuoTest\AutostartRun`
	key, _, err := registry.CreateKey(registry.CURRENT_USER, testPath, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	key.Close()
	t.Cleanup(func() {
		_ = registry.DeleteKey(registry.CURRENT_USER, testPath)
	})

	prev := openRunKey
	openRunKey = func(access uint32) (registry.Key, error) {
		return registry.OpenKey(registry.CURRENT_USER, testPath, access)
	}
	t.Cleanup(func() { openRunKey = prev })

	if on, err := Enabled(); err != nil || on {
		t.Fatalf("Enabled() = %v, %v; want false", on, err)
	}

	if err := SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	on, err := Enabled()
	if err != nil || !on {
		t.Fatalf("Enabled() = %v, %v; want true", on, err)
	}
	key, err = registry.OpenKey(registry.CURRENT_USER, testPath, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	val, _, err := key.GetStringValue(ValueName)
	key.Close()
	if err != nil {
		t.Fatal(err)
	}
	if val == "" {
		t.Fatal("expected non-empty command")
	}

	if err := SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	if on, err := Enabled(); err != nil || on {
		t.Fatalf("after disable Enabled() = %v, %v", on, err)
	}
}
