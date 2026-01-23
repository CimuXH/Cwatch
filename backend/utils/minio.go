package utils

import (
	"context"
	"fmt"
	"log"

	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var mc *minio.Client

// MinIO 配置
const (
	MinioBucket       = "cwatch"        // 存储桶名称
	MinioEndpoint     = "101.132.25.34:9000" // MinIO 服务地址
	UploadURLExpiry   = 15 * time.Minute     // 上传URL有效期
	DownloadURLExpiry = 24 * time.Hour       // 下载URL有效期
)

// InitMinIO 初始化MinIO
func InitMinIO() error {
	// MinIO 凭证
	accessKeyID := "minioadmin"
	secretAccessKey := "minio111111"
	useSSL := false

	// 创建 MinIO 客户端实例
	client, err := minio.New(MinioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return err
	}

	mc = client

	// 验证连接：检查存储桶是否存在，不存在则创建
	ctx := context.Background()
	exists, err := mc.BucketExists(ctx, MinioBucket)
	if err != nil {
		return err
	}

	if !exists {
		// 创建存储桶
		err = mc.MakeBucket(ctx, MinioBucket, minio.MakeBucketOptions{})
		if err != nil {
			return err
		}
		log.Printf("MinIO 存储桶 '%s' 创建成功", MinioBucket)
	}

	// 设置存储桶策略为公开读取（允许匿名下载）
	policy := fmt.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Principal": {"AWS": ["*"]},
				"Action": ["s3:GetObject"],
				"Resource": ["arn:aws:s3:::%s/*"]
			}
		]
	}`, MinioBucket)

	err = mc.SetBucketPolicy(ctx, MinioBucket, policy)
	if err != nil {
		log.Printf("设置存储桶公开策略失败: %v", err)
		// 不返回错误，因为这不是致命错误
	} else {
		log.Printf("存储桶 '%s' 已设置为公开读取", MinioBucket)
	}

	log.Println("MinIO 连接成功")
	return nil
}

// GenerateUploadURL 生成预签名上传URL
// 参数：文件名（应该是UUID生成的唯一文件名）
// 返回：预签名URL和错误
func GenerateUploadURL(filename string) (string, error) {
	ctx := context.Background()

	// 生成预签名PUT URL
	presignedURL, err := mc.PresignedPutObject(ctx, MinioBucket, filename, UploadURLExpiry)
	if err != nil {
		return "", err
	}

	return presignedURL.String(), nil
}

// GenerateDownloadURL 生成永久下载URL
// 参数：文件名
// 返回：永久URL和错误
func GenerateDownloadURL(filename string) (string, error) {
	// 方案1：生成永久的公开URL（推荐用于视频播放）
	// 格式：http://minio-server:port/bucket-name/filename
	permanentURL := fmt.Sprintf("http://%s/%s/%s", MinioEndpoint, MinioBucket, filename)
	return permanentURL, nil
	
	// 方案2：如果需要预签名URL，可以设置更长的过期时间
	// ctx := context.Background()
	// reqParams := make(url.Values)
	// reqParams.Set("response-content-disposition", "inline")
	// presignedURL, err := mc.PresignedGetObject(ctx, MinioBucket, filename, 365*24*time.Hour, reqParams) // 1年有效期
	// if err != nil {
	//     return "", err
	// }
	// return presignedURL.String(), nil
}

// CheckFileExists 检查文件是否存在于MinIO中
// 参数：文件名
// 返回：是否存在和错误
func CheckFileExists(filename string) (bool, error) {
	ctx := context.Background()

	// 获取文件信息，如果文件不存在会返回错误
	_, err := mc.StatObject(ctx, MinioBucket, filename, minio.StatObjectOptions{})
	if err != nil {
		// 检查是否是"文件不存在"的错误
		errResponse := minio.ToErrorResponse(err)
		if errResponse.Code == "NoSuchKey" {
			return false, nil // 文件不存在，但不是错误
		}
		return false, err // 其他错误
	}

	return true, nil // 文件存在
}

