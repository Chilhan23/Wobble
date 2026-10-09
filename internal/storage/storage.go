package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrFileTooLarge    = errors.New("file exceeds maximum allowed size")
	ErrInvalidFileType = errors.New("unsupported or dangerous file type")
	ErrEmptyFile       = errors.New("file is empty")
)

type AttachmentType string

const (
	AttachmentTypeImage    AttachmentType = "image"
	AttachmentTypeVideo    AttachmentType = "video"
	AttachmentTypeDocument AttachmentType = "document"
)

type SavedFile struct {
	RelativePath string
	Filename     string
	MIMEType     string
	Size         int64
	Type         AttachmentType
}

type Service interface {
	Save(publicID uuid.UUID, r io.Reader, maxImageBytes, maxVideoBytes int64) (*SavedFile, error)
	Delete(relativePath string) error
	GenerateSignedURL(baseURL string, messageID int64, ttl time.Duration) string
	VerifySignature(messageID int64, expStr, sig string) bool
	GetAbsolutePath(relativePath string) string
}

type service struct {
	uploadDir string
	secretKey []byte
}

func NewService(uploadDir string, tokenSecret string) Service {
	return &service{
		uploadDir: uploadDir,
		secretKey: []byte(tokenSecret),
	}
}

type URLSigner struct {
	storage Service
	baseURL string
	ttl     time.Duration
}

func NewURLSigner(storage Service, baseURL string, ttl time.Duration) *URLSigner {
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &URLSigner{
		storage: storage,
		baseURL: baseURL,
		ttl:     ttl,
	}
}

func (s *URLSigner) SignURL(messageID int64) string {
	return s.storage.GenerateSignedURL(s.baseURL, messageID, s.ttl)
}

// mimeMapping memetakan Content-Type hasil sniffing ke tipe dan ekstensi yang aman
var mimeMapping = map[string]struct {
	ext            string
	attachmentType AttachmentType
}{
	"image/jpeg": {ext: ".jpg", attachmentType: AttachmentTypeImage},
	"image/png":  {ext: ".png", attachmentType: AttachmentTypeImage},
	"image/webp": {ext: ".webp", attachmentType: AttachmentTypeImage},
	"image/gif":  {ext: ".gif", attachmentType: AttachmentTypeImage},
	"video/mp4":  {ext: ".mp4", attachmentType: AttachmentTypeVideo},
	"video/webm": {ext: ".webm", attachmentType: AttachmentTypeVideo},
}

func (s *service) Save(publicID uuid.UUID, r io.Reader, maxImageBytes, maxVideoBytes int64) (*SavedFile, error) {
	if maxImageBytes <= 0 {
		maxImageBytes = 5 * 1024 * 1024
	}
	if maxVideoBytes <= 0 {
		maxVideoBytes = 15 * 1024 * 1024
	}

	// 1. Baca 512 byte pertama untuk sniffing MIME asli
	headerBuf := make([]byte, 512)
	n, err := io.ReadFull(r, headerBuf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("failed to read file header: %w", err)
	}
	if n == 0 {
		return nil, ErrEmptyFile
	}

	detectedMIME := http.DetectContentType(headerBuf[:n])
	// Tangani kasus MIME dengan parameter charset (contoh: text/plain; charset=utf-8)
	baseMIME := strings.ToLower(strings.TrimSpace(strings.Split(detectedMIME, ";")[0]))

	info, allowed := mimeMapping[baseMIME]
	if !allowed {
		return nil, fmt.Errorf("%w: %s", ErrInvalidFileType, detectedMIME)
	}

	var maxBytes int64
	switch info.attachmentType {
	case AttachmentTypeImage:
		maxBytes = maxImageBytes
	case AttachmentTypeVideo:
		maxBytes = maxVideoBytes
	default:
		maxBytes = maxImageBytes
	}

	// 2. Siapkan folder penyimpanan per tiket: <uploadDir>/<publicID>/
	targetDir := filepath.Join(s.uploadDir, publicID.String())
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create upload directory: %w", err)
	}

	filename := uuid.New().String() + info.ext
	fullPath := filepath.Join(targetDir, filename)
	relPath := filepath.Join(publicID.String(), filename)

	f, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create target file: %w", err)
	}
	defer f.Close()

	// Tulis 512 byte header yang sudah dibaca
	written, err := f.Write(headerBuf[:n])
	if err != nil {
		_ = os.Remove(fullPath)
		return nil, fmt.Errorf("failed to write header to file: %w", err)
	}

	// Batasi sisa stream agar tidak melebihi batas ukuran
	remainingLimit := maxBytes - int64(written)
	if remainingLimit < 0 {
		_ = os.Remove(fullPath)
		return nil, ErrFileTooLarge
	}

	limitedReader := io.LimitReader(r, remainingLimit+1)
	restWritten, copyErr := io.Copy(f, limitedReader)
	if copyErr != nil {
		_ = os.Remove(fullPath)
		return nil, fmt.Errorf("failed to write file body: %w", copyErr)
	}

	totalSize := int64(written) + restWritten
	if totalSize > maxBytes {
		_ = os.Remove(fullPath)
		return nil, ErrFileTooLarge
	}

	return &SavedFile{
		RelativePath: relPath,
		Filename:     filename,
		MIMEType:     baseMIME,
		Size:         totalSize,
		Type:         info.attachmentType,
	}, nil
}

func (s *service) Delete(relativePath string) error {
	absPath := s.GetAbsolutePath(relativePath)
	if absPath == "" {
		return errors.New("invalid file path")
	}
	return os.Remove(absPath)
}

func (s *service) GenerateSignedURL(baseURL string, messageID int64, ttl time.Duration) string {
	exp := time.Now().Add(ttl).Unix()
	sig := s.computeSignature(messageID, exp)

	baseURL = strings.TrimRight(baseURL, "/")
	return fmt.Sprintf("%s/files/%d?exp=%d&sig=%s", baseURL, messageID, exp, sig)
}

func (s *service) VerifySignature(messageID int64, expStr, sig string) bool {
	if expStr == "" || sig == "" {
		return false
	}

	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}

	expectedSig := s.computeSignature(messageID, exp)
	return subtle.ConstantTimeCompare([]byte(sig), []byte(expectedSig)) == 1
}

func (s *service) GetAbsolutePath(relativePath string) string {
	// Cegah path traversal
	cleaned := filepath.Clean(relativePath)
	if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
		return ""
	}
	return filepath.Join(s.uploadDir, cleaned)
}

func (s *service) computeSignature(messageID int64, exp int64) string {
	mac := hmac.New(sha256.New, s.secretKey)
	mac.Write([]byte(fmt.Sprintf("%d:%d", messageID, exp)))
	return hex.EncodeToString(mac.Sum(nil))
}
