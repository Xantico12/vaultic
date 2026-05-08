package vault

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

const magicValue = "vaultic-v1"

var ErrInvalidPassword = errors.New("invalid password")

type Store struct {
	data 	map[string]string
	walFile *os.File
	key 	[]byte
	mu 		sync.Mutex
}

func NewStore(walPath string, password []byte) (*Store, error) {
	file, err := os.OpenFile(walPath, os.O_RDWR | os.O_CREATE | os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}

	s := &Store{
		data: make(map[string]string),
		walFile: file,
	}

	// CHeck if file empty with Stat()
	info, err := file.Stat()
	if err != nil {
		s.walFile.Close()
		return nil, err
	}

	if info.Size() == 0 {
		// First run: generate salt, derive key, write header
		if err := s.initNewVault([]byte(password)); err != nil {
			s.walFile.Close()
			return nil, err
		}
	} else {
		// Existing file: read salt, verify password, replay
		if err := s.openExistingVault([]byte(password)); err != nil {
			s.walFile.Close()
			return nil, err
		}
	}

	return s, nil
}

func (s *Store) initNewVault(password []byte) error {
    salt, err := NewSalt()
    if err != nil {
        return err
    }

    s.key = DeriveKey(password, salt)

    // Write SALT|<base64>\n
    saltLine := fmt.Sprintf("SALT|%s\n", base64.StdEncoding.EncodeToString(salt))
    if _, err := s.walFile.WriteString(saltLine); err != nil {
        return err
    }

    // Encrypt magic and write MAGIC|<base64>\n
    encrypted, err := Encrypt(s.key, []byte(magicValue))
    if err != nil {
        return err
    }
    magicLine := fmt.Sprintf("MAGIC|%s\n", base64.StdEncoding.EncodeToString(encrypted))
    if _, err := s.walFile.WriteString(magicLine); err != nil {
        return err
    }

    return s.walFile.Sync()
}

func (s *Store) openExistingVault(password []byte) error {
    if _, err := s.walFile.Seek(0, io.SeekStart); err != nil {
        return err
    }

    scanner := bufio.NewScanner(s.walFile)

    // Line 1: SALT
    if !scanner.Scan() {
        return errors.New("WAL is missing SALT line")
    }
    saltLine := strings.SplitN(scanner.Text(), "|", 2)
    if len(saltLine) != 2 || saltLine[0] != "SALT" {
        return errors.New("WAL first line is not SALT")
    }
    salt, err := base64.StdEncoding.DecodeString(saltLine[1])
    if err != nil {
        return fmt.Errorf("decode salt: %w", err)
    }

    s.key = DeriveKey(password, salt)

    // Line 2: MAGIC — used to verify password
    if !scanner.Scan() {
        return errors.New("WAL is missing MAGIC line")
    }
    magicLine := strings.SplitN(scanner.Text(), "|", 2)
    if len(magicLine) != 2 || magicLine[0] != "MAGIC" {
        return errors.New("WAL second line is not MAGIC")
    }
    encryptedMagic, err := base64.StdEncoding.DecodeString(magicLine[1])
    if err != nil {
        return fmt.Errorf("decode magic: %w", err)
    }
    decrypted, err := Decrypt(s.key, encryptedMagic)
    if err != nil || string(decrypted) != magicValue {
        return ErrInvalidPassword
    }

    // Lines 3+: replay SET/DELETE
    for scanner.Scan() {
        line := scanner.Text()
        parts := strings.SplitN(line, "|", 3)

        switch parts[0] {
        case "SET":
            encrypted, err := base64.StdEncoding.DecodeString(parts[2])
            if err != nil {
                return fmt.Errorf("decode SET value for %q: %w", parts[1], err)
            }
            plaintext, err := Decrypt(s.key, encrypted)
            if err != nil {
                return fmt.Errorf("decrypt SET value for %q: %w", parts[1], err)
            }
            s.data[parts[1]] = string(plaintext)

        case "DELETE":
            delete(s.data, parts[1])
        }
    }

    return scanner.Err()
}

func (s *Store) Set(key, value string) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    encrypted, err := Encrypt(s.key, []byte(value))
    if err != nil {
        return err
    }
    encoded := base64.StdEncoding.EncodeToString(encrypted)

    entry := fmt.Sprintf("SET|%s|%s\n", key, encoded)
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