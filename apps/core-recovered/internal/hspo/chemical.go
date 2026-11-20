package hspo

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var casPattern = regexp.MustCompile(`\b([0-9]{2,7})[ \t]*[-‐‑‒–—−][ \t]*([0-9]{2})[ \t]*[-‐‑‒–—−][ \t]*([0-9])\b`)

type chemicalRecord struct {
	CAS         string       `json:"cas"`
	Name        string       `json:"preferred_name"`
	IUPAC       string       `json:"preferred_iupac"`
	Formula     string       `json:"preferred_formula"`
	Aliases     string       `json:"aliases"`
	AliasNorms  []string     `json:"alias_norms"`
	Memberships []Membership `json:"memberships"`
}

func (a *App) analyzeChem(w http.ResponseWriter, r *http.Request) {
	out := make([]Chemical, 0)
	if r.Method != http.MethodPost {
		jsonOut(w, http.StatusMethodNotAllowed, out)
		return
	}
	var q struct {
		Text string `json:"text"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&q) != nil {
		jsonOut(w, http.StatusBadRequest, out)
		return
	}
	var records []chemicalRecord
	if err := readJSON(filepath.Join(a.PolicyDir, "chemical_policy.json"), &records); err != nil {
		jsonOut(w, http.StatusServiceUnavailable, map[string]string{"error": "Không đọc được dữ liệu danh mục QA"})
		return
	}
	var exceptions struct {
		References []string `json:"references"`
	}
	if readJSON(filepath.Join(a.StaticDir, "chemical_legal_exceptions.json"), &exceptions) == nil {
		markChemicalExceptions(records, exceptions.References)
	}
	jsonOut(w, http.StatusOK, matchChemicals(q.Text, records))
}

func markChemicalExceptions(records []chemicalRecord, references []string) {
	keys := make(map[string]bool, len(references))
	for _, reference := range references {
		keys[reference] = true
	}
	for _, record := range records {
		for i := range record.Memberships {
			ref := &record.Memberships[i]
			stt := strings.TrimSuffix(strings.TrimSpace(ref.SourceSTT), ".")
			key := strings.Join([]string{ref.SourceCode, ref.Appendix, stt, record.CAS}, "|")
			if keys[key] {
				ref.Relationship = "exception"
			}
		}
	}
}

func matchChemicals(text string, records []chemicalRecord) []Chemical {
	byCAS := make(map[string]chemicalRecord, len(records))
	byName := make(map[string][]string)
	maxWords := 0
	for _, record := range records {
		if record.CAS == "" {
			continue
		}
		byCAS[record.CAS] = record
		names := append([]string{record.Name, record.IUPAC}, strings.Split(record.Aliases, "|")...)
		names = append(names, record.AliasNorms...)
		seen := make(map[string]bool)
		for _, name := range names {
			name = normalizeChemicalName(name)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			byName[name] = append(byName[name], record.CAS)
			maxWords = max(maxWords, strings.Count(name, " ")+1)
		}
	}

	out := make([]Chemical, 0)
	seen := make(map[string]bool)
	declared := make([]string, 0)
	declaredCAS := make(map[string]bool)
	lines := strings.FieldsFunc(text, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ';'
	})
	nameLines := make([]string, 0, len(lines))
	for _, line := range lines {
		matches := casPattern.FindAllStringSubmatch(line, -1)
		if len(matches) == 0 {
			nameLines = append(nameLines, line)
			continue
		}
		for _, m := range matches {
			cas := strings.Join(m[1:], "-")
			if !declaredCAS[cas] {
				declared = append(declared, cas)
				declaredCAS[cas] = true
			}
		}
	}

	for _, line := range nameLines {
		words := strings.Fields(normalizeChemicalName(line))
		for start := 0; start < len(words); {
			end := min(start+maxWords, len(words))
			for ; end > start; end-- {
				name := strings.Join(words[start:end], " ")
				// Short aliases are accepted as a complete query only.
				if (start != 0 || end != len(words)) && end-start == 1 && len(name) < 5 {
					continue
				}
				matches := byName[name]
				if len(matches) == 0 {
					continue
				}
				for _, cas := range matches {
					if !seen[cas] {
						source := "INFERRED_NAME"
						if declaredCAS[cas] {
							source = "DECLARED"
						}
						out = append(out, chemicalResult(cas, byCAS[cas], source, len(matches) > 1 && source != "DECLARED"))
						seen[cas] = true
					}
				}
				break
			}
			start = max(start+1, end)
		}
	}
	if len(nameLines) == 1 {
		query := normalizeChemicalName(nameLines[0])
		for _, cas := range similarChemicalNames(query, byName, seen, declaredCAS) {
			out = append(out, chemicalResult(cas, byCAS[cas], "SIMILAR_NAME", false))
		}
	}
	for _, cas := range declared {
		if !seen[cas] {
			out = append(out, chemicalResult(cas, byCAS[cas], "DECLARED", false))
		}
	}
	return out
}

func similarChemicalNames(query string, byName map[string][]string, seen, declared map[string]bool) []string {
	queryRunes := []rune(query)
	if len(queryRunes) < 5 || len(queryRunes) > 32 || strings.ContainsRune(query, ' ') {
		return nil
	}
	limit := 1
	if len(queryRunes) >= 7 {
		limit = 2
	}
	type candidate struct {
		cas      string
		distance int
	}
	distances := make(map[string]int)
	for name, codes := range byName {
		if strings.ContainsRune(name, ' ') {
			continue
		}
		alias := []rune(name)
		if len(alias) < 5 || len(alias) < len(queryRunes)-limit || len(alias) > len(queryRunes)+limit {
			continue
		}
		distance := chemicalNameDistance(queryRunes, alias)
		if distance == 0 || distance > limit {
			continue
		}
		for _, cas := range codes {
			if old, exists := distances[cas]; !seen[cas] && !declared[cas] && (!exists || distance < old) {
				distances[cas] = distance
			}
		}
	}
	ranked := make([]candidate, 0, len(distances))
	for cas, distance := range distances {
		ranked = append(ranked, candidate{cas, distance})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].distance != ranked[j].distance {
			return ranked[i].distance < ranked[j].distance
		}
		return ranked[i].cas < ranked[j].cas
	})
	out := make([]string, 0, min(len(ranked), 3))
	for _, entry := range ranked[:min(len(ranked), 3)] {
		out = append(out, entry.cas)
	}
	return out
}

func chemicalNameDistance(a, b []rune) int {
	row := make([]int, len(b)+1)
	for j := range row {
		row[j] = j
	}
	for i, x := range a {
		diagonal := row[0]
		row[0] = i + 1
		for j, y := range b {
			above := row[j+1]
			substitution := diagonal
			if x != y {
				substitution++
			}
			row[j+1] = min(above+1, row[j]+1, substitution)
			diagonal = above
		}
	}
	return row[len(b)]
}

func chemicalResult(cas string, record chemicalRecord, source string, ambiguous bool) Chemical {
	item := Chemical{CAS: cas, Name: cas, SourceType: source}
	if record.CAS == "" {
		item.PolicyTitle = "Chưa tìm thấy trong dữ liệu danh mục QA"
		item.Warnings = append(item.Warnings, "Cần đối chiếu lại mã CAS và tài liệu nguồn.")
		return item
	}
	if record.Name != "" {
		item.Name = record.Name
	}
	item.IUPAC = record.IUPAC
	item.Formula = record.Formula
	item.Memberships = record.Memberships
	item.PolicyTitle = "Kết quả đối chiếu dữ liệu danh mục QA"
	if source == "SIMILAR_NAME" {
		item.Warnings = append(item.Warnings, "Tên gần giống có thể là một hóa chất khác. Xác nhận mã CAS trên nhãn hoặc SDS trước khi sử dụng kết quả.")
	} else if ambiguous {
		item.Warnings = append(item.Warnings, "Tên này khớp nhiều mã CAS. Xác nhận lại trên nhãn hoặc SDS.")
	} else if source == "INFERRED_NAME" {
		item.Warnings = append(item.Warnings, "Mã CAS suy từ tên. Xác nhận lại trên nhãn hoặc SDS.")
	}
	if len(item.Memberships) > 0 {
		item.Warnings = append(item.Warnings, "Có bản ghi trong danh mục tham chiếu. Đối chiếu từng phụ lục bên dưới và kiểm tra điều kiện, ngoại lệ theo hồ sơ thực tế.")
	}
	for _, membership := range item.Memberships {
		if membership.Relationship == "exception" {
			item.Warnings = append(item.Warnings, "Mã này nằm trong đoạn ngoại trừ của mục được dẫn. Mở căn cứ để kiểm tra đúng phạm vi ngoại trừ.")
		} else if membership.Appendix == "III" {
			item.Warnings = append(item.Warnings, "Có mục Phụ lục III trong dữ liệu tham chiếu. Cần kiểm tra nhóm kiểm soát và ngoại lệ theo thành phần thực tế.")
		}
	}
	return item
}

func normalizeChemicalName(name string) string {
	const vowels = "aeiouyd"
	variants := [...]string{
		"àáảãạăằắẳẵặâầấẩẫậ", "èéẻẽẹêềếểễệ", "ìíỉĩị",
		"òóỏõọôồốổỗộơờớởỡợ", "ùúủũụưừứửữự", "ỳýỷỹỵ", "đ",
	}
	name = strings.Map(func(r rune) rune {
		r = unicode.ToLower(r)
		for i, accents := range variants {
			if strings.ContainsRune(accents, r) {
				return rune(vowels[i])
			}
		}
		if unicode.IsMark(r) {
			return -1
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, name)
	return strings.Join(strings.Fields(name), " ")
}
