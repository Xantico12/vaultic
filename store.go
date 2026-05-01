package main

import (
	"bufio"
	"fmt"
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

func (s *Store) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := fmt.Sprintf("SET|%s|%s\n", key, value)
	if _, err := s.walFile.WriteString(entry); err != nil {
		return err
	}
	if err := s.walFile.Sync(); err != nil {
		return err
	}

	s.data[key] = value
	return nil
}

func (s *Store) Get(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	val, ok := s.data[key]
	return val, ok
}

func (s *Store) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := fmt.Sprintf("DELETE|%s\n", key)
	if _, err := s.walFile.WriteString(entry); err != nil {
		return err
	}
	if err := s.walFile.Sync(); err != nil {
		return err
	}

	delete(s.data, key)
	return nil
}

func (s *Store) List() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make(map[string]string, len(s.data))
	for k, v := range s.data {
		out[k] = v
	}
	return out
}

func (s *Store) Close() error {
	return s.walFile.Close()
}