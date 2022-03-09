package model

type Comparator struct {
	// represents way to compare threshold and result
	// 0: NA, 1: eq. 2: g, 3: ge, 4: l, 5: le
	Operator  int      `json:"operator,omitempty"`
	Threshold string   `json:"threshold,omitempty"`
	arg       []string `json:"arg,omitempty"`
}
