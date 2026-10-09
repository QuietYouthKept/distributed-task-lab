package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type LocalStore struct {
	root     string
	prefix   string
	maxBytes int64
}

func NewLocalStore(root, prefix string, maxBytes int64) (*LocalStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("artifact root is required")
	}
	if prefix != "" {
		if err := ValidateKey(prefix); err != nil {
			return nil, err
		}
	}
	if maxBytes <= 0 {
		maxBytes = 64 << 20
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	return &LocalStore{root: resolved, prefix: prefix, maxBytes: maxBytes}, nil
}

func (s *LocalStore) scopedKey(key string) (string, error) {
	if err := ValidateKey(key); err != nil {
		return "", err
	}
	if s.prefix != "" {
		key = s.prefix + "/" + key
	}
	return key, nil
}

func (s *LocalStore) pathFor(key string, createParent bool) (string, string, error) {
	scoped, err := s.scopedKey(key)
	if err != nil {
		return "", "", err
	}
	full := filepath.Join(s.root, filepath.FromSlash(scoped))
	parent := s.root
	segments := strings.Split(scoped, "/")
	for _, segment := range segments[:len(segments)-1] {
		candidate := filepath.Join(parent, segment)
		resolved, resolveErr := filepath.EvalSymlinks(candidate)
		if errors.Is(resolveErr, os.ErrNotExist) && createParent {
			if mkdirErr := os.Mkdir(candidate, 0o750); mkdirErr != nil && !errors.Is(mkdirErr, os.ErrExist) {
				return "", "", mkdirErr
			}
			resolved, resolveErr = filepath.EvalSymlinks(candidate)
		}
		if errors.Is(resolveErr, os.ErrNotExist) && !createParent {
			return full, scoped, os.ErrNotExist
		}
		if resolveErr != nil {
			return "", "", resolveErr
		}
		rel, relErr := filepath.Rel(s.root, resolved)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return "", "", ErrInvalidKey
		}
		parent = resolved
	}
	return full, scoped, nil
}

func (s *LocalStore) PutIfAbsent(ctx context.Context, key string, data []byte, contentType string) (ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return ObjectInfo{}, err
	}
	if int64(len(data)) > s.maxBytes {
		return ObjectInfo{}, wrapStoreError("put", key, ErrTooLarge, nil)
	}
	full, scoped, err := s.pathFor(key, true)
	if err != nil {
		return ObjectInfo{}, wrapStoreError("put", key, ErrInvalidKey, err)
	}
	sum := sha256.Sum256(data)
	info := ObjectInfo{Key: key, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data)), ContentType: contentType}
	tmp, err := os.CreateTemp(filepath.Dir(full), ".artifact-write-*")
	if err != nil {
		return ObjectInfo{}, wrapStoreError("put", key, ErrUnknownOutcome, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	_, writeErr := tmp.Write(data)
	syncErr := tmp.Sync()
	closeErr := tmp.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return ObjectInfo{}, wrapStoreError("put", key, ErrUnknownOutcome, errors.Join(writeErr, syncErr, closeErr))
	}
	err = os.Link(tmpName, full)
	if errors.Is(err, os.ErrExist) {
		_, stored, getErr := s.read(full, scoped)
		if getErr != nil {
			return ObjectInfo{}, getErr
		}
		if stored.SHA256 != info.SHA256 {
			return ObjectInfo{}, wrapStoreError("put", key, ErrConflict, nil)
		}
		stored.Key = key
		return stored, nil
	}
	if err != nil {
		return ObjectInfo{}, wrapStoreError("put", key, ErrUnknownOutcome, err)
	}
	return info, nil
}

func (s *LocalStore) read(full, scoped string) ([]byte, ObjectInfo, error) {
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ObjectInfo{}, wrapStoreError("get", scoped, ErrNotFound, err)
		}
		return nil, ObjectInfo{}, err
	}
	rel, err := filepath.Rel(s.root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return nil, ObjectInfo{}, ErrInvalidKey
	}
	f, err := os.Open(resolved)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ObjectInfo{}, wrapStoreError("get", scoped, ErrNotFound, err)
		}
		return nil, ObjectInfo{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, s.maxBytes+1))
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	if int64(len(data)) > s.maxBytes {
		return nil, ObjectInfo{}, wrapStoreError("get", scoped, ErrTooLarge, nil)
	}
	sum := sha256.Sum256(data)
	return data, ObjectInfo{Key: scoped, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data))}, nil
}

func (s *LocalStore) Get(ctx context.Context, key string) ([]byte, ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, ObjectInfo{}, err
	}
	full, scoped, err := s.pathFor(key, false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ObjectInfo{}, wrapStoreError("get", key, ErrNotFound, err)
		}
		return nil, ObjectInfo{}, wrapStoreError("get", key, ErrInvalidKey, err)
	}
	data, info, err := s.read(full, scoped)
	info.Key = key
	return data, info, err
}

func (s *LocalStore) Head(ctx context.Context, key string) (ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return ObjectInfo{}, err
	}
	full, scoped, err := s.pathFor(key, false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ObjectInfo{}, wrapStoreError("head", key, ErrNotFound, err)
		}
		return ObjectInfo{}, wrapStoreError("head", key, ErrInvalidKey, err)
	}
	_, info, err := s.read(full, scoped)
	info.Key = key
	return info, err
}

func (s *LocalStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	full, _, err := s.pathFor(key, false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return wrapStoreError("delete", key, ErrNotFound, err)
		}
		return wrapStoreError("delete", key, ErrInvalidKey, err)
	}
	if err := os.Remove(full); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return wrapStoreError("delete", key, ErrNotFound, err)
		}
		return err
	}
	return nil
}
