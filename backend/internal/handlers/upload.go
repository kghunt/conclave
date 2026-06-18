package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

func UploadFile(uploadDir, baseURL string) http.HandlerFunc {
	os.MkdirAll(uploadDir, 0755)
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10MB
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			writeErr(w, http.StatusBadRequest, "file too large (max 10MB)")
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeErr(w, http.StatusBadRequest, "missing file")
			return
		}
		defer file.Close()

		ext := strings.ToLower(filepath.Ext(header.Filename))
		if !allowedImageExt[ext] {
			writeErr(w, http.StatusBadRequest, "only images are supported")
			return
		}
		if err := validateMIME(file, ext); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
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

var allowedImageExt = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true,
	".gif": true, ".webp": true,
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
