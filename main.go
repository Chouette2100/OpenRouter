package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	openrouter "github.com/OpenRouterTeam/go-sdk"
	"github.com/OpenRouterTeam/go-sdk/models/components"

	"github.com/Chouette2100/exsrapi/v2"
)

/*

0000100 2026-10-15 最初のバージョンを作成
*/

const version = "0000100"

func printAnswerAndStats(res *components.ChatResult, startedAt time.Time, elapsed time.Duration) {
	if res == nil {
		log.Println("res is nil")
		return
	}

	if len(res.Choices) == 0 {
		log.Println("No response choices returned.")
		return
	}

	choice := res.Choices[0]
	answer := ""
	content, isSet := choice.Message.Content.GetOrZero()
	if isSet && content.Str != nil {
		answer = *content.Str
	} else if isSet && content.ArrayOfChatContentItems != nil {
		for _, item := range content.ArrayOfChatContentItems {
			if item.ChatContentText != nil {
				answer += item.ChatContentText.Text
			} else if item.ChatContentImage != nil {
				answer += "[image]"
			} else {
				answer += "[content]"
			}
		}
	}

	log.Println("========================================")
	log.Println("OpenRouter API response")
	log.Println("========================================")
	log.Printf("時刻: %s\n", startedAt.Format(time.RFC3339))
	log.Printf("応答生成時刻: %s\n", time.Unix(res.Created, 0).Format(time.RFC3339))
	log.Printf("モデル: %s\n", res.Model)
	log.Printf("応答ID: %s\n", res.ID)
	log.Printf("実行時間: %s\n", elapsed)
	if choice.FinishReason != nil {
		log.Printf("終了理由: %s\n", *choice.FinishReason)
	}

	if res.Usage != nil {
		log.Printf("入力トークン: %d\n", res.Usage.PromptTokens)
		log.Printf("出力トークン: %d\n", res.Usage.CompletionTokens)
		log.Printf("合計トークン: %d\n", res.Usage.TotalTokens)

		if cost, ok := res.Usage.Cost.GetOrZero(); ok && !res.Usage.Cost.IsNull() {
			log.Printf("利用料金: %.8f\n", cost)
		}
	}

	log.Println("----------------------------------------")
	log.Println("回答:")
	log.Println(answer)
	log.Println("----------------------------------------")
}

func main() {
	// -------------------------------------------
	// ログファイルをオープンする
	logFileName := fmt.Sprintf("OpenRouter_%s_%s.log",
		version, time.Now().Format("20060102"))
	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %s", err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)
	log.Printf("********** Starting OpenRouter application...\n")

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		type Config struct {
			Apikey string `yaml:"apikey"`
		}
		config := Config{}
		err = exsrapi.LoadConfig("config.enc.yaml", &config)
		if err != nil {
			log.Printf("err=%s.\n", err.Error())
			log.Println("OpenRouter API key is not set. " +
				"Set OPENROUTER_API_KEY or set SOPS_AGE_KEY_FILE for config.enc.yaml.")
			os.Exit(1)
		}
		apiKey = config.Apikey
	}
	// -------------------------------------------

	ctx := context.Background()
	startedAt := time.Now()

	s := openrouter.New(
		openrouter.WithSecurity(apiKey),
	)

	model := "deepseek/deepseek-v4.1-flash"
	res, err := s.Chat.Send(ctx, components.ChatRequest{
		Model: &model,
		Messages: []components.ChatMessages{
			components.CreateChatMessagesUser(
				components.ChatUserMessage{
					Content: components.CreateChatUserMessageContentStr(
						"What is the capital of France?",
					),
					Role: components.ChatUserMessageRoleUser,
				},
			),
		},
	}, nil)
	if err != nil {
		log.Fatal(err)
	}

	elapsed := time.Since(startedAt)
	if res != nil {
		if res.ChatResult != nil {
			printAnswerAndStats(res.ChatResult, startedAt, elapsed)
		} else if res.EventStream != nil {
			log.Println("Stream response received; use the streaming API to read chunks.")
		}
	}
}
