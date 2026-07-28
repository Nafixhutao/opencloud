package provisioner

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"

	"github.com/google/uuid"
)

// FakeDatabase is a concurrency-safe database provisioner for tests. A retry
// rotates the password while preserving the same deterministic database/user.
type FakeDatabase struct {
	mu        sync.Mutex
	databases map[uuid.UUID]fakeDatabase
	failNext  error
}

type fakeDatabase struct {
	spec     DatabaseSpec
	password string
}

// NewFakeDatabase constructs an empty fake data plane.
func NewFakeDatabase() *FakeDatabase {
	return &FakeDatabase{databases: make(map[uuid.UUID]fakeDatabase)}
}

// FailNext makes the next data-plane operation fail.
func (f *FakeDatabase) FailNext(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failNext = err
}

// CreateDatabase creates or converges one owned resource.
func (f *FakeDatabase) CreateDatabase(_ context.Context, spec DatabaseSpec) (DatabaseCredentials, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return DatabaseCredentials{}, err
	}
	if existing, ok := f.databases[spec.DatabaseID]; ok {
		if existing.spec.AccountID != spec.AccountID ||
			existing.spec.Engine != spec.Engine ||
			existing.spec.DatabaseName != spec.DatabaseName ||
			existing.spec.Username != spec.Username {
			return DatabaseCredentials{}, errors.New("database ownership mismatch")
		}
	}
	password, err := randomDatabasePassword()
	if err != nil {
		return DatabaseCredentials{}, err
	}
	f.databases[spec.DatabaseID] = fakeDatabase{spec: spec, password: password}
	return DatabaseCredentials{
		Engine:      spec.Engine,
		Host:        spec.Engine + ".test.invalid",
		Port:        databaseDefaultPort(spec.Engine),
		Database:    spec.DatabaseName,
		Username:    spec.Username,
		Password:    password,
		TLSRequired: true,
	}, nil
}

// DeleteDatabase removes one owned resource; missing is success.
func (f *FakeDatabase) DeleteDatabase(_ context.Context, ref DatabaseRef) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return err
	}
	existing, ok := f.databases[ref.DatabaseID]
	if !ok {
		return nil
	}
	if existing.spec.AccountID != ref.AccountID ||
		existing.spec.Engine != ref.Engine ||
		existing.spec.DatabaseName != ref.DatabaseName ||
		existing.spec.Username != ref.Username {
		return errors.New("database ownership mismatch")
	}
	delete(f.databases, ref.DatabaseID)
	return nil
}

// Exists reports fake data-plane state for tests.
func (f *FakeDatabase) Exists(databaseID uuid.UUID) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.databases[databaseID]
	return ok
}

func randomDatabasePassword() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func databaseDefaultPort(engine string) int {
	if engine == "mariadb" {
		return 3306
	}
	return 5432
}
