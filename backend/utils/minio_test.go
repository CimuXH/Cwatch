package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"backend/models"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestMinIOMediaUsesStoredSourceAndRanges(t *testing.T) {
	video := &models.Video{FileName: "source.mp4", Storage: models.StorageMinIO}
	video.ID = 42
	video.SourceObject = videoObjectPrefix(video) + "source/fixed-version"
	content := []byte("0123456789")
	var rangedRead bool
	var artifactWritten bool
	client, err := minio.New("storage.example", &minio.Options{
		Creds: credentials.NewStaticV4("test-access", "test-secret", ""), Secure: true,
		Region: "us-east-1", BucketLookup: minio.BucketLookupPath, MaxRetries: 1,
		Transport: mediaRoundTripper(func(req *http.Request) (*http.Response, error) {
			headers := http.Header{"Etag": {`"test-etag"`}}
			if req.Method == http.MethodPut && req.URL.Path == "/cwatch/"+videoObjectPrefix(video)+"attempt/cover.jpg" {
				body, err := io.ReadAll(req.Body)
				if err != nil {
					return nil, err
				}
				artifactWritten = bytes.Contains(body, []byte("cover bytes"))
				return &http.Response{StatusCode: http.StatusOK, Header: headers, Body: http.NoBody}, nil
			}
			if req.URL.Path != "/cwatch/"+video.SourceObject {
				return nil, fmt.Errorf("unexpected object: %s", req.URL.Path)
			}
			headers.Set("Last-Modified", time.Unix(1700000000, 0).UTC().Format(http.TimeFormat))
			headers.Set("Content-Type", "video/mp4")
			result := &http.Response{Header: headers, StatusCode: http.StatusOK, Body: http.NoBody}
			data := content
			switch req.Method {
			case http.MethodHead:
			case http.MethodGet:
				if requested := req.Header.Get("Range"); requested != "" {
					if !strings.HasPrefix(requested, "bytes=2-") {
						return nil, fmt.Errorf("unexpected Range: %s", requested)
					}
					rangedRead = true
					data = data[2:]
					headers.Set("Content-Range", "bytes 2-9/10")
					result.StatusCode = http.StatusPartialContent
				}
				result.Body = io.NopCloser(bytes.NewReader(data))
			default:
				return nil, fmt.Errorf("unexpected request method: %s", req.Method)
			}
			headers.Set("Content-Length", strconv.Itoa(len(data)))
			result.ContentLength = int64(len(data))
			return result, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	previous := objectStorage
	objectStorage = &minioStorage{client: client, bucket: "cwatch"}
	t.Cleanup(func() { objectStorage = previous })

	ctx := context.Background()
	file, modified, err := OpenVideoMedia(ctx, video, video.FileName)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/media/42/source.mp4", nil)
	req.Header.Set("Range", "bytes=2-5")
	response := httptest.NewRecorder()
	response.Header().Set("Content-Type", "video/mp4")
	http.ServeContent(response, req, video.FileName, modified, file)
	_ = file.Close()
	if response.Code != http.StatusPartialContent || response.Body.String() != "2345" || !rangedRead {
		t.Fatalf("range status=%d body=%q forwarded=%v", response.Code, response.Body.String(), rangedRead)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cover.jpg"), []byte("cover bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := StoreTranscodeArtifact(ctx, video, "attempt", "cover.jpg", dir, "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	if !artifactWritten {
		t.Fatal("expected MinIO PUT to the attempt-specific key")
	}
}

type mediaRoundTripper func(*http.Request) (*http.Response, error)

func (rt mediaRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt(req)
}
