# 使用するGoの公式Dockerイメージ
FROM golang:1.19

# 必要なパッケージをインストール
RUN apt-get update && apt-get install -y \
  awscli \
  unzip \
  curl \
  && rm -rf /var/lib/apt/lists/*

# AWS CLIとGoのインストール
RUN curl "https://dl.min.io/client/mc/release/linux-amd64/mc" -o /usr/local/bin/mc && \
  chmod +x /usr/local/bin/mc

# Goワークスペースの作成
ENV GOPATH=/go
ENV GOROOT=/usr/local/go
ENV PATH=$PATH:$GOROOT/bin:$GOPATH/bin

# 必要なGoライブラリのインストール
RUN go get github.com/aws/aws-sdk-go \
    && go get github.com/360EntSecGroup-Skylar/excelize/v2
