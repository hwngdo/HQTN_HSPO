package hspo

import (
	"reflect"
	"strings"
	"testing"
)

func TestChemicalExceptionMatchesOnlyReferencedMembership(t *testing.T) {
	records := []chemicalRecord{{CAS: "100-37-8", Memberships: []Membership{
		{SourceCode: "ND24_2026", Appendix: "II", SourceSTT: "265."},
		{SourceCode: "ND24_2026", Appendix: "III", SourceSTT: "33."},
	}}}
	markChemicalExceptions(records, []string{"ND24_2026|III|33|100-37-8"})
	row := matchChemicals("100-37-8", records)[0]
	if row.Memberships[0].Relationship != "" || row.Memberships[1].Relationship != "exception" {
		t.Fatalf("unexpected relationships: %+v", row.Memberships)
	}
	warnings := strings.Join(row.Warnings, "\n")
	if !strings.Contains(warnings, "đoạn ngoại trừ") || strings.Contains(warnings, "Có mục Phụ lục III") {
		t.Fatalf("unexpected warnings: %s", warnings)
	}
}

func TestMatchChemicals(t *testing.T) {
	records := []chemicalRecord{
		{CAS: "67-64-1", Name: "Axêtôn", IUPAC: "Propanone", Aliases: "Acetone | common alias", Memberships: []Membership{{Appendix: "II", SourceSTT: "7."}}},
		{CAS: "64-17-5", Name: "Ethanol", Aliases: "common alias"},
	}
	tests := []struct {
		query string
		cas   []string
		mode  []string
	}{
		{"AXETON", []string{"67-64-1"}, []string{"INFERRED_NAME"}},
		{"Thành phần: propanone; ethanol", []string{"67-64-1", "64-17-5"}, []string{"INFERRED_NAME", "INFERRED_NAME"}},
		{"67 – 64 – 1\nAcetone", []string{"67-64-1"}, []string{"DECLARED"}},
		{"common alias", []string{"67-64-1", "64-17-5"}, []string{"INFERRED_NAME", "INFERRED_NAME"}},
		{"unknownacetone", nil, nil},
	}
	for _, test := range tests {
		t.Run(test.query, func(t *testing.T) {
			var cas, modes []string
			for _, row := range matchChemicals(test.query, records) {
				cas = append(cas, row.CAS)
				modes = append(modes, row.SourceType)
			}
			if !reflect.DeepEqual(cas, test.cas) || !reflect.DeepEqual(modes, test.mode) {
				t.Fatalf("got CAS %v and sources %v", cas, modes)
			}
		})
	}
	byName := matchChemicals("common alias", records)
	if len(byName[0].Warnings) != 2 || len(byName[1].Warnings) != 1 {
		t.Fatal("expected ambiguity warning for both candidates and catalog warning for the listed candidate")
	}
}

func TestMatchChemicalsRanksSimilarNamesAfterExactMatch(t *testing.T) {
	records := []chemicalRecord{
		{CAS: "74-82-8", Name: "Metan", Aliases: "Methane"},
		{CAS: "64-17-5", Name: "Etanol", Aliases: "Ethanol"},
		{CAS: "67-56-1", Name: "Metanol", Aliases: "Methanol"},
	}
	rows := matchChemicals("metanol", records)
	if len(rows) != 3 ||
		rows[0].CAS != "67-56-1" || rows[0].SourceType != "INFERRED_NAME" ||
		rows[1].CAS != "64-17-5" || rows[1].SourceType != "SIMILAR_NAME" ||
		rows[2].CAS != "74-82-8" || rows[2].SourceType != "SIMILAR_NAME" ||
		len(rows[1].Warnings) == 0 {
		t.Fatalf("unexpected results: %+v", rows)
	}
	rows = matchChemicals("67-56-1", records)
	if len(rows) != 1 || rows[0].SourceType != "DECLARED" {
		t.Fatalf("explicit CAS should produce only an exact result: %+v", rows)
	}
	rows = matchChemicals("55965-84-9\n7732-18-5\nmetanol", records)
	if len(rows) != 5 || rows[0].CAS != "67-56-1" || rows[1].CAS != "64-17-5" ||
		rows[2].CAS != "74-82-8" || rows[3].CAS != "55965-84-9" || rows[4].CAS != "7732-18-5" ||
		rows[3].SourceType != "DECLARED" || rows[4].SourceType != "DECLARED" {
		t.Fatalf("mixed input should rank name results before other declared codes: %+v", rows)
	}
}
