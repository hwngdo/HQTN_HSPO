package hspo

import "time"

// NOTE: These types are reconstructed from v1.9.4.1 binary strings and the shipped frontend.
// They are not claimed to be byte-for-byte copies of the original source.

type User struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password,omitempty"`
	Role        string `json:"role"`
	Active      bool   `json:"active"`
	CreatedAt   string `json:"created_at,omitempty"`
}

type VerifyItem struct {
	Kind      string `json:"kind"`
	Key       string `json:"key"`
	Label     string `json:"label,omitempty"`
	NeedCheck bool   `json:"need_check,omitempty"`
	Note      string `json:"note,omitempty"`
	L1User    string `json:"l1_user,omitempty"`
	L1Name    string `json:"l1_name,omitempty"`
	L1At      string `json:"l1_at,omitempty"`
	L2User    string `json:"l2_user,omitempty"`
	L2Name    string `json:"l2_name,omitempty"`
	L2At      string `json:"l2_at,omitempty"`
}

type Membership struct {
	Relationship string `json:"relationship,omitempty"`
	SourceCode   string `json:"source_code,omitempty"`
	ListCode     string `json:"list_code,omitempty"`
	ListName     string `json:"list_name,omitempty"`
	Appendix     string `json:"appendix,omitempty"`
	SourceSTT    string `json:"source_stt,omitempty"`
	ControlGroup string `json:"control_group,omitempty"`
}

type Chemical struct {
	CAS         string       `json:"cas,omitempty"`
	Name        string       `json:"name,omitempty"`
	IUPAC       string       `json:"iupac,omitempty"`
	Formula     string       `json:"formula,omitempty"`
	PolicyTitle string       `json:"policy_title,omitempty"`
	SourceType  string       `json:"source_type,omitempty"`
	Warnings    []string     `json:"warnings,omitempty"`
	Memberships []Membership `json:"memberships,omitempty"`
}

type HistoryItem struct {
	ID          string           `json:"id,omitempty"`
	IP          string           `json:"ip,omitempty"`
	Time        string           `json:"time"`
	Username    string           `json:"username,omitempty"`
	DisplayName string           `json:"display_name,omitempty"`
	Module      string           `json:"module"`
	Query       string           `json:"query"`
	SearchMode  string           `json:"search_mode,omitempty"`
	Conditions  []string         `json:"conditions,omitempty"`
	ResultCount int              `json:"result_count"`
	TopResults  []map[string]any `json:"top_results,omitempty"`
}

type Presence struct {
	Username string    `json:"username"`
	IP       string    `json:"ip"`
	LastSeen time.Time `json:"-"`
}
