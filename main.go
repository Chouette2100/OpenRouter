package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Chouette2100/srdblib/v3"
	"github.com/go-gorp/gorp"
	_ "github.com/go-sql-driver/mysql"
	"gopkg.in/yaml.v2"

	"github.com/Chouette2100/exsrapi/v2"
)

/*
0000100 2026-10-15 最初のバージョンを作成
0000200 2026-10-16 deepsek-chatの仕組みを取り込む
*/

const version = "0000200"

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

var openRouterModels = []OpenRouterModel{
	{Name: "deepseek/deepseek-v4.1-flash", Vendor: "DeepSeek", Comment: "高速/安定"},
	{Name: "openai/gpt-4o-mini", Vendor: "OpenAI", Comment: "軽量"},
	{Name: "openai/gpt-4.1-mini", Vendor: "OpenAI", Comment: "バランス"},
	{Name: "anthropic/claude-3.5-sonnet", Vendor: "Anthropic", Comment: "長文"},
	{Name: "google/gemini-2.5-flash", Vendor: "Google", Comment: "高速"},
	{Name: "meta-llama/llama-3.3-70b-instruct", Vendor: "Meta", Comment: "汎用"},
}

type openRouterResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Created int64  `json:"created"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

type pageData struct {
	SelectedSystem string
	SelectedModel  string
	SystemKeys     []string
	ModelList      []OpenRouterModel
	Records        []QaRecord
	Target         string
	StartID        int
	HasPrevious    bool
	HasNext        bool
	Question       string
	Info           string
	Error          string
}

func readFileWithSops(path string) ([]byte, error) {
	if strings.Contains(filepath.Base(path), ".enc.") {
		if os.Getenv("SOPS_AGE_KEY_FILE") == "" {
			return nil, fmt.Errorf("SOPS_AGE_KEY_FILE is empty")
		}
		out, err := exec.Command("sops", "--decrypt", path).Output()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return nil, fmt.Errorf("sops --decrypt failed: %w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
			}
			return nil, fmt.Errorf("sops --decrypt failed: %w", err)
		}
		return out, nil
	}
	return os.ReadFile(path)
}

func loadSystemPrompts() error {
	candidates := []string{
		filepath.Join(".", "systemlist.enc.yml"),
		filepath.Join(".", "systemlist.yml"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			content, err := readFileWithSops(p)
			if err != nil {
				return err
			}
			var cfg struct {
				Systemlist yaml.MapSlice `yaml:"systemlist"`
			}
			if err := yaml.Unmarshal(content, &cfg); err != nil {
				return err
			}
			systems = map[string]string{}
			systemKeys = make([]string, 0, len(cfg.Systemlist))
			for _, item := range cfg.Systemlist {
				key, ok := item.Key.(string)
				if !ok {
					continue
				}
				val, ok := item.Value.(string)
				if !ok {
					continue
				}
				systems[key] = val
				systemKeys = append(systemKeys, key)
			}
			if len(systemKeys) > 0 {
				return nil
			}
		}
	}
	systems = map[string]string{"default": "You are a helpful assistant."}
	systemKeys = []string{"default"}
	return nil
}

func findDBConfigPath() string {
	candidates := []string{
		filepath.Join(".", "DBConfig.enc.yml"),
		filepath.Join(".", "DBConfig_local.enc.yml"),
		filepath.Join(".", "DBConfig_9901.enc.yml"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func setupDB() error {
	cfgPath := findDBConfigPath()
	if cfgPath == "" {
		return fmt.Errorf("DB config file not found in OpenRouter directory")
	}
	conn, cfg, err := srdblib.OpenDb(cfgPath)
	if err != nil {
		return err
	}
	if cfg.UseSSH {
		defer srdblib.Dialer.Close()
	}
	db = conn
	dbmap = &gorp.DbMap{
		Db:              db,
		Dialect:         gorp.MySQLDialect{Engine: "InnoDB", Encoding: "utf8mb4"},
		ExpandSliceArgs: true,
	}
	dbmap.AddTableWithName(QaRecord{}, "qa_records").SetKeys(true, "Id")
	return nil
}

func selectRecords(startID, limit int, target string) ([]QaRecord, error) {
	if dbmap == nil {
		return nil, fmt.Errorf("DB not initialized")
	}
	if target == "" {
		var rows []interface{}
		var err error
		rows, err = dbmap.Select(&QaRecord{}, "SELECT id, timestamp, responsetime, modelname, maxtokens, temperature, `system`, question, answer, itokens, otokens, stopreason, attachmentname, attachmentmime, attachmentdata FROM qa_records WHERE id <= ? ORDER BY id DESC LIMIT ?", startID, limit)
		if err != nil {
			return nil, err
		}
		records := make([]QaRecord, 0, len(rows))
		for _, v := range rows {
			records = append(records, *(v.(*QaRecord)))
		}
		return records, nil
	}
	q := "%" + target + "%"
	var rows []interface{}
	rows, err := dbmap.Select(&QaRecord{}, "SELECT id, timestamp, responsetime, modelname, maxtokens, temperature, `system`, question, answer, itokens, otokens, stopreason, attachmentname, attachmentmime, attachmentdata FROM qa_records WHERE id <= ? AND (question LIKE ? OR answer LIKE ?) ORDER BY id DESC LIMIT ?", startID, q, q, limit)
	if err != nil {
		return nil, err
	}
	records := make([]QaRecord, 0, len(rows))
	for _, v := range rows {
		records = append(records, *(v.(*QaRecord)))
	}
	return records, nil
}

func getMaxID() int {
	var maxID int64
	if err := dbmap.SelectOne(&maxID, "SELECT MAX(id) FROM qa_records"); err != nil || maxID == 0 {
		return 0
	}
	return int(maxID)
}

func buildMessages(systemPrompt string, history []QaRecord, question string) []map[string]string {
	messages := make([]map[string]string, 0, len(history)*2+2)
	if systemPrompt != "" {
		messages = append(messages, map[string]string{"role": "system", "content": systemPrompt})
	}
	for _, rec := range history {
		if rec.Question != "" {
			messages = append(messages, map[string]string{"role": "user", "content": rec.Question})
		}
		if rec.Answer != "" {
			messages = append(messages, map[string]string{"role": "assistant", "content": rec.Answer})
		}
	}
	messages = append(messages, map[string]string{"role": "user", "content": question})
	return messages
}

func askOpenRouter(apiKey, model, systemPrompt, question string, history []QaRecord, maxTokens int, temperature float64) (string, int, int, int, error) {
	if apiKey == "" {
		return "", 0, 0, 0, fmt.Errorf("OPENROUTER_API_KEY is empty")
	}
	if model == "" {
		model = "deepseek/deepseek-v4.1-flash"
	}
	payload := map[string]interface{}{
		"model":       model,
		"messages":    buildMessages(systemPrompt, history, question),
		"max_tokens":  maxTokens,
		"temperature": temperature,
	}
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return "", 0, 0, 0, err
	}

	req, err := http.NewRequest(http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", 0, 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("HTTP-Referer", "http://localhost")
	req.Header.Set("X-Title", "OpenRouter Chat")

	resp, err := (&http.Client{Timeout: 180 * time.Second}).Do(req)
	if err != nil {
		return "", 0, 0, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, 0, 0, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, 0, 0, fmt.Errorf("OpenRouter API Error %d: %s", resp.StatusCode, string(body))
	}
	var result openRouterResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", 0, 0, 0, err
	}
	if result.Error != nil && result.Error.Message != "" {
		return "", 0, 0, 0, fmt.Errorf("OpenRouter: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", 0, 0, 0, fmt.Errorf("OpenRouter returned no choices")
	}
	answer := result.Choices[0].Message.Content
	inputTokens := result.Usage.PromptTokens
	outputTokens := result.Usage.CompletionTokens
	totalTokens := result.Usage.TotalTokens

	log.Println("========================================")
	log.Println("OpenRouter API response")
	log.Println("========================================")
	// log.Printf("時刻: %s\n", startedAt.Format(time.RFC3339))
	log.Printf("応答生成時刻: %s\n", time.Unix(result.Created, 0).Format(time.RFC3339))
	log.Printf("モデル: %s\n", result.Model)
	log.Printf("応答ID: %s\n", result.ID)
	// log.Printf("実行時間: %s\n", elapsed)
	// if choice.FinishReason != nil {
	// 	log.Printf("終了理由: %s\n", *choice.FinishReason)
	// }

	// if result.Usage != nil {
		log.Printf("入力トークン: %d\n", result.Usage.PromptTokens)
		log.Printf("出力トークン: %d\n", result.Usage.CompletionTokens)
		log.Printf("合計トークン: %d\n", result.Usage.TotalTokens)

	//	if cost, ok := result.Usage.Cost.GetOrZero(); ok && !result.Usage.Cost.IsNull() {
	//		log.Printf("利用料金: %.8f\n", cost)
	// }

	log.Println("----------------------------------------")
	log.Println("回答:")
	log.Println(answer)
	log.Println("----------------------------------------")

	return answer, inputTokens, outputTokens, totalTokens, nil
}

func pageHandler(w http.ResponseWriter, r *http.Request) {
	if err := loadSystemPrompts(); err != nil {
		log.Printf("loadSystemPrompts error: %v", err)
	}
	if err := setupDB(); err != nil {
		log.Printf("setupDB error: %v", err)
	}
	if db != nil {
		defer db.Close()
	}

	pageSize := 20
	currentID, _ := strconv.Atoi(r.FormValue("startid"))
	maxID := 0
	if dbmap != nil {
		maxID = getMaxID()
	}
	if currentID <= 0 {
		currentID = maxID
	}
	switch r.FormValue("action") {
	case "next":
		currentID -= pageSize
	case "previous":
		currentID += pageSize
	}
	if currentID < 1 {
		currentID = 1
	}
	if currentID > maxID {
		currentID = maxID
	}

	selectedSystem := r.FormValue("system")
	if selectedSystem == "" && len(systemKeys) > 0 {
		selectedSystem = systemKeys[0]
	}
	selectedModel := r.FormValue("model")
	if selectedModel == "" {
		selectedModel = "deepseek/deepseek-v4.1-flash"
	}
	question := r.FormValue("question")
	target := r.FormValue("target")
	if question != "" {
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			type Config struct {
				Apikey string `yaml:"apikey"`
			}
			config := Config{}
			err := exsrapi.LoadConfig("config.enc.yaml", &config)
			if err != nil {
				http.Error(w, "OPENROUTER_API_KEY is not set", http.StatusUnauthorized)
				err = fmt.Errorf("failed to load config: %v", err)
				return
			}
			apiKey = config.Apikey
		}
		maxTokens, _ := strconv.Atoi(r.FormValue("maxtokens"))
		if maxTokens <= 0 {
			maxTokens = 2000
		}
		tempVal, _ := strconv.ParseFloat(r.FormValue("temperature"), 64)
		if tempVal == 0 {
			tempVal = 0.2
		}
		history := make([]QaRecord, 0, 5)
		if dbmap != nil {
			if qList, err := selectRecords(currentID, 5, ""); err == nil {
				history = qList
			}
		}
		answer, inTok, outTok, totalTok, err := askOpenRouter(apiKey, selectedModel, systems[selectedSystem], question, history, maxTokens, tempVal)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		rec := QaRecord{
			Timestamp:    time.Now(),
			Modelname:    selectedModel,
			Maxtokens:    maxTokens,
			Temperature:  tempVal,
			System:       systems[selectedSystem],
			Question:     question,
			Answer:       answer,
			Itokens:      inTok,
			Otokens:      outTok,
			StopReason:   "stop",
			Responsetime: 0,
		}
		if totalTok > 0 {
			rec.Otokens = outTok
			rec.Itokens = inTok
		}
		if dbmap != nil {
			if err := dbmap.Insert(&rec); err != nil {
				log.Printf("insert failed: %v", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			currentID = maxID + 1
		}
	}

	records := make([]QaRecord, 0, pageSize)
	if dbmap != nil {
		var err error
		records, err = selectRecords(currentID, pageSize, target)
		if err != nil {
			log.Printf("selectRecords failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	data := pageData{
		SelectedSystem: selectedSystem,
		SelectedModel:  selectedModel,
		SystemKeys:     systemKeys,
		ModelList:      openRouterModels,
		Records:        records,
		Target:         target,
		StartID:        currentID,
		Question:       question,
		HasPrevious:    currentID < maxID,
		HasNext:        len(records) == pageSize && currentID > 1,
	}
	if maxID == 0 || len(records) == 0 {
		data.HasPrevious = false
		data.HasNext = false
	}

	funcMap := template.FuncMap{
		"add":           func(a, b int) int { return a + b },
		"TimeToStringY": func(t time.Time) string { return t.Format("06-01-02 15:04") },
		"msToSeconds":   func(n int64) float64 { return float64(n) / 1000.0 },
	}
	tpl := template.Must(template.New("page").Funcs(funcMap).Parse(`
	<!DOCTYPE html>
	<html lang="ja">
	<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>OpenRouter Chat</title>
	<script src="https://cdn.jsdelivr.net/npm/marked/marked.min.js"></script>
	<script src="https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.min.js"></script>
	<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/katex@0.16.9/dist/katex.min.css">
	<script src="https://cdn.jsdelivr.net/npm/katex@0.16.9/dist/katex.min.js"></script>
	<script src="https://cdn.jsdelivr.net/npm/katex@0.16.9/dist/contrib/auto-render.min.js"></script>
	<style>
	 body { font-family: system-ui, sans-serif; background: #f6f8ff; color: #1e2430; margin: 0; padding: 24px; }
	 .container { max-width: 1200px; margin: 0 auto; background: #fff; border-radius: 14px; padding: 22px; box-shadow: 0 10px 30px rgba(46,94,169,0.08); }
	 h1 { margin-top: 0; color: #1a4c8f; }
	 .panel { background: #f7f9fe; border: 1px solid #dfe7f8; border-radius: 10px; padding: 16px; margin-bottom: 18px; }
	 .row { display: flex; gap: 12px; flex-wrap: wrap; align-items: center; margin-bottom: 12px; }
	 label { font-weight: 600; color: #32507f; }
	 select, input, textarea, button { font: inherit; }
	 select, input, textarea { border: 1px solid #cfdcf8; padding: 8px 10px; border-radius: 8px; background: white; }
	 textarea { width: 62em; max-width: 85vw; min-height: 48px; }
	 button { background: linear-gradient(180deg, #2d6cdf 0%, #2257b5 100%); color: white; border: none; border-radius: 8px; padding: 8px 14px; cursor: pointer; }
	 button.secondary { background: linear-gradient(180deg, #8293b5 0%, #677ca0 100%); }
	 .history { border-top: 1px solid #e4ecff; margin-top: 16px; padding-top: 16px; }
	 .meta { font-size: 12px; color: #57709a; margin-bottom: 8px; }
	 .markdown { display: none; border: 1px solid #dfe8f8; border-radius: 8px; background: #fbfcff; width: 62em; max-width: 85vw; padding: 14px; box-sizing: border-box; overflow-wrap: anywhere; }
	 .markdown pre { overflow-x: auto; background: #eef4ff; padding: 10px; border-radius: 6px; }
	 .markdown code { background: #edf4ff; padding: 2px 6px; border-radius: 4px; }
	</style>
	</head>
	<body>
	<div class="container">
	<h1>OpenRouter Chat</h1>
	<form method="POST" action="/openrouter" enctype="multipart/form-data">
	<div class="panel">
	<div class="row">
	<label>System</label>
	<select name="system">
	{{ range .SystemKeys }}
	<option value="{{ . }}" {{ if eq $.SelectedSystem . }}selected{{ end }}>{{ . }}</option>
	{{ end }}
	</select>
	<label>Model</label>
	<select name="model">
	{{ range .ModelList }}
	<option value="{{ .Name }}" {{ if eq $.SelectedModel .Name }}selected{{ end }}>{{ .Name }} ({{ .Vendor }})</option>
	{{ end }}
	</select>
	<label>Max Tokens</label>
	<input type="number" name="maxtokens" value="2000" style="width: 7em;">
	<label>Temperature</label>
	<input type="number" name="temperature" value="0.2" min="0" max="2" step="0.1" style="width: 6em;">
	</div>
	<div class="row">
	<label>質問</label>
	<textarea name="question" rows="3">{{ .Question }}</textarea>
	</div>
	<div class="row">
	<button type="submit">送信</button>
	</div>
	</div>
	<div class="panel">
	<div class="row">
	<label>検索</label>
	<input type="text" name="target" value="{{ .Target }}" style="width: 30em;">
	<button type="submit" class="secondary">検索</button>
	<label>Start ID</label>
	<input type="number" name="startid" value="{{ .StartID }}" style="width: 8em;">
	<button type="submit" class="secondary">移動</button>
	</div>
	<div class="row">
	{{ if .HasPrevious }}<button type="submit" name="action" value="previous" class="secondary">前へ</button>{{ end }}
	{{ if .HasNext }}<button type="submit" name="action" value="next" class="secondary">次へ</button>{{ end }}
	</div>
	</div>
	{{ if .Error }}<div style="color: #a22828;">{{ .Error }}</div>{{ end }}
	{{ if .Info }}<div style="color: #1d5a2a;">{{ .Info }}</div>{{ end }}
	<div class="history">
	{{ range .Records }}
	<div style="margin-bottom: 18px; padding-bottom: 14px; border-bottom: 1px solid #eef2ff;">
	<div class="meta">id={{ .Id }} / model={{ .Modelname }} / {{ TimeToStringY .Timestamp }} / {{ printf "%5.1f" (msToSeconds .Responsetime) }}s / prompt={{ .Itokens }} / completion={{ .Otokens }}</div>
	<div class="row">
	<textarea readonly rows="3">{{ .Question }}</textarea>
	<button type="button" onclick="toggleMarkdown(this.previousElementSibling, this.nextElementSibling)">TXT</button>
	<div class="markdown"></div>
	</div>
	<div class="row">
	<textarea readonly rows="6">{{ .Answer }}</textarea>
	<button type="button" onclick="toggleMarkdown(this.previousElementSibling, this.nextElementSibling)">TXT</button>
	<div class="markdown"></div>
	</div>
	</div>
	{{ end }}
	</div>
	</form>
	</div>
	<script>
	function toggleMarkdown(textArea, mdBox) {
		const button = textArea.nextElementSibling;
		if (textArea.style.display === 'none') {
			textArea.style.display = 'block';
			mdBox.style.display = 'none';
			button.textContent = 'TXT';
			return;
		}
		let raw = textArea.value;
		const mathBlocks = [];
		raw = raw.replace(/(\\\[[\s\S]*?\\\]|\\\(.*?\\\))|(\$\$[\s\S]*?\$\$|\$.*?\$)/g, (match) => {
			const placeholder = 'MATH_' + mathBlocks.length;
			mathBlocks.push(match);
			return placeholder;
		});
		let html = marked.parse(raw);
		mathBlocks.forEach((math, idx) => {
			html = html.replace('MATH_' + idx, math);
		});
		mdBox.innerHTML = html;
		renderMathInElement(mdBox, {
			delimiters: [
				{ left: '$$', right: '$$', display: true },
				{ left: '$', right: '$', display: false },
				{ left: '\\(', right: '\\)', display: false },
				{ left: '\\[', right: '\\]', display: true }
			],
			throwOnError: false
		});
		textArea.style.display = 'none';
		mdBox.style.display = 'block';
		button.textContent = 'MD';
	}
	</script>
	`))
	if err := tpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

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
	}

	http.HandleFunc("/openrouter", pageHandler)
	http.HandleFunc("/", pageHandler)
	log.Println("OpenRouter server started at :9999")
	log.Fatal(http.ListenAndServe(":9999", nil))
}
