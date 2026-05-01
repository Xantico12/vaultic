package main

import (
	"bufio"
	"io"
	"os"
	"strings"
	"sync"
)

type Store struct {
	data map[string]string
	walFile *os.File
	mu sync.Mutex
}

func NewStore(walPath string) (*Store, error) {
	file, err := os.OpenFile(walPath, os.O_RDWR | os.O_CREATE | os.O_APPEND, 0600)

	if err != nil {
		return nil, err
	}

	s := &Store{
		data: make(map[string]string),
		walFile: file,
	}

	if err := s.replay(); err != nil {
		s.walFile.Close()
		return nil, err
	}

	return s, nil
}

func (s *Store) replay() error {
	if _, err := s.walFile.Seek(0, io.SeekStart); err != nil {
		return err
	}

	scanner := bufio.NewScanner(s.walFile)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "|", 3)

		switch parts[0] {
		case "SET":
			s.data[parts[1]] = parts[2]

		case "DELETE":
			delete(s.data, parts[1])
		}
	}
return scanner.Err()
} 
