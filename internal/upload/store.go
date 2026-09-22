package upload

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Store struct {
	root string
}

func NewStore(root string) (*Store, error) {
	s := &Store{root: root}
	for _, d := range []string{s.ChunksRoot(), s.OriginalsRoot(), s.TranscodedRoot()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) ChunksRoot() string {
	return filepath.Join(s.root, "chunks")
}

func (s *Store) OriginalsRoot() string {
	return filepath.Join(s.root, "originals")
}

func (s *Store) TranscodedRoot() string {
	return filepath.Join(s.root, "transcoded")
}

func (s *Store) ChunkDir(sessionID uint) string {
	return filepath.Join(s.ChunksRoot(), strconv.FormatUint(uint64(sessionID), 10))
}

func (s *Store) ChunkPath(sessionID uint, index int) string {
	return filepath.Join(s.ChunkDir(sessionID), strconv.Itoa(index))
}

func (s *Store) OriginalPath(basename, ext string) string {
	return filepath.Join(s.OriginalsRoot(), basename+"."+ext)
}

func (s *Store) RenditionPath(basename, quality string) string {
	return filepath.Join(s.TranscodedRoot(), basename+"_"+quality+".mp4")
}

func (s *Store) WriteChunk(sessionID uint, index int, r io.Reader) (int64, error) {
	if err := os.MkdirAll(s.ChunkDir(sessionID), 0o755); err != nil {
		return 0, err
	}
	path := s.ChunkPath(sessionID, index)
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, r)
	cerr := f.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return 0, err
	}
	if cerr != nil {
		_ = os.Remove(tmp)
		return 0, cerr
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return 0, err
	}
	return n, nil
}

func (s *Store) ListChunks(sessionID uint) ([]int, error) {
	dir := s.ChunkDir(sessionID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var idxs []int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".tmp") {
			continue
		}
		n, err := strconv.Atoi(name)
		if err != nil {
			continue
		}
		idxs = append(idxs, n)
	}
	return idxs, nil
}

func (s *Store) Merge(sessionID uint, dest string, total int) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	for i := 0; i < total; i++ {
		p := s.ChunkPath(sessionID, i)
		in, err := os.Open(p)
		if err != nil {
			return fmt.Errorf("missing chunk %d: %w", i, err)
		}
		_, err = io.Copy(out, in)
		_ = in.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) RemoveChunks(sessionID uint) error {
	return os.RemoveAll(s.ChunkDir(sessionID))
}
