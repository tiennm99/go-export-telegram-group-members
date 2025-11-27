package couchbasestorage

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/couchbase/gocb/v2"
	"github.com/gotd/contrib/storage"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/session"
	"github.com/gotd/td/tg"
)

type CouchbaseStorages struct {
	Cluster  *gocb.Cluster
	Sessions *gocb.Collection
	Peers    *gocb.Collection
	Updates  *gocb.Collection
}

func NewCouchbaseStorages() (*CouchbaseStorages, error) {
	connStr := os.Getenv("COUCHBASE_CONNECTION_STRING")
	if connStr == "" {
		return nil, fmt.Errorf("COUCHBASE_CONNECTION_STRING not set")
	}
	user := os.Getenv("COUCHBASE_USERNAME")
	pass := os.Getenv("COUCHBASE_PASSWORD")
	bucketName := os.Getenv("COUCHBASE_BUCKET_NAME")
	scopeName := os.Getenv("COUCHBASE_SCOPE_NAME")

	cluster, err := gocb.Connect(connStr, gocb.ClusterOptions{
		Authenticator: gocb.PasswordAuthenticator{
			Username: user,
			Password: pass,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("connect to couchbase: %w", err)
	}

	_, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err = cluster.WaitUntilReady(10*time.Second, &gocb.WaitUntilReadyOptions{})
	if err != nil {
		cluster.Close(nil)
		return nil, fmt.Errorf("wait ready: %w", err)
	}

	bucket := cluster.Bucket(bucketName)
	scope := bucket.Scope(scopeName)

	return &CouchbaseStorages{
		Cluster:  cluster,
		Sessions: scope.Collection("sessions"),
		Peers:    scope.Collection("peers"),
		Updates:  scope.Collection("update_states"),
	}, nil
}

type CouchbaseSessionStorage struct {
	coll *gocb.Collection
	key  string
}

func NewCouchbaseSessionStorage(coll *gocb.Collection, key string) *CouchbaseSessionStorage {
	return &CouchbaseSessionStorage{coll: coll, key: key}
}

func (s *CouchbaseSessionStorage) Load(ctx context.Context) (*session.Data, error) {
	var data session.Data
	result, err := s.coll.Get(s.key, &gocb.GetOptions{})
	if err != nil {
		return &data, session.ErrNotFound
	}
	err = result.Content(&data)
	if err != nil {
		return &data, session.ErrNotFound
	}
	return &data, nil
}

func (s *CouchbaseSessionStorage) Save(ctx context.Context, data *session.Data) error {
	_, err := s.coll.Upsert(s.key, data, &gocb.UpsertOptions{})
	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

type CouchbasePeerStorage struct {
	coll *gocb.Collection
}

func NewCouchbasePeerStorage(coll *gocb.Collection) *CouchbasePeerStorage {
	return &CouchbasePeerStorage{coll: coll}
}

func peerKey(id int64) string {
	return strconv.FormatInt(id, 10)
}

func (s *CouchbasePeerStorage) Get(ctx context.Context, id int64) (tg.PeerClass, error) {
	key := peerKey(id)
	var data []byte
	_, err := s.coll.Get(key, &data)
	if err != nil {
		if gocb.IsDocumentNotFoundError(err) {
			return nil, storage.ErrPeerNotFound
		}
		return nil, fmt.Errorf("get peer %d: %w", id, err)
	}
	buf := bin.Buffer{Buf: data}
	dec := bin.NewDecoder(&buf)
	var peer tg.PeerClass
	if err := dec.DecodeRaw(&peer); err != nil {
		return nil, fmt.Errorf("decode peer %d: %w", id, err)
	}
	return peer, nil
}

func (s *CouchbasePeerStorage) Set(ctx context.Context, id int64, peer tg.PeerClass) error {
	key := peerKey(id)
	buf := bin.NewBuffer(nil)
	enc := bin.NewEncoder(buf)
	if err := enc.EncodeRaw(peer); err != nil {
		return fmt.Errorf("encode peer %d: %w", id, err)
	}
	_, err := s.coll.Upsert(key, buf.Bytes())
	if err != nil {
		return fmt.Errorf("upsert peer %d: %w", id, err)
	}
	return nil
}

func (s *CouchbasePeerStorage) Delete(ctx context.Context, id int64) error {
	key := peerKey(id)
	_, err := s.coll.Remove(key, nil)
	if err != nil && !gocb.IsDocumentNotFoundError(err) {
		return fmt.Errorf("delete peer %d: %w", id, err)
	}
	return nil
}

type CouchbaseStateStorage struct {
	coll *gocb.Collection
}

func NewCouchbaseStateStorage(coll *gocb.Collection) *CouchbaseStateStorage {
	return &CouchbaseStateStorage{coll: coll}
}

func (s *CouchbaseStateStorage) GetState(ctx context.Context) (tg.UpdatesState, error) {
	var state tg.UpdatesState
	_, err := s.coll.Get("state", &state)
	if err != nil {
		if gocb.IsDocumentNotFoundError(err) {
			return tg.UpdatesState{}, nil
		}
		return tg.UpdatesState{}, fmt.Errorf("get state: %w", err)
	}
	return state, nil
}

func (s *CouchbaseStateStorage) SetState(ctx context.Context, st tg.UpdatesState) error {
	_, err := s.coll.Upsert("state", st)
	if err != nil {
		return fmt.Errorf("set state: %w", err)
	}
	return nil
}
