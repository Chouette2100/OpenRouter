package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Chouette2100/srdblib/v3"
	"github.com/go-gorp/gorp"
)

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
	dbmap.AddTableWithName(User{}, "user").SetKeys(true, "Email")
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

func selectRecordsByIDs(ids []int) ([]QaRecord, error) {
	if dbmap == nil {
		return nil, fmt.Errorf("DB not initialized")
	}
	if len(ids) == 0 {
		return []QaRecord{}, nil
	}

	uniq := make(map[int]struct{}, len(ids))
	normalized := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := uniq[id]; exists {
			continue
		}
		uniq[id] = struct{}{}
		normalized = append(normalized, id)
	}
	if len(normalized) == 0 {
		return []QaRecord{}, nil
	}
	sort.Ints(normalized)

	records := make([]QaRecord, 0, len(normalized))
	for _, id := range normalized {
		var record QaRecord
		itf, err := dbmap.Get(&record, id)
		if err != nil {
			return nil, err
		}
		if itf == nil {
			continue
		}
		records = append(records, *(itf.(*QaRecord)))
	}
	return records, nil
}
