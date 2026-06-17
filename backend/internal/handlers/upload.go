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
		allowed := map[string]bool{
			".jpg": true, ".jpeg": true, ".png": true,
			".gif": true, ".webp": true, ".svg": true,
		}
		if !allowed[ext] {
			writeErr(w, http.StatusBadRequest, "only images are supported")
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
		io.Copy(out, file)

		writeJSON(w, http.StatusOK, map[string]string{
			"url": fmt.Sprintf("%s/avatars/%s", baseURL, filename),
		})
	}
}
