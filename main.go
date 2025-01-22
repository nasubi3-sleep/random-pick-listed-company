package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"bytes"
	"io"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/tealeg/xlsx"
)

type Event struct {
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
}

func handler(ctx context.Context, event Event) (string, error) {
	// S3の設定
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("ap-northeast-1"), // 必要に応じて変更
	})
	if err != nil {
		return "", fmt.Errorf("failed to create AWS session: %v", err)
	}

	s3Client := s3.New(sess)

	// S3からオブジェクトを取得
	result, err := s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(event.Bucket),
		Key:    aws.String(event.Key),
	})
	if err != nil {
		return "", fmt.Errorf("failed to get object from S3: %v", err)
	}
	defer result.Body.Close()

	// S3オブジェクトをバイトスライスに読み込む
	var buffer bytes.Buffer
	_, err = io.Copy(&buffer, result.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read S3 object body: %v", err)
	}

	// Excelファイルをパース
	xlFile, err := xlsx.OpenBinary(buffer.Bytes())
	if err != nil {
		return "", fmt.Errorf("failed to parse Excel file: %v", err)
	}

	// B2～B3847を抽出
	var values []string
	for _, sheet := range xlFile.Sheets {
		for i := 1; i < 3847; i++ { // 行は0インデックスベース
			cell := sheet.Cell(i, 1) // B列は1インデックス
			if cell != nil {
				values = append(values, cell.String())
			}
		}
		break // 最初のシートのみ処理
	}

	if len(values) == 0 {
		return "", fmt.Errorf("no values found in specified range")
	}

	// ランダムに値を選択
	rand.Seed(time.Now().UnixNano())
	randomValue := values[rand.Intn(len(values))]

	return randomValue, nil
}

func main() {
	lambda.Start(handler)
}
