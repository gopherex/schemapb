package schemapb_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	schemapb "github.com/gopherex/schemapb/go/schemapb"
)

// catalog.json pins translation across ports: for every error of the
// kitchen-sink result (all templated codes, rules with and without an id),
// the catalog key and the message rendered from a partial catalog with the
// default display. A key absent from the catalog falls back to message.
var testCatalog = schemapb.Catalog{
	"ERROR_CODE_GTE_VIOLATED":     "должно быть не меньше {expected} (сейчас {actual})",
	"ERROR_CODE_PATTERN_MISMATCH": "не соответствует шаблону {expected}",
	"ERROR_CODE_UNKNOWN_VARIANT":  "неизвестный вариант {actual}",
	"ERROR_CODE_REQUIRED_MISSING": "обязательное поле",
	"cross":                       "i64 должно покрывать i32",
	"no-ssh":                      "порт 22 запрещён",
}

func TestGoldenCatalog(t *testing.T) {
	t.Parallel()

	// The input is the committed full-errors.json (written by its own
	// golden test), so every port reads the very same errors.
	raw, err := os.ReadFile(filepath.Join(goldenDir, "full-errors.json"))
	if err != nil {
		t.Fatal(err)
	}

	res := &schemapb.ValidationResult{}
	if err := protojson.Unmarshal(raw, res); err != nil {
		t.Fatal(err)
	}

	type entry struct {
		Error   json.RawMessage `json:"error"`
		Key     string          `json:"key"`
		Message string          `json:"message"`
	}

	doc := struct {
		Catalog schemapb.Catalog `json:"catalog"`
		Cases   []entry          `json:"cases"`
	}{Catalog: testCatalog}

	for _, e := range res.GetErrors() {
		one, err := protojson.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}

		doc.Cases = append(doc.Cases, entry{
			Error:   one,
			Key:     schemapb.MessageKey(e),
			Message: testCatalog.Message(e, nil),
		})
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	checkGolden(t, "catalog.json", append(out, '\n'))
}
