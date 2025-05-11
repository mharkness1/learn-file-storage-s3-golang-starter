package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	// TODO: implement the upload here
	const maxMemory int = 10 << 20
	err = r.ParseMultipartForm(int64(maxMemory))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse form", err)
		return
	}

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get file", err)
		return
	}
	defer file.Close()

	headerDetails := header.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(headerDetails)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to process file", err)
		return
	}

	contentTypeToExt := map[string]string{
		"image/png":  ".png",
		"image/jpeg": ".jpg",
	}
	extension, ok := contentTypeToExt[mediaType]
	if !ok {
		respondWithError(w, http.StatusBadRequest, "Unsupported file type", nil)
		return
	}

	databaseVideo, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video", err)
		return
	}
	if databaseVideo.UserID != userID {
		respondWithError(w, http.StatusForbidden, "You don't own this video", err)
		return
	}

	key := make([]byte, 32)
	rand.Read(key)
	keyEncoding := base64.RawURLEncoding.EncodeToString(key)

	fileName := fmt.Sprintf("%s%s", keyEncoding, extension)
	filePath := filepath.Join(cfg.assetsRoot, fileName)
	newFile, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create file", err)
		return
	}
	defer newFile.Close()

	_, err = io.Copy(newFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to save file", err)
		return
	}

	thumbnailURL := fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, fileName)
	updatedVideo := databaseVideo
	updatedVideo.ThumbnailURL = &thumbnailURL
	updatedVideo.UpdatedAt = time.Now()

	err = cfg.db.UpdateVideo(updatedVideo)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}
	respondWithJSON(w, http.StatusOK, updatedVideo)
}
