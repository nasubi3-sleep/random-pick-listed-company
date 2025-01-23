package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

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

type JQuantsResponse struct {
	Info []struct {
		Date             string `json:"Date"`
		Code             string `json:"Code"`
		CompanyName      string `json:"CompanyName"`
		CompanyNameEnglish string `json:"CompanyNameEnglish"`
		Sector17Code     string `json:"Sector17Code"`
		Sector17CodeName string `json:"Sector17CodeName"`
		Sector33Code     string `json:"Sector33Code"`
		Sector33CodeName string `json:"Sector33CodeName"`
		ScaleCategory    string `json:"ScaleCategory"`
		MarketCode       string `json:"MarketCode"`
		MarketCodeName   string `json:"MarketCodeName"`
		MarginCode       string `json:"MarginCode"`
		MarginCodeName   string `json:"MarginCodeName"`
	} `json:"info"`
}

// J-Quants APIから会社情報を取得する関数
func getCompanyInfo(code string) (JQuantsResponse, error) {
	apiURL := "https://api.jquants.com/v1/listed/info"

	// APIトークンを環境変数から取得
	accessToken := os.Getenv("JQUANTS_API_TOKEN")
	if accessToken == "" {
		return JQuantsResponse{}, fmt.Errorf("JQUANTS_API_TOKEN is not set in environment variables")
	}

	// APIリクエストの作成
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return JQuantsResponse{}, fmt.Errorf("failed to create request: %v", err)
	}

	// クエリパラメータとヘッダーを設定
	q := req.URL.Query()
	q.Add("code", code)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	// HTTPクライアントでリクエストを送信
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return JQuantsResponse{}, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// レスポンスの確認
	if resp.StatusCode != http.StatusOK {
		return JQuantsResponse{}, fmt.Errorf("failed to fetch data: status code %d", resp.StatusCode)
	}

	// レスポンスのパース
	var response JQuantsResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return JQuantsResponse{}, fmt.Errorf("failed to parse response: %v", err)
	}

	return response, nil
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

	// Excelファイルをパース
	xlFile, err := xlsx.OpenReaderAt(result.Body, result.ContentLength)
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
	randomCode := values[rand.Intn(len(values))]

	// 選択したコードを使用してJ-Quants APIから会社情報を取得
	companyInfo, err := getCompanyInfo(randomCode)
	if err != nil {
		return "", fmt.Errorf("failed to get company info: %v", err)
	}

	// 取得した会社情報をフォーマットしてレスポンスを作成
	if len(companyInfo.Info) > 0 {
		company := companyInfo.Info[0]
		return fmt.Sprintf("Code: %s, Name: %s, Market: %s, Sector: %s",
			company.Code, company.CompanyName, company.MarketCodeName, company.Sector33CodeName), nil
	}

	return fmt.Sprintf("Code: %s, but no company information found in J-Quants API", randomCode), nil
}

func main() {
	lambda.Start(handler)
}
