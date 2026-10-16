package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Chouette2100/exsrapi/v2"
)

type pageData struct {
	SelectedSystem string
	SelectedModel  string
	SystemKeys     []string
	ModelList      []OpenRouterModel
	Records        []QaRecord
	SelectedIDs    map[int]bool
	Target         string
	StartID        int
	HasPrevious    bool
	HasNext        bool
	Question       string
	Info           string
	Error          string
}

func parseSelectedIDs(vals []string) ([]int, map[int]bool) {
	ids := make([]int, 0, len(vals))
	selected := make(map[int]bool, len(vals))
	for _, v := range vals {
		id, err := strconv.Atoi(v)
		if err != nil || id <= 0 {
			continue
		}
		if selected[id] {
			continue
		}
		selected[id] = true
		ids = append(ids, id)
	}
	return ids, selected
}

func buildReferencedIDText(history []QaRecord) string {
	if len(history) == 0 {
		return ""
	}
	ids := make([]string, 0, len(history))
	for _, rec := range history {
		if rec.Id <= 0 {
			continue
		}
		ids = append(ids, strconv.Itoa(rec.Id))
	}
	if len(ids) == 0 {
		return ""
	}
	return "\nwith ID:" + strings.Join(ids, ", ")
}

func openrouterHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil && err != http.ErrNotMultipart {
		log.Printf("ParseMultipartForm warning: %v", err)
	}
	selectedIDs, selectedMap := parseSelectedIDs(r.Form["historyid"])

	if err := loadSystemPrompts(); err != nil {
		log.Printf("loadSystemPrompts error: %v", err)
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
	case "clearchecks":
		selectedIDs = nil
		selectedMap = map[int]bool{}
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
		selectedModel = defaultModelName()
	}
	question := r.FormValue("question")
	target := r.FormValue("target")
	if question != "" {
		startedAt := time.Now()
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			type Config struct {
				Apikey string `yaml:"apikey"`
			}
			config := Config{}
			err := exsrapi.LoadConfig("config.enc.yaml", &config)
			if err != nil {
				http.Error(w, "OPENROUTER_API_KEY is not set", http.StatusUnauthorized)
				log.Printf("failed to load config: %v", err)
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
		history := make([]QaRecord, 0, len(selectedIDs))
		if dbmap != nil {
			if len(selectedIDs) > 0 {
				if qList, err := selectRecordsByIDs(selectedIDs); err == nil {
					history = qList
				} else {
					log.Printf("selectRecordsByIDs failed: %v", err)
				}
			}
		}
		answer, inTok, outTok, totalTok, err := askOpenRouter(apiKey, selectedModel, systems[selectedSystem], question, history, maxTokens, tempVal)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		rec := QaRecord{
			Timestamp:    startedAt,
			Modelname:    selectedModel,
			Maxtokens:    maxTokens,
			Temperature:  tempVal,
			System:       systems[selectedSystem],
			Question:     question,
			Answer:       answer,
			Itokens:      inTok,
			Otokens:      outTok,
			StopReason:   "stop",
			Responsetime: time.Since(startedAt).Milliseconds(),
		}
		if len(selectedIDs) > 0 {
			rec.Question += buildReferencedIDText(history)
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
			if selectedMap == nil {
				selectedMap = map[int]bool{}
			}
			selectedMap[rec.Id] = true
			currentID = maxID + 1
		}
		question = ""
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
		SelectedIDs:    selectedMap,
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
	tpl, err := template.New("templates/openrouter.gtpl").Funcs(funcMap).ParseFiles("templates/openrouter.gtpl")
	if err != nil {
		log.Printf("template parse error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tpl.ExecuteTemplate(w, "openrouter.gtpl", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
