package noservice

// OnlyModel has no service interface.
type OnlyModel struct {
	ID   uint   `json:"id"   gorm:"primaryKey"`
	Name string `json:"name" gorm:"not null"`
}

type HelperStruct struct {
	Tags    []string          `json:"tags"`
	Options map[string]string `json:"options"`
	Ptr     *OnlyModel        `json:"ptr"`
}
