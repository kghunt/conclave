package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultVideoMaxMB = 50

func UploadFile(uploadDir, baseURL string, db *pgxpool.Pool) http.HandlerFunc {
	os.MkdirAll(uploadDir, 0755)
	return func(w http.ResponseWriter, r *http.Request) {
		videoMaxMB := int64(videoSizeLimitMB(r.Context(), db))
		imageLimitBytes := int64(10 << 20) // always 10MB for images
		videoLimitBytes := videoMaxMB << 20

		// Use the larger limit for the initial body read so we can inspect type first
		maxBytes := videoLimitBytes
		if imageLimitBytes > maxBytes {
			maxBytes = imageLimitBytes
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes+1024) // +1024 for form overhead
		if err := r.ParseMultipartForm(maxBytes); err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("file too large (max %dMB for video, 10MB for images)", videoMaxMB))
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeErr(w, http.StatusBadRequest, "missing file")
			return
		}
		defer file.Close()

		ext := strings.ToLower(filepath.Ext(header.Filename))
		isVideo := allowedVideoExt[ext]
		isImage := allowedImageExt[ext]

		if !isImage && !isVideo {
			writeErr(w, http.StatusBadRequest, "unsupported file type (images: jpg/png/gif/webp; video: mp4/webm/mov)")
			return
		}

		if isVideo {
			if videoMaxMB == 0 {
				writeErr(w, http.StatusBadRequest, "video uploads are disabled")
				return
			}
			if header.Size > videoLimitBytes {
				writeErr(w, http.StatusBadRequest, fmt.Sprintf("video too large (max %dMB)", videoMaxMB))
				return
			}
			if err := validateVideoMIME(file, ext); err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
		} else {
			if header.Size > imageLimitBytes {
				writeErr(w, http.StatusBadRequest, "image too large (max 10MB)")
				return
			}
			if err := validateMIME(file, ext); err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
		}

		filename := uuid.New().String() + ext
		dest := filepath.Join(uploadDir, filename)
		out, err := os.Create(dest)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "save failed")
			return
		}
		defer out.Close()
		if _, err := io.Copy(out, file); err != nil {
			os.Remove(dest)
			writeErr(w, http.StatusInternalServerError, "save failed")
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"url": fmt.Sprintf("%s/avatars/%s", baseURL, filename),
		})
	}
}

func videoSizeLimitMB(ctx context.Context, db *pgxpool.Pool) int {
	var val string
	db.QueryRow(ctx, `SELECT value FROM settings WHERE key = 'max_video_size_mb'`).Scan(&val)
	if val == "" {
		return defaultVideoMaxMB
	}
	n, err := strconv.Atoi(val)
	if err != nil || n < 0 {
		return defaultVideoMaxMB
	}
	return n
}

// DeleteUploadedFile removes a media file from disk if the message content is
// a URL that points to our own upload directory. Safe against path traversal.
func DeleteUploadedFile(uploadDir, baseURL, content string) {
	prefix := baseURL + "/avatars/"
	if !strings.HasPrefix(content, prefix) {
		return
	}
	filename := strings.TrimSpace(content[len(prefix):])
	// Reject anything with path separators or parent-directory segments
	if filename == "" || strings.ContainsAny(filename, "/\\") || strings.Contains(filename, "..") {
		return
	}
	_ = os.Remove(filepath.Join(uploadDir, filename))
}

var allowedImageExt = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true,
	".gif": true, ".webp": true,
}

var allowedVideoExt = map[string]bool{
	".mp4": true, ".webm": true, ".mov": true,
}

func validateVideoMIME(f io.ReadSeeker, ext string) error {
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("file read error")
	}
	detected := http.DetectContentType(buf[:n])
	// Go's DetectContentType returns video/mp4, video/webm, or application/octet-stream
	// for QuickTime/mov. We accept any video/* prefix or octet-stream for mov.
	if strings.HasPrefix(detected, "video/") {
		return nil
	}
	// QuickTime files are often detected as application/octet-stream
	if ext == ".mov" && (strings.HasPrefix(detected, "application/octet-stream") || strings.HasPrefix(detected, "video/")) {
		return nil
	}
	return fmt.Errorf("file content does not match declared video type")
}

// validateMIME reads the first 512 bytes to detect the actual content type and
// verifies it matches the declared file extension. The read position is reset.
func validateMIME(f io.ReadSeeker, ext string) error {
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("file read error")
	}
	if !imageMIMEMatches(buf[:n], ext) {
		return fmt.Errorf("file content does not match declared type")
	}
	return nil
}

func imageMIMEMatches(buf []byte, ext string) bool {
	// WebP: RIFF????WEBP — not in Go's DetectContentType
	if ext == ".webp" {
		return len(buf) >= 12 &&
			string(buf[0:4]) == "RIFF" &&
			string(buf[8:12]) == "WEBP"
	}
	detected := http.DetectContentType(buf)
	expected := map[string]string{
		".jpg": "image/jpeg", ".jpeg": "image/jpeg",
		".png": "image/png", ".gif": "image/gif",
	}[ext]
	return expected != "" && strings.HasPrefix(detected, expected)
}
