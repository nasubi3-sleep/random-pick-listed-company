package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// 認証用構造体
type AuthRequest struct {
	MailAddress string `json:"mailaddress"`
	Password    string `json:"password"`
}

type AuthResponse struct {
	RefreshToken string `json:"refreshToken"`
}

type IDTokenResponse struct {
	IDToken string `json:"idToken"`
}

// J-Quants API用構造体
type JQuantsResponse struct {
	Info []struct {
		Code             string `json:"Code"`
		CompanyName      string `json:"CompanyName"`
		MarketCodeName   string `json:"MarketCodeName"`
		Sector33CodeName string `json:"Sector33CodeName"`
	} `json:"info"`
}

// リフレッシュトークンを取得
func GetRefreshToken(email, password string) (string, error) {
	url := "https://api.jquants.com/v1/token/auth_user"
	requestBody := AuthRequest{
		MailAddress: email,
		Password:    password,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request data: %v", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to authenticate: status code %d", resp.StatusCode)
	}

	var authResponse AuthResponse
	err = json.NewDecoder(resp.Body).Decode(&authResponse)
	if err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	return authResponse.RefreshToken, nil
}

// IDトークンを取得
func GetIDToken(refreshToken string) (string, error) {
	url := fmt.Sprintf("https://api.jquants.com/v1/token/auth_refresh?refreshtoken=%s", refreshToken)

	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get ID token: status code %d", resp.StatusCode)
	}

	var idTokenResponse IDTokenResponse
	err = json.NewDecoder(resp.Body).Decode(&idTokenResponse)
	if err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	return idTokenResponse.IDToken, nil
}

// J-Quants APIから銘柄情報を取得
func FetchCompanyInfo(code string, idToken string) (JQuantsResponse, error) {
	apiURL := "https://api.jquants.com/v1/listed/info"

	// APIリクエストの作成
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return JQuantsResponse{}, fmt.Errorf("failed to create request: %v", err)
	}

	// クエリパラメータとヘッダーを設定
	q := req.URL.Query()
	q.Add("code", code)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", idToken))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return JQuantsResponse{}, fmt.Errorf("failed to fetch data: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return JQuantsResponse{}, fmt.Errorf("failed to fetch data: status code %d", resp.StatusCode)
	}

	var result JQuantsResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return JQuantsResponse{}, fmt.Errorf("failed to parse response: %v", err)
	}

	return result, nil
}

func handler(ctx context.Context, event Event) (interface{}, error) {
	// 環境変数から認証情報を取得
	email := os.Getenv("JQUANTS_EMAIL")
	password := os.Getenv("JQUANTS_PASSWORD")

	if email == "" || password == "" {
		return "", fmt.Errorf("email or password not set in environment variables")
	}

	// リフレッシュトークンを取得
	refreshToken, err := GetRefreshToken(email, password)
	if err != nil {
		return "", fmt.Errorf("failed to get refresh token: %v", err)
	}

	// IDトークンを取得
	idToken, err := GetIDToken(refreshToken)
	if err != nil {
		return "", fmt.Errorf("failed to get ID token: %v", err)
	}

	// S3の設定
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("ap-northeast-1"),
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
		for i := 1; i < 3847; i++ {
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

	// J-Quants APIを呼び出して銘柄情報を取得
	info, err := FetchCompanyInfo(randomCode, idToken)
	if err != nil {
		return "", fmt.Errorf("failed to fetch company info: %v", err)
	}

	// 結果をフォーマットして返す
	if len(info.Info) > 0 {
		company := info.Info[0]
		// company をそのまま返す
		return company, nil
	}

	return nil, fmt.Errorf("No company information found")
}

func main() {
	lambda.Start(handler)
}