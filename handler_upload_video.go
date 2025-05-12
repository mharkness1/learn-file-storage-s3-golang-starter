package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {

	const maxUploadSize int = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxUploadSize))

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

	databaseVideo, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video", err)
		return
	}
	if databaseVideo.UserID != userID {
		respondWithError(w, http.StatusForbidden, "You don't own this video", err)
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get file", err)
		return
	}
	defer file.Close()

	headerDetails := header.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(headerDetails)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error processing video file", err)
		return
	}

	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Video format is not an accepted file type", nil)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create temp file", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to save video to disk", err)
		return
	}

	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to process file", err)
		return
	}

	processVideoFilePath, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to generate fast start", err)
		return
	}
	defer os.Remove(processVideoFilePath)

	fastStartFile, err := os.Open(processVideoFilePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to get fast start file", err)
		return
	}
	defer fastStartFile.Close()

	var prefix string
	if aspectRatio == "16:9" {
		prefix = "landscape"
	} else if aspectRatio == "9:16" {
		prefix = "portrait"
	} else {
		prefix = "other"
	}

	/*
		_, err = tempFile.Seek(0, io.SeekStart)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Error reading the file", err)
			return
		}
	*/

	key := make([]byte, 32)
	rand.Read(key)
	keyEncoding := hex.EncodeToString(key)
	s3Key := fmt.Sprintf("%s,%s/%s.mp4", cfg.s3Bucket, prefix, keyEncoding)

	cfg.s3client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &s3Key,
		Body:        fastStartFile,
		ContentType: &mediaType,
	})

	s3Url := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, s3Key)
	updatedVideo := databaseVideo
	updatedVideo.VideoURL = &s3Url

	err = cfg.db.UpdateVideo(updatedVideo)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed updating video record", err)
		return
	}

	respondWithJSON(w, http.StatusOK, updatedVideo)
}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	s3Presign := s3.NewPresignClient(s3Client)

	presignedReq, err := s3Presign.PresignGetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expireTime))

	if err != nil {
		return "", err
	}

	return presignedReq.URL, nil
}
