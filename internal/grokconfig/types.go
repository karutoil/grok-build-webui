package grokconfig

const (
	TypeString     = "string"
	TypeBool       = "bool"
	TypeInt        = "int"
	TypeFloat      = "float"
	TypeEnum       = "enum"
	TypeStringList = "string_list"
	TypeMap        = "map"
	TypeIntMap     = "int_map"
)

type Field struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type"`
	Default     any      `json:"default,omitempty"`
	Options     []string `json:"options,omitempty"`
	Secret      bool     `json:"secret,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
}

type Section struct {
	ID          string  `json:"id"`
	Group       string  `json:"group"`
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	Fields      []Field `json:"fields"`
}

type Collection struct {
	ID          string     `json:"id"`
	Group       string     `json:"group"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Prefix      string     `json:"prefix"`
	KeyLabel    string     `json:"key_label"`
	Fields      []Field    `json:"item_fields"`
	Templates   []Template `json:"templates,omitempty"`
}

type Template struct {
	ID          string         `json:"id"`
	Label       string         `json:"label"`
	Description string         `json:"description,omitempty"`
	Values      map[string]any `json:"values"`
	SuggestedID string         `json:"suggested_id,omitempty"`
}

type Group struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

type FieldView struct {
	Field
	Value any  `json:"value"`
	Set   bool `json:"set"`
}

type SectionView struct {
	ID          string      `json:"id"`
	Group       string      `json:"group"`
	Title       string      `json:"title"`
	Description string      `json:"description,omitempty"`
	Fields      []FieldView `json:"fields"`
}

type ItemView struct {
	ID     string      `json:"id"`
	Fields []FieldView `json:"fields"`
	Extra  []string    `json:"extra,omitempty"`
}

type CollectionView struct {
	ID          string     `json:"id"`
	Group       string     `json:"group"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Prefix      string     `json:"prefix"`
	KeyLabel    string     `json:"key_label"`
	ItemFields  []Field    `json:"item_fields"`
	Templates   []Template `json:"templates,omitempty"`
	Items       []ItemView `json:"items"`
}

type View struct {
	Path        string           `json:"path"`
	Exists      bool             `json:"exists"`
	MTime       string           `json:"mtime,omitempty"`
	Raw         string           `json:"raw"`
	Groups      []Group          `json:"groups"`
	Sections    []SectionView    `json:"sections"`
	Collections []CollectionView `json:"collections"`
}

type Patch struct {
	Set          map[string]any             `json:"set"`
	Unset        []string                   `json:"unset"`
	Collections  map[string]CollectionPatch `json:"collections"`
	Raw          *string                    `json:"raw"`
	Force        bool                       `json:"force"`
	IfMatchMTime string                     `json:"if_match_mtime"`
}

type CollectionPatch struct {
	Items  map[string]ItemPatch `json:"items"`
	Delete []string             `json:"delete"`
	Rename map[string]string    `json:"rename"`
}

type ItemPatch struct {
	Set   map[string]any `json:"set"`
	Unset []string       `json:"unset"`
}

func (p Patch) Empty() bool {
	if p.Raw != nil {
		return false
	}
	if len(p.Set) > 0 || len(p.Unset) > 0 {
		return false
	}
	for _, c := range p.Collections {
		if len(c.Items) > 0 || len(c.Delete) > 0 || len(c.Rename) > 0 {
			return false
		}
	}
	return true
}
