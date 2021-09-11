package store

import (
	"context"
	"encoding/gob"
	"fmt"
	"net/url"
	"os"

	"github.com/gofrs/uuid"
)

func init() {
	gob.Register(gobStore{})
}

var _ Store = (*FileStore)(nil)
var _ AuthStore = (*FileStore)(nil)

type gobStore struct {
	Hot     map[string]*url.URL
	UserHot map[string]map[string]*url.URL
}

type FileStore struct {
	store   *gobStore
	enc     *gob.Encoder
	persist *os.File
}

// NewFileStore create new NewFileStore instance
func NewFileStore(filepath string) (*FileStore, error) {
	fd, err := os.OpenFile(filepath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return nil, fmt.Errorf("cannot open file at path %s: %w", filepath, err)
	}

	gs := gobStore{
		Hot:     make(map[string]*url.URL),
		UserHot: make(map[string]map[string]*url.URL),
	}

	dec := gob.NewDecoder(fd)
	if err := dec.Decode(&gs); err != nil {
		// truncate bad file for reuse
		if err := fd.Truncate(0); err != nil {
			return nil, fmt.Errorf("cannot truncate broken storage file: %w", err)
		}
	}

	return &FileStore{
		store:   &gs,
		enc:     gob.NewEncoder(fd),
		persist: fd,
	}, nil
}

func (f *FileStore) Save(_ context.Context, u *url.URL) (id string, err error) {
	id = fmt.Sprintf("%x", len(f.store.Hot))
	f.store.Hot[id] = u
	return id, f.flush()
}

func (f *FileStore) Load(_ context.Context, id string) (u *url.URL, err error) {
	if u, ok := f.store.Hot[id]; ok {
		return u, nil
	}
	return nil, ErrNotFound
}

func (f *FileStore) SaveUser(ctx context.Context, uid uuid.UUID, u *url.URL) (id string, err error) {
	id, err = f.Save(ctx, u)
	if err != nil {
		return "", fmt.Errorf("cannot save URL to shared store: %w", err)
	}
	if _, ok := f.store.UserHot[uid.String()]; !ok {
		f.store.UserHot[uid.String()] = make(map[string]*url.URL)
	}
	f.store.UserHot[uid.String()][id] = u
	return id, f.flush()
}

func (f *FileStore) LoadUser(ctx context.Context, uid uuid.UUID, id string) (u *url.URL, err error) {
	urls, err := f.LoadUsers(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("cannot load user urls: %w", err)
	}
	if u, ok := urls[id]; ok {
		return u, nil
	}
	return nil, ErrNotFound
}

func (f *FileStore) LoadUsers(_ context.Context, uid uuid.UUID) (urls map[string]*url.URL, err error) {
	if urls, ok := f.store.UserHot[uid.String()]; ok {
		return urls, nil
	}
	return nil, ErrNotFound
}

func (f *FileStore) Close() error {
	if err := f.flush(); err != nil {
		return fmt.Errorf("cannot flush data to file: %w", err)
	}
	return f.persist.Close()
}

func (f *FileStore) flush() error {
	return f.enc.Encode(f.store)
}
