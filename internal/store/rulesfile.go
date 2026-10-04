package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"

	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/rules/lists"
)

// RulesFile is rules.json: user rules and list metadata.
type RulesFile struct {
	Version int          `json:"version"`
	Rules   []rules.Rule `json:"rules"`
	Lists   []lists.List `json:"lists"`
}

func emptyRules() RulesFile {
	return RulesFile{Version: 1, Rules: []rules.Rule{}, Lists: []lists.List{}}
}

// LoadRules reads rules.json. A missing file is empty; a file that cannot be
// parsed is renamed to rules.json.bak and an empty set is returned with
// recovered=true.
func LoadRules(path string) (RulesFile, bool, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return emptyRules(), false, nil
	}
	if err != nil {
		return emptyRules(), false, err
	}
	f := emptyRules()
	if jerr := json.Unmarshal(bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}), &f); jerr != nil {
		if err := os.Rename(path, path+".bak"); err != nil {
			return emptyRules(), true, err
		}
		return emptyRules(), true, nil
	}
	if f.Rules == nil {
		f.Rules = []rules.Rule{}
	}
	if f.Lists == nil {
		f.Lists = []lists.List{}
	}
	f.Version = 1
	return f, false, nil
}

// SaveRules writes rules.json atomically.
func SaveRules(path string, f RulesFile) error {
	f.Version = 1
	return WriteJSONAtomic(path, f)
}
