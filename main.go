package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-gorp/gorp"
	_ "github.com/go-sql-driver/mysql"
)

/*
0000100 2026-10-15 最初のバージョンを作成
0000200 2026-10-16 deepseek-chatの仕組みを取り込む
0000300 2026-10-17 取り込まれていなかった機能を追加した上でリファクタリングを行う。
*/

const version = "0000300"

var (
	db         *sql.DB
	dbmap      *gorp.DbMap
	systems    map[string]string
	systemKeys []string
)

type QaRecord struct {
	Id             int       `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	Responsetime   int64     `json:"responsetime"`
	Modelname      string    `json:"modelname"`
	Maxtokens      int       `json:"maxtokens"`
	Temperature    float64   `json:"temperature"`
	System         string    `json:"system"`
	Question       string    `json:"question"`
	Answer         string    `json:"answer"`
	Itokens        int       `json:"itokens"`
	Otokens        int       `json:"otokens"`
	StopReason     string    `json:"stop_reason"`
	AttachmentName string    `json:"attachment_name"`
	AttachmentMime string    `json:"attachment_mime"`
	AttachmentData string    `json:"attachment_data"`
}

type OpenRouterModel struct {
	Name    string
	Vendor  string
	Comment string
}

var openRouterModels = []OpenRouterModel{}

func main() {

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

	if err := loadSystemPrompts(); err != nil {
		log.Printf("loadSystemPrompts err: %v", err)
	}
	if err := setupDB(); err != nil {
		log.Printf("setupDB err: %v", err)
	}
	if dbmap == nil {
		log.Println("database not available; continuing with form only")
	} else if db != nil {
		defer db.Close()
	}

	http.HandleFunc("/openrouter", validateJWT(openrouterHandler))
	http.HandleFunc("/", validateJWT(openrouterHandler))
	http.HandleFunc("/login", loginHandler)
	port := os.Getenv("HTTPPORT")
	if port == "" {
		port = "9999"
	}
	addr := ":" + port
	if strings.HasPrefix(port, ":") {
		addr = port
	}
	log.Printf("OpenRouter server started at %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
