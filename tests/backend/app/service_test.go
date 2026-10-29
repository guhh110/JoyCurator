package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"joycurator/internal/infra/fsops"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/trace"
)

type configStore struct {
	value            model.Settings
	entered, release chan struct{}
}

func (s *configStore) LoadSettings() (model.Settings, error) { return s.value, nil }
func (s *configStore) SaveSettings(value model.Settings) error {
	if s.entered != nil {
		close(s.entered)
		<-s.release
	}
	s.value = value
	return nil
}

func TestSettingsCommitRetainsAdmissionLockUntilPersisted(t *testing.T) {
	root := t.TempDir()
	source, target := filepath.Join(root, "source"), filepath.Join(root, "target")
	for _, dir := range []string{source, target} {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	access, err := fsops.New([]string{source, target})
	if err != nil {
		t.Fatal(err)
	}
	coordinator := organizer.NewCoordinator()
	defer coordinator.Close()
	value := model.DefaultSettings()
	value.SourceRoot, value.TargetRoot = access.Roots[0], access.Roots[1]
	store := &configStore{value: value, entered: make(chan struct{}), release: make(chan struct{})}
	invalidations := 0
	service := NewConfig(store, access, coordinator, matching.Factory(func(model.Settings, *trace.Recorder) (matching.Provider, error) { return nil, nil }), func() { invalidations++ })
	updated := make(chan error, 1)
	go func() { updated <- service.Update(ConfigUpdate{Settings: value}) }()
	select {
	case <-store.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("save did not start")
	}
	attempted, admitted := make(chan struct{}), make(chan struct{})
	var observedVersion int
	go func() {
		close(attempted)
		coordinator.Lock()
		defer coordinator.Unlock()
		observedVersion = store.value.Version
		close(admitted)
	}()
	<-attempted
	select {
	case <-admitted:
		t.Fatal("execution admission passed configuration commit")
	default:
	}
	close(store.release)
	if err := <-updated; err != nil {
		t.Fatal(err)
	}
	select {
	case <-admitted:
	case <-time.After(3 * time.Second):
		t.Fatal("admission remained locked")
	}
	if observedVersion != value.Version+1 || invalidations != 1 {
		t.Fatal(observedVersion, invalidations)
	}
	if err := service.Update(ConfigUpdate{Settings: value}); err == nil {
		t.Fatal("stale version accepted")
	}
}
