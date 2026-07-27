// Package backup implements authenticated, streaming encryption for
// control-plane PostgreSQL archives. It deliberately uses only the Go standard
// library so backup confidentiality does not depend on an extra runtime tool.
package backup

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	formatMagic = "OCBKP\x01"
	chunkSize   = 64 * 1024
)

var (
	// ErrInvalidArchive is returned for malformed or unauthenticated input.
	ErrInvalidArchive = errors.New("invalid encrypted backup archive")
)

// DecodeKey parses a base64-encoded 256-bit backup encryption key.
func DecodeKey(encoded string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		key, err = base64.RawStdEncoding.DecodeString(encoded)
	}
	if err != nil {
		return nil, errors.New("BACKUP_ENCRYPTION_KEY must be base64")
	}
	if len(key) != 32 {
		return nil, errors.New("BACKUP_ENCRYPTION_KEY must decode to exactly 32 bytes")
	}
	return key, nil
}

// Encrypt writes an authenticated chunked AES-256-GCM stream. Each chunk has a
// unique nonce and authenticates the format header, sequence number, and length.
func Encrypt(dst io.Writer, src io.Reader, key []byte) error {
	aead, err := newAEAD(key)
	if err != nil {
		return err
	}
	noncePrefix := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, noncePrefix); err != nil {
		return fmt.Errorf("generate backup nonce: %w", err)
	}
	header := append([]byte(formatMagic), noncePrefix...)
	if _, err := dst.Write(header); err != nil {
		return fmt.Errorf("write backup header: %w", err)
	}

	reader := bufio.NewReaderSize(src, chunkSize)
	buf := make([]byte, chunkSize)
	var sequence uint32
	for {
		n, readErr := io.ReadFull(reader, buf)
		switch {
		case readErr == nil:
		case errors.Is(readErr, io.ErrUnexpectedEOF):
		case errors.Is(readErr, io.EOF):
			n = 0
		default:
			return fmt.Errorf("read backup input: %w", readErr)
		}
		if err := writeChunk(dst, aead, header, noncePrefix, sequence, buf[:n]); err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		if sequence == ^uint32(0) {
			return errors.New("backup input exceeds encrypted stream limit")
		}
		sequence++
		if errors.Is(readErr, io.ErrUnexpectedEOF) {
			return writeChunk(dst, aead, header, noncePrefix, sequence, nil)
		}
	}
}

// Decrypt authenticates and restores a stream written by Encrypt.
func Decrypt(dst io.Writer, src io.Reader, key []byte) error {
	aead, err := newAEAD(key)
	if err != nil {
		return err
	}
	header := make([]byte, len(formatMagic)+8)
	if _, err := io.ReadFull(src, header); err != nil {
		return ErrInvalidArchive
	}
	if string(header[:len(formatMagic)]) != formatMagic {
		return ErrInvalidArchive
	}
	noncePrefix := header[len(formatMagic):]
	var sequence uint32
	for {
		var lengthBytes [4]byte
		if _, err := io.ReadFull(src, lengthBytes[:]); err != nil {
			return ErrInvalidArchive
		}
		plainLength := binary.BigEndian.Uint32(lengthBytes[:])
		if plainLength > chunkSize {
			return ErrInvalidArchive
		}
		sealed := make([]byte, int(plainLength)+aead.Overhead())
		if _, err := io.ReadFull(src, sealed); err != nil {
			return ErrInvalidArchive
		}
		nonce := chunkNonce(noncePrefix, sequence)
		aad := chunkAAD(header, sequence, plainLength)
		plain, err := aead.Open(nil, nonce, sealed, aad)
		if err != nil {
			return ErrInvalidArchive
		}
		if plainLength == 0 {
			var trailing [1]byte
			if _, err := io.ReadFull(src, trailing[:]); !errors.Is(err, io.EOF) {
				return ErrInvalidArchive
			}
			return nil
		}
		if _, err := dst.Write(plain); err != nil {
			return fmt.Errorf("write decrypted backup: %w", err)
		}
		if sequence == ^uint32(0) {
			return ErrInvalidArchive
		}
		sequence++
	}
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, errors.New("backup encryption key must be exactly 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("initialize backup cipher: %w", err)
	}
	return cipher.NewGCM(block)
}

func writeChunk(
	dst io.Writer,
	aead cipher.AEAD,
	header, noncePrefix []byte,
	sequence uint32,
	plain []byte,
) error {
	var lengthBytes [4]byte
	binary.BigEndian.PutUint32(lengthBytes[:], uint32(len(plain)))
	nonce := chunkNonce(noncePrefix, sequence)
	aad := chunkAAD(header, sequence, uint32(len(plain)))
	sealed := aead.Seal(nil, nonce, plain, aad)
	if _, err := dst.Write(lengthBytes[:]); err != nil {
		return fmt.Errorf("write backup chunk length: %w", err)
	}
	if _, err := dst.Write(sealed); err != nil {
		return fmt.Errorf("write encrypted backup chunk: %w", err)
	}
	return nil
}

func chunkNonce(prefix []byte, sequence uint32) []byte {
	nonce := make([]byte, 12)
	copy(nonce, prefix)
	binary.BigEndian.PutUint32(nonce[8:], sequence)
	return nonce
}

func chunkAAD(header []byte, sequence, plainLength uint32) []byte {
	aad := make([]byte, 0, len(header)+8)
	aad = append(aad, header...)
	var encoded [8]byte
	binary.BigEndian.PutUint32(encoded[:4], sequence)
	binary.BigEndian.PutUint32(encoded[4:], plainLength)
	return append(aad, encoded[:]...)
}
