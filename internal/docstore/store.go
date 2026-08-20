package docstore

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

type Store struct {
	rootDir string
	logger  *zap.Logger
}

func NewStore(rootDir string, logger *zap.Logger) *Store {
	if rootDir == "" {
		rootDir = "./data/docs"
	}
	return &Store{
		rootDir: filepath.Clean(rootDir),
		logger:  logger,
	}
}

func (s *Store) RootDir() string { return s.rootDir }

func (s *Store) Save(docID string, reader io.Reader) error {
	if !isValidDocID(docID) {
		return fmt.Errorf("invalid doc_id: %q", docID)
	}
	path := s.GetPath(docID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	if _, err := io.Copy(f, reader); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("write document: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename to final: %w", err)
	}
	return nil
}

func (s *Store) Load(docID string) (io.ReadCloser, error) {
	if !isValidDocID(docID) {
		return nil, fmt.Errorf("invalid doc_id: %q", docID)
	}
	path := s.GetPath(docID)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("document not found: %s", docID)
		}
		return nil, fmt.Errorf("open document: %w", err)
	}
	return f, nil
}

func (s *Store) Delete(docID string) error {
	if !isValidDocID(docID) {
		return fmt.Errorf("invalid doc_id: %q", docID)
	}
	path := s.GetPath(docID)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove document: %w", err)
	}
	return nil
}

func (s *Store) GetPath(docID string) string {
	if len(docID) < 2 {
		return filepath.Join(s.rootDir, "_other", docID+".bin")
	}
	return filepath.Join(s.rootDir, docID[0:2], docID+".bin")
}

func (s *Store) CalcChecksum(docID string) (string, error) {
	rc, err := s.Load(docID)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	h := sha256.New()
	if _, err := io.Copy(h, rc); err != nil {
		return "", fmt.Errorf("hash document: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (s *Store) Exists(docID string) bool {
	if !isValidDocID(docID) {
		return false
	}
	if _, err := os.Stat(s.GetPath(docID)); err != nil {
		return false
	}
	return true
}

func isValidDocID(docID string) bool {
	if len(docID) < 2 {
		return false
	}
	if docID == "." || docID == ".." {
		return false
	}
	if strings.ContainsAny(docID, `/\`) {
		return false
	}
	if strings.HasPrefix(docID, ".") {
		return false
	}
	return true
}